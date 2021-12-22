package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"log"
	"net"
	"net/http"
	"sync"
)

const (
	CommandPing = 100
	CommandPong = 101
)

type Server struct {
	Once    sync.Once
	id      string
	address string
	users   map[string]net.Conn
	sync.RWMutex
}

func NewServer(id string, address string) *Server {
	return &Server{id: id, address: address, users: make(map[string]net.Conn)}
}

func (s *Server) handleBinary(user string, message []byte) {
	s.RLock()
	defer s.RUnlock()
	i := 0
	command := binary.BigEndian.Uint16(message[i : i+2])
	i += 2
	payloadLen := binary.BigEndian.Uint32(message[i : i+4])
	log.Println("command: %v payloadLen:%v", command, payloadLen)
	if command == CommandPing {
		conn := s.users[user]
		err := wsutil.WriteServerBinary(conn, []byte{0, CommandPong, 0, 0, 0, 0})
		if err != nil {
			log.Println("server write binary err:", err)
		}
	}
}

func (s *Server) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, req *http.Request) {
		fmt.Println("recv conn")
		conn, _, _, err := ws.UpgradeHTTP(req, writer)
		if err != nil {
			conn.Close()
			log.Println("err:", err)
			return
		}
		user := req.URL.Query().Get("user")
		if user == "" {
			conn.Close()
			return
		}
		old, ok := s.AddUser(user, conn)
		if ok {
			old.Close()
		}
		log.Printf("user %s in ", user)
		go func(user string, conn net.Conn) {
			err := s.readLoop(user, conn)
			if err != nil {
				log.Println("read loop err:", err)
			}
			conn.Close()
			s.delUser(user)
			log.Printf("user %s out", user)
		}(user, conn)
	})
	listen, err := net.Listen("tcp", ":"+s.address)
	if err != nil {
		log.Fatalln(err)
	}
	http.Serve(listen, mux)
}

func (s *Server) AddUser(user string, conn net.Conn) (net.Conn, bool) {
	s.Lock()
	defer s.Unlock()
	old, ok := s.users[user]
	s.users[user] = conn
	return old, ok
}

func (s *Server) readLoop(user string, conn net.Conn) error {
	for {
		frame, err := ws.ReadFrame(conn)
		if err != nil {
			return err
		}
		if frame.Header.OpCode == ws.OpClose {
			return errors.New("remote side close the conn")
		}
		if frame.Header.Masked {
			ws.Cipher(frame.Payload, frame.Header.Mask, 0)
		}
		if frame.Header.OpCode == ws.OpText {
			go s.handle(user, string(frame.Payload))
		}
		if frame.Header.OpCode == ws.OpPing {
			go s.handleBinary(user, frame.Payload)
		}
	}
}

func (s *Server) delUser(user string) {
	s.Lock()
	defer s.Unlock()
	delete(s.users, user)
}

func (s *Server) shutDown() {
	s.Once.Do(func() {
		s.Lock()
		defer s.Unlock()
		for _, conn := range s.users {
			conn.Close()
		}
	})
}

func (s *Server) handle(curUser string, message string) {
	log.Println("recv message %s from %s", message, curUser)
	s.Lock()
	defer s.Unlock()
	broadcast := fmt.Sprintf("%s -- from %s", message, curUser)
	for user, conn := range s.users {
		if user == curUser {
			continue
		}
		err := s.writeText(conn, broadcast)
		if err != nil {
			log.Printf("write %s msg err:%s", user, err)
		}
	}
}

func (s *Server) writeText(conn net.Conn, broadcast string) error {
	f := ws.NewTextFrame([]byte(broadcast))
	return ws.WriteFrame(conn, f)
}

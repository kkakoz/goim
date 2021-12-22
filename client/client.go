package client

import (
	"context"
	"errors"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"log"
	"net"
	"net/url"
	"time"
)

type handler struct {
	conn      net.Conn
	close     chan struct{}
	recv      chan []byte
	heartbeat time.Duration
}

func (h *handler) readloop(conn net.Conn) error {
	_ = h.conn.SetReadDeadline(time.Now().Add(h.heartbeat * 3))
	for {
		frame, err := ws.ReadFrame(conn)
		if err != nil {
			return err
		}
		if frame.Header.OpCode == ws.OpClose {
			return errors.New("remote side close the channel")
		}
		if frame.Header.OpCode == ws.OpPong {
			_ = h.conn.SetReadDeadline(time.Now().Add(h.heartbeat * 3))
		}
		if frame.Header.OpCode == ws.OpText {
			h.recv <- frame.Payload
		}
	}
}

func (h *handler) heartbeatloop() error {
	log.Println("heart beat loop started")
	tick := time.NewTicker(h.heartbeat)
	for range tick.C {
		log.Println("ping")
		err := wsutil.WriteClientMessage(h.conn, ws.OpPing, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func connect(addr string) (*handler, error) {
	_, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	conn, _, _, err := ws.Dial(context.TODO(), addr)
	if err != nil {
		return nil, err
	}

	h := &handler{
		conn:      conn,
		close:     make(chan struct{}, 1),
		recv:      make(chan []byte, 10),
		heartbeat: time.Second * 10,
	}

	go func() {
		err := h.readloop(conn)
		if err != nil {
			log.Println("read loop err:", err)
		}
		h.close <- struct{}{}
	}()

	go func() {
		err := h.heartbeatloop()
		if err != nil {
			log.Println("heart loop err:", err)
		}
		h.close <- struct{}{}
	}()

	return h, nil
}

func (h *handler) sendText(s string) error {
	log.Println("send msg", s)
	return wsutil.WriteClientText(h.conn, []byte(s))
}

type StartOptions struct {
	addr string
	user string
}

func run(ctx context.Context, ops *StartOptions) error {
	url := fmt.Sprintf("%s?user=%s", ops.addr, ops.user)
	log.Println("connect to ", url)
	h, err := connect(url)
	if err != nil {
		return err
	}
	go func() {
		for msg := range h.recv {
			log.Println("recv msg:", string(msg))
		}
	}()

	tk := time.NewTicker(time.Second * 6)
	for {
		select {
		case <-tk.C:
			err := h.sendText("hello")
			if err != nil {
				log.Println("send text err:", err)
			}
		case <-h.close:
			log.Println("conn close")
			return nil
		}
	}
}

package client

import (
	"context"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	go func() {
		err := run(context.TODO(), &StartOptions{
			addr: "ws://localhost:10001",
			user: "zhangsan",
		})
		if err != nil {
			t.Fatal(err)
		}
	}()
	go func() {
		err := run(context.TODO(), &StartOptions{
			addr: "ws://localhost:10001",
			user: "lisi",
		})
		if err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Minute)
}

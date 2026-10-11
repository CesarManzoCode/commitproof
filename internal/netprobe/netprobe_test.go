package netprobe

import (
	"net"
	"testing"
	"time"
)

func TestConnectOKAndRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	open := ln.Addr().String()
	l2, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := l2.Addr().String()
	l2.Close()
	defer ln.Close()
	c := RunConnect([]string{open, closed}, time.Second, 3)
	if !c.Result(open).AllOK() {
		t.Fatalf("open port: %+v", c.Result(open))
	}
	if m, ok := c.Result(open).MedianUS(); !ok || m <= 0 {
		t.Fatalf("median %d %v", m, ok)
	}
	if !c.Result(closed).AllFailedWith(KindRefused) || c.Result(closed).AllFailedWith(KindTimeout) {
		t.Fatalf("closed port: %+v", c.Result(closed))
	}
	if c.Result("1.2.3.4:5") != nil {
		t.Fatal("unknown address has a result")
	}
}

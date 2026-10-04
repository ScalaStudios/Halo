package mailtest

import (
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
)

type Delivery struct {
	Auth, From, To, Data string
}

type Server struct {
	Addr      string
	mu        sync.Mutex
	rejecting int
	received  []Delivery
}

func Start(t *testing.T, rejecting int) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &Server{Addr: ln.Addr().String(), rejecting: rejecting}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *Server) Reject(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejecting = n
}

func (s *Server) Deliveries() []Delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Delivery(nil), s.received...)
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	c := textproto.NewConn(conn)
	c.PrintfLine("220 mx.test ESMTP")
	var d Delivery
	for {
		line, err := c.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			c.PrintfLine("250-mx.test")
			c.PrintfLine("250 AUTH PLAIN")
		case "AUTH":
			d.Auth = arg
			c.PrintfLine("235 2.7.0 Authentication successful")
		case "MAIL":
			d.From = arg
			c.PrintfLine("250 2.1.0 OK")
		case "RCPT":
			s.mu.Lock()
			reject := s.rejecting > 0
			if reject {
				s.rejecting--
			}
			s.mu.Unlock()
			if reject {
				c.PrintfLine("451 4.3.0 Mailbox temporarily unavailable")
				continue
			}
			d.To = arg
			c.PrintfLine("250 2.1.5 OK")
		case "DATA":
			c.PrintfLine("354 Go ahead")
			data, err := c.ReadDotBytes()
			if err != nil {
				return
			}
			d.Data = string(data)
			s.mu.Lock()
			s.received = append(s.received, d)
			s.mu.Unlock()
			c.PrintfLine("250 2.0.0 Queued")
		case "QUIT":
			c.PrintfLine("221 2.0.0 Bye")
			return
		default:
			c.PrintfLine("502 5.5.2 Not implemented")
		}
	}
}

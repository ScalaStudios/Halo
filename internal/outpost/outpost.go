package outpost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

const heartbeat = 25 * time.Second

var userAgent = "halo-outpost/1 (" + runtime.GOOS + "/" + runtime.GOARCH + ")"

type message struct {
	Type    string `json:"type"`
	Stream  string `json:"stream,omitempty"`
	Address string `json:"address,omitempty"`
	Message string `json:"message,omitempty"`
}

func Enroll(ctx context.Context, server, token string) (string, error) {
	body, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/v1/outpost/enroll", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		Key   string `json:"key"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("halo answered %s without a JSON body; check the server address", res.Status)
	}
	if out.Error != nil {
		return "", errors.New(out.Error.Message)
	}
	if res.StatusCode != http.StatusCreated || out.Key == "" {
		return "", fmt.Errorf("halo answered %s without an outpost key", res.Status)
	}
	return out.Key, nil
}

type Agent struct {
	Server    string
	Key       string
	GuacdPort int
	Log       *slog.Logger
}

func (a *Agent) Run(ctx context.Context) error {
	wait := time.Second
	for {
		started := time.Now()
		err := a.serve(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > time.Minute {
			wait = time.Second
		}
		a.Log.Warn("lost the connection to Halo, retrying", "error", err, "in", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

func (a *Agent) dial(ctx context.Context, path string) (*websocket.Conn, error) {
	base, err := url.Parse(strings.TrimRight(a.Server, "/"))
	if err != nil {
		return nil, err
	}
	origin := base.String()
	base.Scheme = map[string]string{"https": "wss", "http": "ws"}[base.Scheme]
	config, err := websocket.NewConfig(base.String()+path, origin)
	if err != nil {
		return nil, err
	}
	config.Header.Set("Authorization", "Bearer "+a.Key)
	config.Header.Set("User-Agent", userAgent)
	config.Dialer = &net.Dialer{Timeout: 15 * time.Second}
	upgrade, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := config.DialContext(upgrade)
	if err != nil {
		return nil, fmt.Errorf("halo refused the connection to %s; check the server address and that the outpost was not deleted or enrolled again: %w", path, err)
	}
	return conn, nil
}

func (a *Agent) serve(ctx context.Context) error {
	conn, err := a.dial(ctx, "/api/v1/outpost/connect")
	if err != nil {
		return err
	}
	defer closeSocket(conn)
	stop := context.AfterFunc(ctx, func() { closeSocket(conn) })
	defer stop()
	a.Log.Info("connected to Halo", "server", a.Server)
	var writeMu sync.Mutex
	send := func(m message) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return sendControl(conn, m)
	}
	for {
		conn.SetReadDeadline(time.Now().Add(3 * heartbeat))
		var raw string
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return err
		}
		var m message
		if json.Unmarshal([]byte(raw), &m) != nil {
			continue
		}
		switch m.Type {
		case "ping":
			if err := send(message{Type: "pong"}); err != nil {
				return err
			}
		case "dial", "desktop":
			go a.relay(ctx, send, m)
		}
	}
}

func sendControl(conn *websocket.Conn, m message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(raw))
}

func closeSocket(conn *websocket.Conn) {
	conn.SetDeadline(time.Now())
	conn.Close()
}

func (a *Agent) relay(ctx context.Context, send func(message) error, m message) {
	address := m.Address
	if m.Type == "desktop" {
		if a.GuacdPort <= 0 || a.GuacdPort > 65535 {
			send(message{Type: "failed", Stream: m.Stream, Message: "the outpost does not have a local Guacamole proxy configured"})
			return
		}
		address = fmt.Sprintf("127.0.0.1:%d", a.GuacdPort)
	}
	target, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		a.Log.Warn("could not reach a host for Halo", "address", m.Address, "error", err)
		send(message{Type: "failed", Stream: m.Stream, Message: err.Error()})
		return
	}
	defer target.Close()
	stream, err := a.dial(ctx, "/api/v1/outpost/streams/"+url.PathEscape(m.Stream))
	if err != nil {
		send(message{Type: "failed", Stream: m.Stream, Message: err.Error()})
		return
	}
	stream.PayloadType = websocket.BinaryFrame
	defer closeSocket(stream)
	a.Log.Info("relaying a session", "address", m.Address)
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(stream, target)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(target, stream)
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	a.Log.Info("session ended", "address", m.Address)
}

package remote

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/websocket"

	"halo/internal/store"
)

const desktopFrameLimit = 1 << 20

func desktopDimension(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 320 || value > 4096 {
		return fallback
	}
	return value
}

func writeDesktop(w io.Writer, fields ...string) error {
	var instruction strings.Builder
	for i, field := range fields {
		if i > 0 {
			instruction.WriteByte(',')
		}
		instruction.WriteString(strconv.Itoa(utf8.RuneCountInString(field)))
		instruction.WriteByte('.')
		instruction.WriteString(field)
	}
	instruction.WriteByte(';')
	_, err := io.WriteString(w, instruction.String())
	return err
}

func readDesktop(reader *bufio.Reader) ([]string, error) {
	var fields []string
	size := 0
	for len(fields) < 256 {
		n := 0
		for digits := 0; ; digits++ {
			b, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if b == '.' && digits > 0 {
				break
			}
			if b < '0' || b > '9' || digits >= 7 {
				return nil, errors.New("invalid desktop frame")
			}
			n = 10*n + int(b-'0')
		}
		if n > desktopFrameLimit-size {
			return nil, errors.New("desktop frame is too large")
		}
		var value strings.Builder
		for range n {
			r, width, err := reader.ReadRune()
			if err != nil {
				return nil, err
			}
			size += width
			if size > desktopFrameLimit || r == utf8.RuneError && width == 1 {
				return nil, errors.New("invalid desktop frame")
			}
			value.WriteRune(r)
		}
		fields = append(fields, value.String())
		end, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		switch end {
		case ';':
			return fields, nil
		case ',':
		default:
			return nil, errors.New("invalid desktop frame delimiter")
		}
	}
	return nil, errors.New("too many desktop frame fields")
}

func openDesktop(conn io.ReadWriter, target store.RemoteTarget, password string, width, height int) (*bufio.Reader, error) {
	if err := writeDesktop(conn, "select", target.Protocol); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	args, err := readDesktop(reader)
	if err != nil {
		return nil, err
	}
	if len(args) < 3 || args[0] != "args" || args[1] != "VERSION_1_5_0" {
		return nil, errors.New("the outpost needs a compatible Guacamole 1.6 proxy")
	}
	params := map[string]string{
		"hostname": target.Host, "port": strconv.Itoa(target.Port), "username": target.Username, "password": password,
		"disable-copy": "true", "disable-paste": "true", "disable-audio": "true",
	}
	if target.Protocol == "rdp" {
		params["security"] = target.RDPSecurity
		fingerprint, err := hex.DecodeString(target.HostKey)
		if err != nil || len(fingerprint) != 32 {
			return nil, errors.New("invalid RDP certificate fingerprint")
		}
		params["cert-fingerprints"] = "sha256:" + strings.ReplaceAll(fmt.Sprintf("% x", fingerprint), " ", ":")
		params["ignore-cert"], params["cert-tofu"] = "false", "false"
		if domain, user, ok := strings.Cut(target.Username, "\\"); ok {
			params["domain"], params["username"] = domain, user
		}
	}
	fields := []string{"connect", args[1]}
	for _, name := range args[2:] {
		fields = append(fields, params[name])
		delete(params, name)
	}
	if _, missing := params["hostname"]; missing || params["port"] != "" || params["password"] != "" ||
		target.Protocol == "rdp" && (params["cert-fingerprints"] != "" || params["security"] != "") {
		return nil, errors.New("the outpost proxy does not support the required desktop settings")
	}
	for _, frame := range [][]string{{"size", strconv.Itoa(width), strconv.Itoa(height), "96"}, {"audio"}, {"video"}, {"image", "image/png", "image/jpeg"}, fields} {
		if err := writeDesktop(conn, frame...); err != nil {
			return nil, err
		}
	}
	result, err := readDesktop(reader)
	if err != nil {
		return nil, err
	}
	if len(result) != 2 || result[0] != "ready" {
		return nil, errors.New("the proxy refused the desktop connection")
	}
	return reader, nil
}

func (h *handlers) desktop(ctx context.Context, end context.CancelCauseFunc, rs store.RemoteSession, target store.RemoteTarget, password string, b *bridge, ends time.Time, width, height int) error {
	link, err := h.hub.Dial(ctx, deref(rs.OutpostID), "", "desktop")
	if err != nil {
		return errors.New("The outpost could not open the desktop proxy. Check the outpost and its Guacamole configuration.")
	}
	defer link.Close()
	stop := context.AfterFunc(ctx, func() { link.Close() })
	defer stop()
	if err := link.conn.SetDeadline(time.Now().Add(handshakeLimit)); err != nil {
		return errClosed
	}
	reader, err := openDesktop(link.conn, target, password, width, height)
	if err != nil {
		slog.Warn("desktop handshake failed", "session", rs.ID, "error", err)
		return errors.New("The desktop proxy could not start a session.")
	}
	var initial strings.Builder
	for {
		frame, err := readDesktop(reader)
		if err != nil || frame[0] == "error" || frame[0] == "disconnect" {
			return errors.New("The remote desktop refused the connection. Check the host, credentials and certificate.")
		}
		if err := writeDesktop(&initial, frame...); err != nil || initial.Len() > desktopFrameLimit {
			return errFailure
		}
		if frame[0] == "size" && len(frame) == 4 && frame[1] == "0" {
			break
		}
		if initial.Len() == desktopFrameLimit {
			return errFailure
		}
	}
	if err := h.activeRemoteSession(ctx, rs.ID); err != nil {
		return err
	}
	if !h.allowed(ctx, rs) {
		return errRevoked
	}
	if err := link.conn.SetDeadline(time.Time{}); err != nil {
		return errClosed
	}
	if err := b.emit(event{Type: "connected", Username: rs.Username, Address: rs.Address, EndsAt: &ends}); err != nil {
		return errClosed
	}
	if _, err := b.Write([]byte(initial.String())); err != nil {
		return errClosed
	}
	go h.watch(ctx, end, b, rs)
	go func() { end(b.desktopInput(link.conn)) }()
	go func() {
		_, err := io.Copy(b, reader)
		if err != nil {
			end(errClosed)
		} else {
			end(errors.New("The remote desktop disconnected."))
		}
	}()
	<-ctx.Done()
	return context.Cause(ctx)
}

func (b *bridge) desktopInput(conn *websocket.Conn) error {
	for {
		b.ws.SetReadDeadline(time.Now().Add(3 * heartbeat))
		var message string
		if err := websocket.Message.Receive(b.ws, &message); err != nil {
			return errClosed
		}
		var frame struct {
			Type   string   `json:"type"`
			Opcode string   `json:"opcode"`
			Args   []string `json:"args"`
		}
		if len(message) > 1024 || json.Unmarshal([]byte(message), &frame) != nil {
			return errClosed
		}
		if frame.Type == "ping" {
			continue
		}
		if frame.Type != "desktop" || !allowDesktopInput(frame.Opcode, frame.Args) {
			return errClosed
		}
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := writeDesktop(conn, append([]string{frame.Opcode}, frame.Args...)...); err != nil {
			return errClosed
		}
		b.in.Add(int64(len(message)))
	}
}

func allowDesktopInput(opcode string, args []string) bool {
	switch opcode {
	case "nop":
		return len(args) == 0
	case "key", "size":
		if len(args) != 2 {
			return false
		}
	case "mouse", "ack":
		if len(args) != 3 {
			return false
		}
	case "sync":
		if len(args) != 1 {
			return false
		}
	default:
		return false
	}
	for i, arg := range args {
		if opcode == "ack" && i == 1 {
			continue
		}
		value, err := strconv.ParseInt(arg, 10, 64)
		if err != nil || value < 0 || opcode == "size" && (value < 320 || value > 4096) {
			return false
		}
	}
	return true
}

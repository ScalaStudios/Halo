package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"halo/internal/config"
	"halo/internal/store"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
	Configured() bool
}

type Log struct{}

func (Log) Send(ctx context.Context, m Message) error {
	slog.InfoContext(ctx, "email not sent: SMTP is not configured", "to", m.To, "subject", m.Subject)
	return nil
}

func (Log) Configured() bool {
	return false
}

const maxAttempts = 5

type Outbox struct {
	cfg config.Config
	st  *store.Store
}

func New(cfg config.Config, st *store.Store) *Outbox {
	return &Outbox{cfg: cfg, st: st}
}

func (o *Outbox) Configured() bool {
	return o.cfg.SMTP.Configured()
}

func (o *Outbox) Send(ctx context.Context, m Message) error {
	organization := o.cfg.Organization
	if settings, err := o.st.Settings(ctx); err == nil && settings.OrganizationName != "" {
		organization = settings.OrganizationName
	}
	msg, err := o.st.QueueMail(ctx, m.To, m.Subject, m.Text+"\n\nHalo · "+organization+"\n", o.Configured())
	if err != nil {
		return err
	}
	if !o.Configured() {
		slog.InfoContext(ctx, "email queued but not sent: SMTP is not configured", "to", m.To, "subject", m.Subject)
		return nil
	}
	return o.deliver(ctx, msg)
}

func (o *Outbox) Retry(ctx context.Context) error {
	return o.retry(ctx, time.Now())
}

func (o *Outbox) retry(ctx context.Context, now time.Time) error {
	if !o.Configured() {
		return nil
	}
	due, err := o.st.ClaimDueMail(ctx, now, maxAttempts, 20)
	for _, msg := range due {
		if err := o.deliver(ctx, msg); err != nil {
			slog.WarnContext(ctx, "email delivery failed", "message", msg.ID, "attempt", msg.Attempts, "error", err)
		}
	}
	return err
}

func (o *Outbox) deliver(ctx context.Context, msg store.OutboxMessage) error {
	failure := o.transmit(ctx, msg)
	if err := o.st.RecordMailDelivery(context.WithoutCancel(ctx), msg.ID, failure); err != nil {
		return errors.Join(failure, err)
	}
	return failure
}

func (o *Outbox) transmit(ctx context.Context, msg store.OutboxMessage) error {
	s := o.cfg.SMTP
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("HALO_SMTP_FROM %q is not an email address: %w", s.From, err)
	}
	addr := net.JoinHostPort(s.Host, s.Port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.Port == "465" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: s.Host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if s.Port != "465" && !loopback(s.Host) {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return fmt.Errorf("%s did not accept STARTTLS, and Halo only sends email in plain text to localhost: %w", addr, err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(msg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(compose(from, msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || ip != nil && ip.IsLoopback()
}

func compose(from *mail.Address, msg store.OutboxMessage) []byte {
	var b bytes.Buffer
	for _, h := range [][2]string{
		{"From", from.String()},
		{"To", (&mail.Address{Address: msg.To}).String()},
		{"Subject", mime.QEncoding.Encode("utf-8", msg.Subject)},
		{"Date", msg.CreatedAt.Format(time.RFC1123Z)},
		{"Message-ID", "<" + msg.ID + "@" + from.Address[strings.LastIndex(from.Address, "@")+1:] + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/plain; charset=utf-8"},
		{"Content-Transfer-Encoding", "quoted-printable"},
	} {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	body := quotedprintable.NewWriter(&b)
	body.Write([]byte(msg.Text))
	body.Close()
	return b.Bytes()
}

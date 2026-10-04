package mail

import (
	"context"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"

	"halo/internal/config"
	"halo/internal/mail/mailtest"
	"halo/internal/store"
	"halo/internal/testdb"
)

func outbox(t *testing.T, smtp *mailtest.Server) (*Outbox, *store.Store) {
	st := testdb.New(t)
	cfg := config.Config{Organization: "Fernway Systems"}
	if smtp != nil {
		host, port, _ := net.SplitHostPort(smtp.Addr)
		cfg.SMTP = config.SMTP{Host: host, Port: port, Username: "halo", Password: "s3cret", From: "Halo <halo@fernway.example>"}
	}
	return New(cfg, st), st
}

func latest(t *testing.T, st *store.Store) store.OutboxMessage {
	t.Helper()
	messages, err := st.ListMail(context.Background(), 1)
	if err != nil || len(messages) != 1 {
		t.Fatalf("outbox: %v %v", messages, err)
	}
	return messages[0]
}

func TestOutboxDeliversOverSMTP(t *testing.T) {
	server := mailtest.Start(t, 0)
	o, st := outbox(t, server)
	ctx := context.Background()
	expires := time.Date(2026, 10, 11, 14, 2, 0, 0, time.UTC)
	if err := o.Send(ctx, Invite("sam@example.com", "Sam Lee", "Uma Patel", "https://auth.example.com/enroll?token=abc", expires)); err != nil {
		t.Fatal(err)
	}

	got := server.Deliveries()
	if len(got) != 1 {
		t.Fatalf("deliveries: %+v", got)
	}
	d := got[0]
	credentials, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(d.Auth, "PLAIN "))
	if string(credentials) != "\x00halo\x00s3cret" || d.From != "FROM:<halo@fernway.example>" || d.To != "TO:<sam@example.com>" {
		t.Fatalf("envelope: %+v", d)
	}
	msg, err := mail.ReadMessage(strings.NewReader(d.Data))
	if err != nil {
		t.Fatal(err)
	}
	for header, want := range map[string]string{
		"From": `"Halo" <halo@fernway.example>`, "To": "<sam@example.com>", "Subject": "Set up your Halo account",
		"Mime-Version": "1.0", "Content-Type": "text/plain; charset=utf-8", "Content-Transfer-Encoding": "quoted-printable",
	} {
		if msg.Header.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, msg.Header.Get(header), want)
		}
	}
	if date, err := msg.Header.Date(); err != nil || time.Since(date) > time.Minute {
		t.Errorf("Date header: %v %v", date, err)
	}
	queued := latest(t, st)
	if msg.Header.Get("Message-Id") != "<"+queued.ID+"@fernway.example>" {
		t.Errorf("Message-ID = %q", msg.Header.Get("Message-Id"))
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	if text != queued.Text || !strings.HasPrefix(text, "Hi Sam,\n\nUma Patel invited you") || !strings.Contains(text, "\nhttps://auth.example.com/enroll?token=abc\n") ||
		!strings.Contains(text, "expires 11 October 2026, 14:02 UTC.") || !strings.HasSuffix(text, "\n\nHalo · Fernway Systems\n") {
		t.Fatalf("body:\n%s", text)
	}
	if queued.SentAt == nil || queued.Attempts != 1 || queued.LastError != "" {
		t.Fatalf("outbox row: %+v", queued)
	}
}

func TestOutboxRetriesWithBackoff(t *testing.T) {
	server := mailtest.Start(t, 2)
	o, st := outbox(t, server)
	ctx := context.Background()
	start := time.Now()
	if err := o.Send(ctx, MagicLink("ada@example.com", "Ada", "", "https://auth.example.com/sign-in/magic?token=x")); err == nil || !strings.Contains(err.Error(), "451") {
		t.Fatalf("first attempt should fail with the server's 451: %v", err)
	}
	if m := latest(t, st); m.Attempts != 1 || m.SentAt != nil || !strings.Contains(m.LastError, "451") {
		t.Fatalf("after first failure: %+v", m)
	}
	step := func(after time.Duration, attempts int, sent bool) {
		t.Helper()
		if err := o.retry(ctx, start.Add(after)); err != nil {
			t.Fatal(err)
		}
		if m := latest(t, st); m.Attempts != attempts || (m.SentAt != nil) != sent {
			t.Fatalf("at +%s: want %d attempts, sent=%v, got %+v", after, attempts, sent, m)
		}
	}
	step(30*time.Second, 1, false)
	step(2*time.Minute, 2, false)
	step(3*time.Minute, 2, false)
	step(5*time.Minute, 3, true)
	if m := latest(t, st); m.LastError != "" || len(server.Deliveries()) != 1 {
		t.Fatalf("after recovery: %+v, %d deliveries", m, len(server.Deliveries()))
	}

	server.Reject(100)
	if err := o.Send(ctx, MagicLink("ada@example.com", "Ada", "Grafana", "https://auth.example.com/sign-in/magic?token=y")); err == nil {
		t.Fatal("want a delivery error")
	}
	step(2*time.Minute, 2, false)
	step(5*time.Minute, 3, false)
	step(10*time.Minute, 4, false)
	step(20*time.Minute, 5, false)
	step(40*time.Minute, 5, false)
	step(3*time.Hour, 5, false)
	if m := latest(t, st); !strings.Contains(m.LastError, "451") {
		t.Fatalf("last error not recorded: %+v", m)
	}
}

func TestOutboxWithoutSMTP(t *testing.T) {
	o, st := outbox(t, nil)
	ctx := context.Background()
	if o.Configured() {
		t.Fatal("no SMTP host means not configured")
	}
	if err := o.Send(ctx, Reset("sam@example.com", "Sam Lee", "Uma Patel", "https://auth.example.com/enroll?token=r", time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := o.Retry(ctx); err != nil {
		t.Fatal(err)
	}
	m := latest(t, st)
	if m.Attempts != 0 || m.SentAt != nil || m.Subject != "Reset how you sign in to Halo" || !strings.Contains(m.Text, "https://auth.example.com/enroll?token=r") {
		t.Fatalf("queued message: %+v", m)
	}
}

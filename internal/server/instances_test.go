package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"halo/internal/config"
	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

func TestTwoInstancesShareOneDatabase(t *testing.T) {
	public, _ := url.Parse("http://localhost:3200")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cfg := config.Config{Organization: "Test", PublicURL: public, SecretKey: key, Dev: true}
	var instances [2]*env
	var runs [2]atomic.Int32
	var stops [2]context.CancelFunc
	var stopped [2]chan struct{}
	for i, st := range testdb.Instances(t, 2) {
		app, err := New(cfg, st)
		if err != nil {
			t.Fatal(err)
		}
		app.Jobs.Every("count runs", 10*time.Millisecond, func(context.Context) error {
			runs[i].Add(1)
			return nil
		})
		instances[i] = &env{t: t, st: st, h: app.Handler, app: app}
		instances[i].expect("", "GET", "/readyz", nil, http.StatusServiceUnavailable, "")
		ctx, stop := context.WithCancel(context.Background())
		stops[i], stopped[i] = stop, make(chan struct{})
		go func() {
			defer close(stopped[i])
			app.Run(ctx)
		}()
		t.Cleanup(func() {
			stop()
			<-stopped[i]
		})
	}
	a, b := instances[0], instances[1]
	ctx := context.Background()
	eventually(t, "both instances must report ready", func() bool {
		return a.call("", "GET", "/readyz", nil).Code == http.StatusOK && b.call("", "GET", "/readyz", nil).Code == http.StatusOK
	})

	eventually(t, "background jobs must run", func() bool { return runs[0].Load()+runs[1].Load() > 0 })
	time.Sleep(200 * time.Millisecond)
	leader := 0
	if runs[1].Load() > 0 {
		leader = 1
	}
	if runs[1-leader].Load() != 0 {
		t.Fatalf("background jobs must run in one instance at a time: %d and %d runs", runs[0].Load(), runs[1].Load())
	}

	_, admin, _ := a.person("ada@example.com", "passkey", false, "global_admin")
	if s, err := b.st.Settings(ctx); err != nil || s.LockoutThreshold != 5 {
		t.Fatalf("settings: %+v %v", s.LockoutThreshold, err)
	}
	a.expect(admin, "PATCH", "/api/v1/settings", map[string]any{"lockoutThreshold": 9}, http.StatusOK, "")
	eventually(t, "a settings change on one instance must reach the other before its cache expires", func() bool {
		s, err := b.st.Settings(ctx)
		return err == nil && s.LockoutThreshold == 9
	})

	grace, _, seed := a.person("grace@example.com", "passkey", true)
	if err := a.st.SetPassword(ctx, grace.ID, secret.HashPassword("correct horse battery staple"), false); err != nil {
		t.Fatal(err)
	}
	if err := a.st.SetMethodEnabled(ctx, "password", true); err != nil {
		t.Fatal(err)
	}
	off, _ := json.Marshal(false)
	if err := a.st.SaveSettingValues(ctx, map[string]json.RawMessage{"passwordBreachCheck": off}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "both instances must see the password settings", func() bool {
		sa, errA := a.st.Settings(ctx)
		sb, errB := b.st.Settings(ctx)
		return errA == nil && errB == nil && !sa.PasswordBreachCheck && !sb.PasswordBreachCheck
	})
	signIn := map[string]any{"email": "grace@example.com", "password": "correct horse battery staple"}
	a.expect("", "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_SECOND_FACTOR_REQUIRED")
	signIn["code"], _ = totp.GenerateCode(seed, time.Now())
	cookies := b.expect("", "POST", "/api/v1/auth/password", signIn, http.StatusOK, "").Result().Cookies()
	i := slices.IndexFunc(cookies, func(c *http.Cookie) bool { return c.Name == httpx.SessionCookie })
	if i < 0 {
		t.Fatalf("no session cookie: %v", cookies)
	}
	a.expect(cookies[i].Value, "GET", "/api/v1/me", nil, http.StatusOK, "")
	a.expect("", "POST", "/api/v1/auth/password", signIn, http.StatusUnauthorized, "ERR_CODE_MISMATCH")

	account, err := a.st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Terraform", Roles: []string{"global_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.UpdateServiceAccount(ctx, account.ID, account.Name, "", nil, 3); err != nil {
		t.Fatal(err)
	}
	_, apiKey, err := a.st.CreateAPIKey(ctx, account.ID, "Terraform key", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		minute := time.Now().Truncate(time.Minute)
		var codes []int
		for n := range 4 {
			codes = append(codes, instances[n%2].call(apiKey, "GET", "/api/v1/users", nil).Code)
		}
		if time.Now().Truncate(time.Minute).Equal(minute) {
			if !slices.Equal(codes, []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusTooManyRequests}) {
				t.Fatalf("the rate limit must count requests to both instances: %v", codes)
			}
			break
		}
	}

	retransmission := []byte("203.0.113.9:1812 retransmission")
	if _, fresh, err := a.st.ClaimRADIUSRequest(ctx, retransmission, time.Minute); err != nil || !fresh {
		t.Fatalf("first RADIUS request: %v %v", fresh, err)
	}
	if reply, fresh, err := b.st.ClaimRADIUSRequest(ctx, retransmission, time.Minute); err != nil || fresh || reply != nil {
		t.Fatalf("a retransmission while the other instance answers must be dropped: %v %q %v", fresh, reply, err)
	}
	if err := a.st.KeepRADIUSReply(ctx, retransmission, []byte("reply")); err != nil {
		t.Fatal(err)
	}
	if reply, fresh, err := b.st.ClaimRADIUSRequest(ctx, retransmission, time.Minute); err != nil || fresh || string(reply) != "reply" {
		t.Fatalf("a retransmission must get the first instance's reply: %v %q %v", fresh, reply, err)
	}
	expires := time.Now().Add(time.Minute)
	if err := a.st.UseDPoPProof(ctx, "thumbprint", "proof-1", expires); err != nil {
		t.Fatal(err)
	}
	if err := b.st.UseDPoPProof(ctx, "thumbprint", "proof-1", expires); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("a DPoP proof used on one instance must be refused on the other: %v", err)
	}

	stops[leader]()
	<-stopped[leader]
	instances[leader].expect("", "GET", "/readyz", nil, http.StatusServiceUnavailable, "")
	eventually(t, "the other instance must take over background jobs", func() bool { return runs[1-leader].Load() > 0 })
}

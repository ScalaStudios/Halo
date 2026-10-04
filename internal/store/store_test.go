package store_test

import (
	"context"
	"strings"
	"testing"

	"halo/internal/id"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestUsersGroupsAndApps(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()

	rule := `user.department == "Engineering"`
	eng, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	admins, err := st.CreateGroup(ctx, store.NewGroup{Name: "Admins", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	ada, err := st.CreateUser(ctx, store.NewUser{Email: "Ada@Example.com", Name: "Ada", Department: "Engineering", Status: "active", Roles: []string{"global_admin"}, GroupIDs: []string{admins.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Duplicate"}); err != store.ErrConflict {
		t.Fatalf("duplicate email: want ErrConflict, got %v", err)
	}
	if len(ada.GroupIDs) != 2 || ada.Roles[0] != "Global administrator" || ada.Strength != "single-factor" {
		t.Fatalf("unexpected user: %+v", ada)
	}

	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Grafana", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://grafana.example.com/login/generic_oauth"}, GroupIDs: []string{eng.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(app.ClientID, "hl_") || app.UserCount != 1 {
		t.Fatalf("unexpected app: %+v", app)
	}
	_, value, err := st.AddClientSecret(ctx, app.ID, "Production", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyClientSecret(ctx, app.ClientID, value); err != store.ErrNotFound {
		t.Fatalf("expired secret must not verify, got %v", err)
	}
	_, value, err = st.AddClientSecret(ctx, app.ID, "Production", 1e12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyClientSecret(ctx, app.ClientID, value); err != nil {
		t.Fatalf("valid secret: %v", err)
	}
	if _, err := st.VerifyClientSecret(ctx, app.ClientID, value+"x"); err != store.ErrNotFound {
		t.Fatalf("wrong secret must not verify, got %v", err)
	}

	reloaded, err := st.GetUser(ctx, ada.ID)
	if err != nil || len(reloaded.AppIDs) != 1 || reloaded.AppIDs[0] != app.ID {
		t.Fatalf("app assignment through dynamic group: %+v %v", reloaded.AppIDs, err)
	}
}

func TestSessionsAndEnrollment(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, store.NewUser{Email: "sam@example.com", Name: "Sam"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := st.CreateEnrollmentToken(ctx, u.ID, "invite", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := st.PeekEnrollmentToken(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("peek: %v", err)
	}
	if _, _, err := st.ConsumeEnrollmentToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ConsumeEnrollmentToken(ctx, token); err != store.ErrNotFound {
		t.Fatalf("token reuse must fail, got %v", err)
	}

	first, firstToken, err := st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey", UserAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Browser != "Firefox 143" || first.OS != "Linux" {
		t.Fatalf("user agent parsing: %+v", first)
	}
	_, _, err = st.CreateSession(ctx, store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.RevokeUserSessions(ctx, u.ID, first.ID); err != nil || n != 1 {
		t.Fatalf("revoke others: %d %v", n, err)
	}
	if _, err := st.SessionByToken(ctx, firstToken); err != nil {
		t.Fatalf("kept session must still resolve: %v", err)
	}
	if err := st.RevokeSession(ctx, first.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SessionByToken(ctx, firstToken); err != store.ErrNotFound {
		t.Fatalf("revoked session must not resolve, got %v", err)
	}
}

func TestIDs(t *testing.T) {
	a, b := id.New("usr"), id.New("usr")
	if len(a) != 30 || a == b || !strings.HasPrefix(a, "usr_") {
		t.Fatalf("ids: %s %s", a, b)
	}
}

func TestParseUserAgent(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1": "iPhone|Safari 26",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0":           "Windows PC|Edge 141",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36":                                   "Linux computer|Chrome 141",
		"kubelogin/v1.34.0 (linux/amd64)": "Unknown device|kubelogin 1.34",
	}
	for ua, want := range cases {
		device, _, browser := store.ParseUserAgent(ua)
		if got := device + "|" + browser; got != want {
			t.Errorf("%s: got %s, want %s", ua, got, want)
		}
	}
}

package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"halo/internal/store"
)

func TestExpiredGuestLosesAccessOnTheNextRequest(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	sponsor, _, _ := e.person("sam@example.com", "passkey", false)
	until := time.Now().Add(time.Hour)
	guest, err := e.st.CreateUser(ctx, store.NewUser{Email: "pat@partner.example", Name: "Pat", Status: "active", Guest: true, SponsorID: &sponsor.ID, ExpiresAt: &until})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := e.st.CreateSession(ctx, store.NewSession{UserID: guest.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	e.expect(token, "GET", "/api/v1/me", nil, http.StatusOK, "")
	if err := e.st.SetGuestAccess(ctx, guest.ID, sponsor.ID, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	e.expect(token, "GET", "/api/v1/me", nil, http.StatusUnauthorized, "ERR_UNAUTHENTICATED")
}

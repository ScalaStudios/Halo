package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"halo/internal/httpx"
	"halo/internal/id"
	"halo/internal/store"
	"halo/internal/webhooks"
)

const logoutEvent = "http://schemas.openid.net/event/backchannel-logout"

var LogoutClient = httpx.GuardedClient()

func DeliverLogouts(ctx context.Context, st *store.Store, issuer string, now time.Time) error {
	due, err := st.ClaimLogoutDeliveries(ctx, now, 2*time.Minute, 100)
	if err != nil {
		return err
	}
	s := &storage{st: st, issuer: issuer}
	for _, d := range due {
		app, err := st.GetApplication(ctx, d.AppID)
		if err != nil {
			return err
		}
		token, err := s.logoutToken(ctx, app, d.Body, now)
		if err != nil {
			return err
		}
		code, message := 0, "The back-channel logout URI is not a valid URL."
		if req, err := http.NewRequestWithContext(ctx, http.MethodPost, app.BackchannelLogoutURI, strings.NewReader(url.Values{"logout_token": {token}}.Encode())); err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("User-Agent", "Halo-Logout")
			code, message = webhooks.Post(LogoutClient, req)
		}
		if err := webhooks.Record(ctx, st, d, code, message, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *storage) logoutToken(ctx context.Context, app store.Application, body string, now time.Time) (string, error) {
	var session struct{ Sub, Sid string }
	if err := json.Unmarshal([]byte(body), &session); err != nil {
		return "", err
	}
	ctx = withAlgorithms(ctx)
	if err := choose(ctx, app.TokenPolicy.IDTokenAlg, nil, nil); err != nil {
		return "", err
	}
	key, err := s.SigningKey(ctx)
	if err != nil {
		return "", err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: key.SignatureAlgorithm(), Key: jose.JSONWebKey{Key: key.Key(), KeyID: key.ID()}}, (&jose.SignerOptions{}).WithType("logout+jwt"))
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"iss": s.issuer, "aud": app.ClientID, "iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix(), "jti": id.New("lgt"),
		"sub": session.Sub, "sid": session.Sid, "events": map[string]any{logoutEvent: map[string]any{}}})
	if err != nil {
		return "", err
	}
	signed, err := signer.Sign(claims)
	if err != nil {
		return "", err
	}
	return signed.CompactSerialize()
}

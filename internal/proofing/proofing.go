package proofing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const (
	startLimit = 5
	pollLimit  = 120
	maxBody    = 64 << 10
)

var (
	Client         = httpx.GuardedClient()
	StripeURL      = "https://api.stripe.com/"
	errSignature   = httpx.Fail(http.StatusUnauthorized, "ERR_SIGNATURE", "The signature header is missing, older than 5 minutes or does not match the webhook signing secret saved in Halo.")
	errUnavailable = httpx.Fail(http.StatusBadGateway, "ERR_IDENTITY_PROVIDER_UNAVAILABLE", "Halo could not start the identity check. Try again in a few minutes, or ask your administrator to check your identity in person.")
	errNoProvider  = httpx.Fail(http.StatusConflict, "ERR_NO_IDENTITY_PROVIDER", "Your organization checks identities in person. Ask your administrator to check yours.")
	errTooMany     = httpx.Fail(http.StatusTooManyRequests, "ERR_RATE_LIMITED", "You started 5 identity checks in the last hour. Wait an hour and try again, or ask your administrator to check your identity in person.")
)

type Subject struct {
	UserID   string
	SignUpID string
	Name     string
}

type session struct {
	ID     string
	URL    string
	Status string
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	mux.Handle("POST /api/v1/identity-proofing/webhook", httpx.Handle(func(w http.ResponseWriter, r *http.Request) error { return webhook(w, r, d.Store) }))
	if d.Jobs != nil {
		d.Jobs.Every("poll identity checks", 10*time.Minute, func(ctx context.Context) error { return pollAll(ctx, d.Store) })
	}
	return nil
}

func Name(s store.Settings) string {
	if s.IdentityProofingProvider == "stripe" {
		return "Stripe Identity"
	}
	return s.IdentityProofingName
}

func SecureURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func Begin(ctx context.Context, st *store.Store, sub Subject, returnURL, ip string) (string, error) {
	s, err := st.Settings(ctx)
	if err != nil {
		return "", err
	}
	if s.IdentityProofingProvider == "" {
		return "", errNoProvider
	}
	if n, _, err := st.CountRequest(ctx, sub.UserID+sub.SignUpID, "identity-proof", startLimit, time.Hour); err != nil {
		return "", err
	} else if n > startLimit {
		return "", errTooMany
	}
	apiKey, _, err := st.ProofingSecrets(ctx)
	if err != nil {
		return "", err
	}
	started, err := create(ctx, s, apiKey, returnURL)
	if err != nil {
		slog.WarnContext(ctx, "identity check not started", "provider", s.IdentityProofingProvider, "error", err)
		return "", errUnavailable
	}
	err = st.Tx(ctx, func(tx *store.Store) error {
		if _, err := tx.CreateIdentityProof(ctx, store.NewIdentityProof{UserID: sub.UserID, SignUpID: sub.SignUpID, Method: "provider", Provider: s.IdentityProofingProvider, Reference: started.ID, Status: "pending"}); err != nil {
			return err
		}
		e := store.AuditEvent{Action: "user.identity_proof.start", Summary: "Started an identity check with " + Name(s), TargetType: "sign_up", TargetID: sub.SignUpID, TargetLabel: sub.Name, IP: ip}
		if sub.UserID != "" {
			e.ActorID, e.TargetType, e.TargetID = &sub.UserID, "user", sub.UserID
		}
		return tx.RecordAudit(ctx, e)
	})
	return started.URL, err
}

func Refresh(ctx context.Context, st *store.Store, p store.IdentityProof) (store.IdentityProof, error) {
	s, err := st.Settings(ctx)
	if err != nil || p.Status != "pending" || p.Method != "provider" || p.Provider != s.IdentityProofingProvider {
		return p, err
	}
	if n, _, err := st.CountRequest(ctx, p.ID, "identity-proof-poll", pollLimit, time.Hour); err != nil || n > pollLimit {
		return p, err
	}
	apiKey, _, err := st.ProofingSecrets(ctx)
	if err != nil {
		return p, err
	}
	status, err := check(ctx, s, apiKey, p.Reference)
	if err != nil {
		slog.WarnContext(ctx, "identity check not polled", "proof", p.ID, "error", err)
		return p, nil
	}
	if status == "pending" {
		return p, nil
	}
	done, err := complete(ctx, st, s, p.Reference, status)
	if errors.Is(err, store.ErrNotFound) {
		return p, nil
	}
	return done, err
}

func pollAll(ctx context.Context, st *store.Store) error {
	pending, err := st.PendingIdentityProofs(ctx)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if _, err := Refresh(ctx, st, p); err != nil {
			return err
		}
	}
	return nil
}

func complete(ctx context.Context, st *store.Store, s store.Settings, reference, status string) (store.IdentityProof, error) {
	var p store.IdentityProof
	err := st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if p, err = tx.CompleteIdentityProof(ctx, s.IdentityProofingProvider, reference, status); err != nil {
			return err
		}
		summary := Name(s) + " verified the identity of " + p.Subject
		if status == "failed" {
			summary = Name(s) + " could not verify the identity of " + p.Subject
		}
		e := store.AuditEvent{Action: "user.identity_proof.result", Summary: summary, TargetType: "sign_up", TargetLabel: p.Subject}
		if p.UserID != nil {
			e.TargetType, e.TargetID = "user", *p.UserID
		} else if p.SignUpID != nil {
			e.TargetID = *p.SignUpID
		}
		return tx.RecordAudit(ctx, e)
	})
	return p, err
}

func create(ctx context.Context, s store.Settings, apiKey, returnURL string) (session, error) {
	var req *http.Request
	var err error
	if s.IdentityProofingProvider == "stripe" {
		form := url.Values{"type": {"document"}, "return_url": {returnURL}}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, StripeURL+"v1/identity/verification_sessions", strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		body, _ := json.Marshal(map[string]string{"returnUrl": returnURL})
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, s.IdentityProofingEndpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	if err != nil {
		return session{}, err
	}
	started, err := call(req, s.IdentityProofingProvider, apiKey)
	if err == nil && (started.ID == "" || len(started.ID) > 200 || !SecureURL(started.URL)) {
		err = errors.New("the provider answered without a session id and an https address")
	}
	return started, err
}

func check(ctx context.Context, s store.Settings, apiKey, reference string) (string, error) {
	endpoint := strings.TrimSuffix(s.IdentityProofingEndpoint, "/") + "/" + url.PathEscape(reference)
	if s.IdentityProofingProvider == "stripe" {
		endpoint = StripeURL + "v1/identity/verification_sessions/" + url.PathEscape(reference)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	got, err := call(req, s.IdentityProofingProvider, apiKey)
	return got.Status, err
}

func call(req *http.Request, provider, apiKey string) (session, error) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := Client.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return session{}, fmt.Errorf("Halo could not reach %s: %w", req.URL.Host, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return session{}, fmt.Errorf("%s answered %d", req.URL.Host, res.StatusCode)
	}
	var body struct {
		ID        string    `json:"id"`
		URL       string    `json:"url"`
		Status    string    `json:"status"`
		LastError *struct{} `json:"last_error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&body); err != nil {
		return session{}, fmt.Errorf("%s answered with a body Halo could not read", req.URL.Host)
	}
	return session{ID: body.ID, URL: body.URL, Status: outcome(provider, body.Status, body.LastError != nil)}, nil
}

func outcome(provider, status string, lastError bool) string {
	switch {
	case status == "verified":
		return "verified"
	case provider == "stripe" && (status == "canceled" || status == "requires_input" && lastError):
		return "failed"
	case provider != "stripe" && status == "failed":
		return "failed"
	}
	return "pending"
}

var stripeEvents = map[string]string{
	"identity.verification_session.verified":       "verified",
	"identity.verification_session.requires_input": "failed",
	"identity.verification_session.canceled":       "failed",
}

func parse(provider string, body []byte) (string, string, string, error) {
	if provider == "stripe" {
		var e struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Data struct {
				Object struct {
					ID string `json:"id"`
				} `json:"object"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &e); err != nil || e.ID == "" || len(e.ID) > 200 {
			return "", "", "", httpx.Invalid("Send a Stripe event with an id.")
		}
		return e.ID, e.Data.Object.ID, stripeEvents[e.Type], nil
	}
	var e struct {
		ID      string `json:"id"`
		Session string `json:"session"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.ID == "" || len(e.ID) > 200 || e.Session == "" || e.Status != "verified" && e.Status != "failed" {
		return "", "", "", httpx.Invalid(`Send "id", "session" and a "status" of verified or failed.`)
	}
	return e.ID, e.Session, e.Status, nil
}

func webhook(w http.ResponseWriter, r *http.Request, st *store.Store) error {
	ctx := r.Context()
	s, err := st.Settings(ctx)
	if err != nil {
		return err
	}
	_, signing, err := st.ProofingSecrets(ctx)
	if errors.Is(err, store.ErrNotFound) || err == nil && (s.IdentityProofingProvider == "" || signing == "") {
		return errSignature
	}
	if err != nil {
		return err
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		return httpx.Invalid("Send at most 64 KB in one event.")
	}
	header := r.Header.Get("Halo-Signature")
	if s.IdentityProofingProvider == "stripe" {
		header = r.Header.Get("Stripe-Signature")
	}
	if !secret.VerifySignature([]byte(signing), header, body, time.Now()) {
		return errSignature
	}
	eventID, reference, status, err := parse(s.IdentityProofingProvider, body)
	if err != nil {
		return err
	}
	err = st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.UseProofingEvent(ctx, s.IdentityProofingProvider+":"+eventID, time.Now().Add(2*secret.SignatureTolerance)); err != nil || status == "" {
			return err
		}
		_, err := complete(ctx, tx, s, reference, status)
		return err
	})
	if err != nil && !errors.Is(err, store.ErrConflict) && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]bool{"received": true})
}

package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

var (
	errDeviceCode      = httpx.Fail(http.StatusNotFound, "ERR_DEVICE_CODE", "This code is wrong, has expired or was already used. Run halo login again to get a new one.")
	errDeviceAssigned  = httpx.Fail(http.StatusForbidden, "ERR_NOT_ASSIGNED", "You don't have access to this application. Ask your administrator to assign it to you.")
	errDeviceBlocked   = httpx.Fail(http.StatusForbidden, "ERR_BLOCKED_BY_POLICY", "Your organization's access policy blocked this sign-in. If you think this is wrong, contact your administrator.")
	errDeviceStrongest = httpx.Fail(http.StatusForbidden, "ERR_STRONGER_AUTH_REQUIRED", "This application requires a passkey or security key. Sign out, sign in with one of those, then approve again.")
)

func ensureCLI(ctx context.Context, st *store.Store) error {
	_, err := st.GetApplicationByClientID(ctx, CLIClientID)
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = st.CreateApplication(ctx, store.NewApplication{
		Name:        "Halo CLI",
		Description: "Built in. Signs people in to the halo command-line tool with the device flow. Every active user can use it, so group assignment does not apply.",
		Protocol:    "oidc",
		Type:        "native",
		ClientID:    CLIClientID,
		Scopes:      []string{"openid", "profile", "email", "groups", "offline_access"},
	})
	if errors.Is(err, store.ErrConflict) {
		return nil
	}
	return err
}

func (s *storage) StoreDeviceAuthorization(ctx context.Context, clientID, deviceCode, userCode string, expires time.Time, scopes []string) error {
	app, err := s.app(ctx, clientID)
	if err != nil {
		return err
	}
	c, err := s.client(ctx, app)
	if err != nil {
		return err
	}
	if scopes, err = op.ValidateAuthReqScopes(c, scopes); err != nil {
		return err
	}
	granted, err := s.st.GrantedResources(ctx, clientID)
	if err != nil {
		return err
	}
	if scopes, err = withResources(ctx, granted, scopes); err != nil {
		return err
	}
	d := store.DeviceAuthorization{UserCode: userCode, ClientID: clientID, Scopes: scopes, ExpiresAt: expires}
	if r, ok := ctx.Value(requestKey{}).(*http.Request); ok {
		d.IP, d.UserAgent = httpx.ClientIP(r), r.UserAgent()
	}
	err = s.st.CreateDeviceAuthorization(ctx, secret.Hash(deviceCode), d)
	if errors.Is(err, store.ErrConflict) {
		return op.ErrDuplicateUserCode
	}
	return err
}

func (s *storage) GetDeviceAuthorizatonState(ctx context.Context, clientID, deviceCode string) (*op.DeviceAuthorizationState, error) {
	d, err := s.st.PollDeviceAuthorization(ctx, secret.Hash(deviceCode), clientID, devicePollInterval)
	if errors.Is(err, store.ErrSlowDown) {
		return nil, fmt.Errorf("%w: %w", context.DeadlineExceeded, err)
	}
	if err != nil {
		return nil, err
	}
	if d.State == "used" {
		return nil, errors.New("the device code was already exchanged for tokens")
	}
	state := &op.DeviceAuthorizationState{ClientID: clientID, Scopes: d.Scopes, Expires: d.ExpiresAt, Done: d.State == "approved", Denied: d.State == "denied",
		Subject: deref(d.UserID), AMR: d.AMR, AuthTime: deref(d.AuthTime)}
	if state.Done {
		granted, err := s.st.GrantedResources(ctx, clientID)
		if err != nil {
			return nil, err
		}
		state.Audience, _ = audience(granted, clientID, d.Scopes)
	}
	return state, nil
}

type deviceHandlers struct {
	st     *store.Store
	policy access.Engine
}

type pendingDevice struct {
	user    store.User
	session store.Session
	device  store.DeviceAuthorization
	app     store.Application
}

func userCode(raw string) string {
	code := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToUpper(raw))
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}

func (h *deviceHandlers) pending(r *http.Request) (pendingDevice, error) {
	var p pendingDevice
	var signedIn bool
	p.user, _ = httpx.CurrentUser(r)
	if p.session, signedIn = httpx.CurrentSession(r); !signedIn {
		return p, httpx.ErrUnauthenticated
	}
	var err error
	if p.device, err = h.st.PendingDeviceAuthorization(r.Context(), userCode(r.PathValue("code"))); errors.Is(err, store.ErrNotFound) {
		return p, errDeviceCode
	} else if err != nil {
		return p, err
	}
	if p.app, err = usable(h.st.GetApplicationByClientID(r.Context(), p.device.ClientID)); err != nil {
		return p, errDeviceCode
	}
	return p, nil
}

func (h *deviceHandlers) show(w http.ResponseWriter, r *http.Request) error {
	p, err := h.pending(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{
		"userCode": p.device.UserCode, "application": p.app.Name, "clientId": p.app.ClientID, "scopes": p.device.Scopes,
		"ip": p.device.IP, "userAgent": p.device.UserAgent, "requestedAt": p.device.CreatedAt, "expiresAt": p.device.ExpiresAt,
	})
}

func (h *deviceHandlers) decide(approve bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		p, err := h.pending(r)
		if err != nil {
			return err
		}
		ctx := r.Context()
		_, os, browser := store.ParseUserAgent(r.UserAgent())
		event := store.SignInEvent{Result: "interrupted", Method: p.session.Method, UserID: &p.user.ID, Email: p.user.Email, AppID: &p.app.ID, IP: httpx.ClientIP(r), Device: browser + " · " + os,
			Reason: "Denied on the device confirmation page."}
		if approve {
			reason, refusal, err := h.check(r, p, &event)
			if err != nil {
				return err
			}
			if refusal != nil {
				event.Result, event.Reason = "failure", reason
				if err := h.st.RecordSignIn(ctx, event); err != nil {
					return err
				}
				return refusal
			}
			event.Result, event.Reason = "success", ""
		}
		err = h.st.Tx(ctx, func(tx *store.Store) error {
			if err := tx.DecideDeviceAuthorization(ctx, p.device.UserCode, approve, p.user.ID, p.session.Method, amr(p.session.Method), p.session.CreatedAt); err != nil {
				return err
			}
			return tx.RecordSignIn(ctx, event)
		})
		if errors.Is(err, store.ErrNotFound) {
			return errDeviceCode
		}
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"approved": approve, "application": p.app.Name})
	}
}

func (h *deviceHandlers) check(r *http.Request, p pendingDevice, event *store.SignInEvent) (string, *httpx.Error, error) {
	if !assigned(p.user, p.app) {
		return "User is not assigned to this application.", errDeviceAssigned, nil
	}
	decision, err := h.policy.Evaluate(r.Context(), access.Input{User: p.user, App: &p.app, Method: p.session.Method, IP: event.IP, UserAgent: r.UserAgent(), DeviceToken: httpx.DeviceToken(r), Session: &p.session})
	if err != nil {
		return "", nil, err
	}
	event.ID, event.Risk = decision.EventID, decision.Risk
	switch {
	case decision.Effect == access.Block:
		return policyReason(decision), errDeviceBlocked, nil
	case decision.Effect == access.RequirePhishingResistant && p.session.Method != "passkey" && p.session.Method != "security-key":
		return policyReason(decision), errDeviceStrongest, nil
	}
	return "", nil, nil
}

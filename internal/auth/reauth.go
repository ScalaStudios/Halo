package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/go-webauthn/webauthn/protocol"

	"halo/internal/httpx"
	"halo/internal/store"
)

func (h *handler) reauthSession(r *http.Request) (store.User, store.Session, []store.WebAuthnCredential, error) {
	u, sess, err := httpx.RequireUser(r)
	if err == nil && sess.ID == "" {
		err = httpx.ErrUnauthenticated
	}
	if err == nil && !u.CanReauthenticate() {
		err = httpx.ErrNoReauthMethod
	}
	if err != nil {
		return u, sess, nil, err
	}
	creds, err := h.st.ListWebAuthnCredentials(r.Context(), u.ID)
	return u, sess, creds, err
}

func (h *handler) reauthBegin(w http.ResponseWriter, r *http.Request) error {
	u, _, creds, err := h.reauthSession(r)
	if err != nil {
		return err
	}
	if len(creds) == 0 {
		return httpx.JSON(w, http.StatusOK, map[string]string{"method": "totp"})
	}
	assertion, session, err := h.wa.BeginLogin(waUser{u, creds})
	if err != nil {
		return err
	}
	ceremonyID, err := h.st.CreateCeremony(r.Context(), "passkey-login", &u.ID, ceremony{Session: session})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"method": "passkey", "ceremony": ceremonyID, "options": assertion})
}

func (h *handler) reauthenticate(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ceremony   string          `json:"ceremony"`
		Credential json.RawMessage `json:"credential"`
		Code       string          `json:"code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, sess, creds, err := h.reauthSession(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	a := attempt{method: "totp", email: u.Email, user: &u}
	if len(creds) > 0 {
		a.method = "passkey"
		var c ceremony
		owner, err := h.st.TakeCeremony(ctx, in.Ceremony, "passkey-login", &c)
		if errors.Is(err, store.ErrNotFound) || err == nil && (owner == nil || *owner != u.ID) {
			return h.reject(r, a, "The passkey prompt expired or was already used.", errPasskeyRejected)
		}
		if err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
		if err != nil {
			return h.reject(r, a, "The passkey response was malformed.", errPasskeyRejected)
		}
		cred, err := h.wa.ValidateLogin(waUser{u, creds}, *c.Session, parsed)
		if err != nil {
			return h.reject(r, a, "The passkey signature could not be verified.", errPasskeyRejected)
		}
		if cred.Authenticator.CloneWarning {
			return h.reject(r, a, "The passkey's signature counter went backwards, so it may have been cloned.", errPasskeyRejected)
		}
		stored := creds[slices.IndexFunc(creds, func(s store.WebAuthnCredential) bool { return bytes.Equal(s.CredentialID, cred.ID) })]
		a.method = stored.Kind
		if err := h.st.TouchWebAuthnCredential(ctx, stored.ID, int64(cred.Authenticator.SignCount), cred.Flags.BackupState); err != nil {
			return err
		}
	} else {
		settings, err := h.st.Settings(ctx)
		if err != nil {
			return err
		}
		release, err := h.lockout(r, a, settings, "codes")
		if err != nil {
			return err
		}
		defer release()
		reason, err := h.useTOTP(ctx, u.ID, in.Code)
		if err != nil {
			return err
		}
		if reason != "" {
			return h.reject(r, a, reason, errCodeMismatch)
		}
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.MarkReauthenticated(ctx, sess.ID); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.reauthenticate", u.Name+" confirmed it's them with "+map[string]string{"passkey": "a passkey", "security-key": "a security key", "totp": "an authenticator app"}[a.method]+" before a sensitive change")
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

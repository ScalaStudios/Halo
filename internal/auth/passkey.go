package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"halo/internal/httpx"
	"halo/internal/store"
)

type ceremony struct {
	Session *webauthn.SessionData `json:"session,omitempty"`
	Kind    string                `json:"kind,omitempty"`
	TOTPID  string                `json:"totpId,omitempty"`
}

type waUser struct {
	user  store.User
	creds []store.WebAuthnCredential
}

func (w waUser) WebAuthnID() []byte          { return []byte(w.user.ID) }
func (w waUser) WebAuthnName() string        { return w.user.Email }
func (w waUser) WebAuthnDisplayName() string { return w.user.Name }

func (w waUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, len(w.creds))
	for i, c := range w.creds {
		transports := make([]protocol.AuthenticatorTransport, len(c.Transports))
		for j, t := range c.Transports {
			transports[j] = protocol.AuthenticatorTransport(t)
		}
		out[i] = webauthn.Credential{
			ID:              c.CredentialID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Transport:       transports,
			Flags:           webauthn.CredentialFlags{BackupEligible: c.BackupEligible, BackupState: c.BackupState},
			Authenticator:   webauthn.Authenticator{AAGUID: c.AAGUID, SignCount: uint32(c.SignCount)},
		}
	}
	return out
}

func (h *handler) startCeremony(w http.ResponseWriter, r *http.Request, kind string, userID *string, data ceremony, options any) error {
	ceremonyID, err := h.st.CreateCeremony(r.Context(), kind, userID, data)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"ceremony": ceremonyID, "options": options})
}

func (h *handler) passkeyBegin(w http.ResponseWriter, r *http.Request) error {
	assertion, session, err := h.wa.BeginDiscoverableLogin()
	if err != nil {
		return err
	}
	return h.startCeremony(w, r, "passkey-login", nil, ceremony{Session: session}, assertion)
}

func (h *handler) passkeyFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ceremony    string          `json:"ceremony"`
		Credential  json.RawMessage `json:"credential"`
		AuthRequest string          `json:"authRequest"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.target(ctx, in.AuthRequest)
	if err != nil {
		return err
	}
	a := attempt{method: "passkey", app: app, request: in.AuthRequest}
	var c ceremony
	if _, err := h.st.TakeCeremony(ctx, in.Ceremony, "passkey-login", &c); errors.Is(err, store.ErrNotFound) {
		return h.reject(r, a, "The passkey prompt expired or was already used.", errPasskeyRejected)
	} else if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
	if err != nil {
		return h.reject(r, a, "The passkey response was malformed.", errPasskeyRejected)
	}
	var stored store.WebAuthnCredential
	_, cred, err := h.wa.ValidatePasskeyLogin(func(rawID, _ []byte) (webauthn.User, error) {
		found, err := h.st.WebAuthnCredentialByCredentialID(ctx, rawID)
		if err != nil {
			return nil, err
		}
		u, err := h.st.GetUser(ctx, found.UserID)
		if err != nil {
			return nil, err
		}
		stored, a.user, a.method = found, &u, found.Kind
		return waUser{u, []store.WebAuthnCredential{found}}, nil
	}, *c.Session, parsed)
	if err != nil {
		reason := "The passkey is not registered with Halo."
		if a.user != nil {
			reason = "The passkey signature could not be verified."
		}
		return h.reject(r, a, reason, errPasskeyRejected)
	}
	if cred.Authenticator.CloneWarning {
		return h.reject(r, a, "The passkey's signature counter went backwards, so it may have been cloned.", errPasskeyRejected)
	}
	if err := h.st.TouchWebAuthnCredential(ctx, stored.ID, int64(cred.Authenticator.SignCount), cred.Flags.BackupState); err != nil {
		return err
	}
	return h.finish(w, r, a, nil, nil)
}

func (h *handler) registerBegin(w http.ResponseWriter, r *http.Request, u store.User, kind string) error {
	creds, err := h.st.ListWebAuthnCredentials(r.Context(), u.ID)
	if err != nil {
		return err
	}
	user := waUser{u, creds}
	selection := protocol.AuthenticatorSelection{
		ResidentKey:        protocol.ResidentKeyRequirementRequired,
		RequireResidentKey: protocol.ResidentKeyRequired(),
		UserVerification:   protocol.VerificationPreferred,
	}
	if kind == "security-key" {
		selection.AuthenticatorAttachment = protocol.CrossPlatform
	}
	creation, session, err := h.wa.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(selection),
		webauthn.WithExclusions(webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()))
	if err != nil {
		return err
	}
	return h.startCeremony(w, r, "passkey-register", &u.ID, ceremony{Session: session, Kind: kind}, creation)
}

func (h *handler) registerFinish(r *http.Request, u store.User, ceremonyID string, raw json.RawMessage, label string) (store.WebAuthnCredential, error) {
	var c ceremony
	owner, err := h.st.TakeCeremony(r.Context(), ceremonyID, "passkey-register", &c)
	if errors.Is(err, store.ErrNotFound) || err == nil && (owner == nil || *owner != u.ID) {
		return store.WebAuthnCredential{}, errCeremonyExpired
	}
	if err != nil {
		return store.WebAuthnCredential{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return store.WebAuthnCredential{}, errRegistration
	}
	cred, err := h.wa.CreateCredential(waUser{user: u}, *c.Session, parsed)
	if err != nil {
		return store.WebAuthnCredential{}, errRegistration
	}
	transports := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		transports[i] = string(t)
	}
	kind := c.Kind
	if kind == "" {
		kind = credentialKind(cred)
	}
	if label == "" {
		label = defaultLabel(kind, r.UserAgent())
	}
	return store.WebAuthnCredential{
		UserID:          u.ID,
		Kind:            kind,
		Label:           label,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       int64(cred.Authenticator.SignCount),
		Transports:      transports,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
	}, nil
}

func credentialKind(cred *webauthn.Credential) string {
	if !cred.Flags.BackupEligible {
		for _, t := range cred.Transport {
			if t == protocol.USB || t == protocol.NFC || t == protocol.BLE {
				return "security-key"
			}
		}
	}
	return "passkey"
}

func (h *handler) enrollee(r *http.Request, token string) (store.User, string, error) {
	u, purpose, err := h.st.PeekEnrollmentToken(r.Context(), token)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return u, "", errLinkExpired
	case err == nil && (u.Status == "suspended" || u.Status == "deprovisioned"):
		return u, "", errSuspended
	}
	return u, purpose, err
}

func (h *handler) enrollInfo(w http.ResponseWriter, r *http.Request) error {
	u, purpose, err := h.enrollee(r, r.URL.Query().Get("token"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"user": map[string]string{"name": u.Name, "email": u.Email}, "purpose": purpose})
}

func (h *handler) enrollBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := h.enrollee(r, in.Token)
	if err != nil {
		return err
	}
	return h.registerBegin(w, r, u, "")
}

func (h *handler) enrollFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token      string          `json:"token"`
		Ceremony   string          `json:"ceremony"`
		Credential json.RawMessage `json:"credential"`
		Label      string          `json:"label"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := h.enrollee(r, in.Token)
	if err != nil {
		return err
	}
	cred, err := h.registerFinish(r, u, in.Ceremony, in.Credential, in.Label)
	if err != nil {
		return err
	}
	ctx := r.Context()
	return h.finish(w, r, attempt{method: cred.Kind, user: &u}, nil, func(tx *store.Store) error {
		_, emailed, err := tx.ConsumeEnrollmentToken(ctx, in.Token)
		if errors.Is(err, store.ErrNotFound) {
			return errLinkExpired
		}
		if err != nil {
			return err
		}
		if emailed {
			if err := tx.MarkEmailVerified(ctx, u.ID); err != nil {
				return err
			}
		}
		if _, err := tx.AddWebAuthnCredential(ctx, cred); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.enroll", u.Name+" enrolled "+cred.Label)
	})
}

func (h *handler) addPasskeyBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Kind string `json:"kind"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	if in.Kind != "passkey" && in.Kind != "security-key" {
		return httpx.Invalid("Kind must be passkey or security-key.")
	}
	return h.registerBegin(w, r, u, in.Kind)
}

func (h *handler) addPasskeyFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ceremony   string          `json:"ceremony"`
		Credential json.RawMessage `json:"credential"`
		Label      string          `json:"label"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	cred, err := h.registerFinish(r, u, in.Ceremony, in.Credential, in.Label)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var m store.Method
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if m, err = tx.AddWebAuthnCredential(ctx, cred); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.method.add", u.Name+" added "+cred.Label)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, m)
}

package auth

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/metadata"
	"github.com/go-webauthn/webauthn/metadata/providers/memory"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	fetchMetadata          = metadata.Fetch
	errMetadataUnavailable = httpx.Fail(http.StatusServiceUnavailable, "ERR_METADATA_UNAVAILABLE", "Halo couldn't reach the FIDO Metadata Service to check this key's make and model. Try again in a few minutes.")
)

type trustList struct {
	mu     sync.Mutex
	loaded time.Time
	mds    metadata.Provider
}

func (t *trustList) provider(ctx context.Context) (metadata.Provider, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.mds != nil && time.Since(t.loaded) < 24*time.Hour {
		return t.mds, nil
	}
	m, err := fetchMetadata()
	if err == nil {
		var p metadata.Provider
		if p, err = memory.New(memory.WithMetadata(m.ToMap())); err == nil {
			t.mds = p
		}
	}
	if err != nil {
		slog.WarnContext(ctx, "fido metadata service unavailable", "error", err)
		if t.mds == nil {
			return nil, errMetadataUnavailable
		}
	}
	t.loaded = time.Now()
	return t.mds, nil
}

func requiresAttestation(rules []store.AuthenticatorRule) bool {
	return slices.ContainsFunc(rules, func(rule store.AuthenticatorRule) bool { return rule.Attestation })
}

func attestedType(t string) bool {
	return t != "" && t != string(metadata.None) && t != string(metadata.BasicSurrogate)
}

func aaguidString(b []byte) string {
	id, _ := uuid.FromBytes(b)
	return id.String()
}

func breaks(rule store.AuthenticatorRule, synced, attested bool, aaguid string) string {
	switch {
	case rule.DeviceBound && synced:
		return "can only add a security key or a passkey that stays on one device. This passkey can sync to your other devices, for example through iCloud Keychain or Google Password Manager."
	case rule.Attestation && !attested:
		return "can only add a key whose make and model Halo can confirm with the FIDO Metadata Service, and this one couldn't be confirmed. If your browser asks, allow Halo to see your key's make and model, or use a key your administrator approved."
	case len(rule.AAGUIDs) > 0 && !slices.Contains(rule.AAGUIDs, aaguid):
		return "can only add approved key models, and this one isn't on the list. Ask your administrator which keys are approved."
	}
	return ""
}

func (h *handler) checkAuthenticator(ctx context.Context, u store.User, cred *webauthn.Credential) error {
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	rules := settings.AuthenticatorRulesFor(u)
	attested := false
	if requiresAttestation(rules) && attestedType(cred.AttestationType) {
		mds, err := h.trust.provider(ctx)
		if err != nil {
			return err
		}
		attested = cred.Verify(mds, protocol.AttestationPolicy{}, protocol.SignaturePolicy{}) == nil
	}
	for _, rule := range rules {
		if problem := breaks(rule, cred.Flags.BackupEligible, attested, aaguidString(cred.Authenticator.AAGUID)); problem != "" {
			g, err := h.st.GetGroup(ctx, rule.GroupID)
			if err != nil {
				return err
			}
			return httpx.Fail(http.StatusForbidden, "ERR_AUTHENTICATOR_NOT_ALLOWED", "Members of "+g.Name+" "+problem)
		}
	}
	return nil
}

func (h *handler) compliantKeys(ctx context.Context, u store.User, rules []store.AuthenticatorRule) ([]string, error) {
	creds, err := h.st.ListWebAuthnCredentials(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, c := range creds {
		if !slices.ContainsFunc(rules, func(rule store.AuthenticatorRule) bool {
			return breaks(rule, c.BackupEligible, attestedType(c.AttestationType), aaguidString(c.AAGUID)) != ""
		}) {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

func minimumKeys(rules []store.AuthenticatorRule) int {
	n := 0
	for _, rule := range rules {
		n = max(n, rule.Minimum)
	}
	return n
}

func keyCount(n int) string {
	if n == 1 {
		return "1 passkey or security key"
	}
	return strconv.Itoa(n) + " passkeys or security keys"
}

func (h *handler) myPasskeyRules(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	rules := settings.AuthenticatorRulesFor(u)
	out := struct {
		Groups         []string `json:"groups"`
		DeviceBound    bool     `json:"deviceBound"`
		Attestation    bool     `json:"attestation"`
		ApprovedModels bool     `json:"approvedModels"`
		Minimum        int      `json:"minimum"`
		Keys           int      `json:"keys"`
	}{Groups: []string{}, Attestation: requiresAttestation(rules), Minimum: minimumKeys(rules)}
	for _, rule := range rules {
		g, err := h.st.GetGroup(ctx, rule.GroupID)
		if err != nil {
			return err
		}
		out.Groups = append(out.Groups, g.Name)
		out.DeviceBound = out.DeviceBound || rule.DeviceBound
		out.ApprovedModels = out.ApprovedModels || len(rule.AAGUIDs) > 0
	}
	keys, err := h.compliantKeys(ctx, u, rules)
	if err != nil {
		return err
	}
	out.Keys = len(keys)
	return httpx.JSON(w, http.StatusOK, out)
}

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	oidcclient "github.com/zitadel/oidc/v3/pkg/client"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	workloadTokenLifetime = time.Hour
	workloadClockSkew     = time.Minute
)

var (
	workloadClient     = &http.Client{Timeout: 10 * time.Second}
	workloadAlgorithms = []jose.SignatureAlgorithm{jose.RS256, jose.RS384, jose.RS512, jose.PS256, jose.PS384, jose.PS512, jose.ES256, jose.ES384, jose.ES512}
	issuerKeyLifetime  = time.Hour
	issuerKeyRefetch   = time.Minute
)

type issuerKeys struct {
	sync.Mutex
	keys    []jose.JSONWebKey
	fetched time.Time
}

type workloads struct {
	st      *store.Store
	dev     bool
	mu      sync.Mutex
	issuers map[string]*issuerKeys
}

func (h *workloads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, scopes, err := h.exchange(r)
	if err != nil {
		var oidcErr *oidc.Error
		if !errors.As(err, &oidcErr) {
			slog.ErrorContext(r.Context(), "workload identity exchange failed", "error", err)
			err = oidc.ErrServerError().WithDescription("Halo could not complete the exchange. Try again; if it keeps failing, check the server log.")
		}
		op.RequestError(w, r, err, nil)
		return
	}
	_ = httpx.JSON(w, http.StatusOK, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int(workloadTokenLifetime.Seconds()), "scope": strings.Join(scopes, " ")})
}

func (h *workloads) exchange(r *http.Request) (string, []string, error) {
	ctx := r.Context()
	accountID, assertion := r.PostForm.Get("client_id"), r.PostForm.Get("client_assertion")
	if accountID == "" || assertion == "" {
		return "", nil, oidc.ErrInvalidRequest().WithDescription("Send client_id with the service account ID and client_assertion with the workload's token in the form body.")
	}
	account, err := h.st.GetServiceAccount(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil, oidc.ErrInvalidClient().WithDescription("client_id is not the ID of a service account.")
	}
	if err != nil {
		return "", nil, err
	}
	tok, err := jwt.ParseSigned(assertion, workloadAlgorithms)
	var unverified jwt.Claims
	if err != nil || tok.UnsafeClaimsWithoutVerification(&unverified) != nil {
		return "", nil, oidc.ErrInvalidClient().WithDescription("client_assertion must be a JWT signed with RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384 or ES512.")
	}
	candidates := slices.DeleteFunc(account.FederatedCredentials, func(c store.FederatedCredential) bool { return c.Issuer != unverified.Issuer })
	if len(candidates) == 0 {
		return "", nil, oidc.ErrInvalidClient().WithDescription("%s does not trust tokens from the issuer %q. Add a federated credential for it on the service account's page.", account.Name, unverified.Issuer)
	}
	var claims jwt.Claims
	raw := map[string]any{}
	if err := h.verify(ctx, unverified.Issuer, tok, &claims, &raw); err != nil {
		return "", nil, err
	}
	candidates = slices.DeleteFunc(candidates, func(c store.FederatedCredential) bool { return c.Subject != claims.Subject })
	if len(candidates) == 0 {
		return "", nil, oidc.ErrInvalidClient().WithDescription("%s has no federated credential for the subject %q from %s.", account.Name, claims.Subject, claims.Issuer)
	}

	fail := func(credential, description string, args ...any) error {
		e := oidc.ErrInvalidClient().WithDescription(description, args...)
		if err := h.st.RecordSignIn(ctx, signIn(r, account, credential, "failure", e.Description)); err != nil {
			return err
		}
		return e
	}
	now := time.Now()
	if claims.Expiry == nil || claims.IssuedAt == nil {
		return "", nil, fail(claims.Issuer, "The token has no exp or iat claim. Halo accepts only tokens that expire.")
	}
	switch err := claims.ValidateWithLeeway(jwt.Expected{Time: now}, workloadClockSkew); {
	case errors.Is(err, jwt.ErrExpired):
		return "", nil, fail(claims.Issuer, "The token expired at %s.", claims.Expiry.Time().UTC().Format(time.RFC3339))
	case errors.Is(err, jwt.ErrNotValidYet):
		return "", nil, fail(claims.Issuer, "The token is not valid before %s.", claims.NotBefore.Time().UTC().Format(time.RFC3339))
	case err != nil:
		return "", nil, fail(claims.Issuer, "The token was issued in the future, at %s. Check the clock of its issuer.", claims.IssuedAt.Time().UTC().Format(time.RFC3339))
	}
	expected := []string{}
	for _, c := range candidates {
		expected = append(expected, c.Audience)
	}
	candidates = slices.DeleteFunc(candidates, func(c store.FederatedCredential) bool { return !claims.Audience.Contains(c.Audience) })
	if len(candidates) == 0 {
		return "", nil, fail(claims.Issuer, "The token's audience %q does not include %s.", []string(claims.Audience), strings.Join(expected, " or "))
	}
	i := slices.IndexFunc(candidates, func(c store.FederatedCredential) bool { return mismatch(c.Conditions, raw) == "" })
	if i < 0 {
		c := candidates[0]
		return "", nil, fail(c.Name, "The token's %s claim does not have the value that the federated credential “%s” requires.", mismatch(c.Conditions, raw), c.Name)
	}
	credential := candidates[i]
	if account.Status != "active" {
		return "", nil, fail(credential.Name, "%s is disabled. Enable it in Halo before workloads sign in as it.", account.Name)
	}

	var token string
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		if token, err = tx.IssueFederatedToken(ctx, credential, now.Add(workloadTokenLifetime)); err != nil {
			return err
		}
		if err := tx.RecordSignIn(ctx, signIn(r, account, credential.Name, "success", "")); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &account.ID, Action: "federated_credential.exchange", TargetType: "service_account", TargetID: account.ID, TargetLabel: account.Name, IP: httpx.ClientIP(r),
			Summary: "Signed in with federated credential “" + credential.Name + "” as " + claims.Subject + " from " + claims.Issuer + " and got a token with scope " + strings.Join(credential.Scopes, " and ") + " for 1 hour"})
	})
	return token, credential.Scopes, err
}

func (h *workloads) verify(ctx context.Context, issuer string, tok *jwt.JSONWebToken, claims ...any) error {
	kid, alg := tok.Headers[0].KeyID, tok.Headers[0].Algorithm
	for _, stale := range []bool{false, true} {
		keys, err := h.keys(ctx, issuer, stale)
		if err != nil {
			slog.WarnContext(ctx, "workload identity issuer unreachable", "issuer", issuer, "error", err)
			return oidc.ErrServerError().WithDescription("Halo could not fetch the signing keys of %s. Try again in a few minutes.", issuer)
		}
		key, err := oidc.FindMatchingKey(kid, oidc.KeyUseSignature, alg, keys...)
		if err != nil {
			continue
		}
		if err := tok.Claims(key.Key, claims...); errors.Is(err, jose.ErrCryptoFailure) {
			return oidc.ErrInvalidClient().WithDescription("The signature of client_assertion does not match the signing key %q of %s.", kid, issuer)
		} else if err != nil {
			return oidc.ErrInvalidClient().WithDescription("client_assertion has claims of the wrong type.")
		}
		return nil
	}
	return oidc.ErrInvalidClient().WithDescription("client_assertion is signed with a key that %s does not publish.", issuer)
}

func (h *workloads) keys(ctx context.Context, issuer string, stale bool) ([]jose.JSONWebKey, error) {
	h.mu.Lock()
	k := h.issuers[issuer]
	if k == nil {
		k = &issuerKeys{}
		h.issuers[issuer] = k
	}
	h.mu.Unlock()
	k.Lock()
	defer k.Unlock()
	age := time.Since(k.fetched)
	if !k.fetched.IsZero() && age < issuerKeyLifetime && (!stale || age < issuerKeyRefetch) {
		return k.keys, nil
	}
	discovery, err := oidcclient.Discover(ctx, issuer, workloadClient)
	if err != nil {
		return nil, err
	}
	if !httpx.ValidIssuer(discovery.JwksURI, h.dev) {
		return nil, fmt.Errorf("jwks_uri %q is not an https URL", discovery.JwksURI)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discovery.JwksURI, nil)
	if err != nil {
		return nil, err
	}
	var set jose.JSONWebKeySet
	if err := httphelper.HttpRequest(workloadClient, req, &set); err != nil {
		return nil, err
	}
	k.keys, k.fetched = set.Keys, time.Now()
	return k.keys, nil
}

func mismatch(conditions map[string]string, claims map[string]any) string {
	for _, claim := range slices.Sorted(maps.Keys(conditions)) {
		if value, ok := claims[claim].(string); !ok || value != conditions[claim] {
			return claim
		}
	}
	return ""
}

func signIn(r *http.Request, account store.ServiceAccount, credential, result, reason string) store.SignInEvent {
	return store.SignInEvent{UserID: &account.ID, Email: account.Email, Result: result, Method: "federated", IP: httpx.ClientIP(r), Device: "Workload identity · " + credential, Reason: reason}
}

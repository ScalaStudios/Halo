package main

import (
	"cmp"
	"fmt"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"halo/internal/dpop"
)

const scope = "billing.read"

type accessClaims struct {
	oidc.TokenClaims
	Scope        oidc.SpaceDelimitedArray `json:"scope"`
	Confirmation struct {
		Thumbprint string `json:"jkt"`
	} `json:"cnf"`
}

type proofKey struct {
	thumbprint string
	id         string
}

type balanceAPI struct {
	audience string
	endpoint *url.URL
	issuer   string
	keys     oidc.KeySet
	mu       sync.Mutex
	proofs   map[proofKey]time.Time
}

func main() {
	issuer := strings.TrimRight(cmp.Or(os.Getenv("HALO_ISSUER"), "http://localhost:3200"), "/")
	resource, err := url.Parse(cmp.Or(os.Getenv("HALO_RESOURCE"), "http://localhost:9002"))
	if err != nil || resource.Host == "" || resource.User != nil || resource.RawQuery != "" || resource.Fragment != "" ||
		(resource.Scheme != "https" && !(resource.Scheme == "http" && slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, resource.Hostname()))) {
		log.Fatal("HALO_RESOURCE must be the API resource's HTTPS identifier or a localhost HTTP URL")
	}
	keys := rp.NewRemoteKeySet(&http.Client{Timeout: 5 * time.Second}, issuer+"/oauth2/keys")
	api := &balanceAPI{
		audience: resource.String(),
		endpoint: resource.JoinPath("balance"),
		issuer:   issuer,
		keys:     keys,
		proofs:   make(map[proofKey]time.Time),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+api.endpoint.Path, api.balance)
	server := &http.Server{Addr: "127.0.0.1:9002", Handler: mux, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Printf("Demo API listening on %s for %s", server.Addr, api.endpoint)
	log.Fatal(server.ListenAndServe())
}

func (a *balanceAPI) balance(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Values("Authorization")
	if len(auth) != 1 {
		w.Header().Set("WWW-Authenticate", `DPoP realm="billing"`)
		http.Error(w, "An access token and its DPoP proof are required.", http.StatusUnauthorized)
		return
	}
	scheme, token, _ := strings.Cut(auth[0], " ")
	if !strings.EqualFold(scheme, "DPoP") || token == "" {
		w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		http.Error(w, "Use the DPoP authorization scheme.", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	var claims accessClaims
	payload, err := oidc.ParseToken(token, &claims)
	if err != nil || claims.Issuer != a.issuer || !slices.Contains(claims.Audience, a.audience) ||
		claims.Confirmation.Thumbprint == "" || !claims.GetExpiration().After(now) ||
		oidc.CheckSignature(r.Context(), token, payload, &claims, []string{"RS256", "ES256", "EdDSA"}, a.keys) != nil ||
		(claims.NotBefore != 0 && claims.NotBefore.AsTime().After(now)) {
		w.Header().Set("WWW-Authenticate", `DPoP error="invalid_token"`)
		http.Error(w, "The access token is invalid for this API.", http.StatusUnauthorized)
		return
	}
	proof, err := dpop.Verify(r, a.endpoint, token, now)
	if err != nil || proof.Thumbprint != claims.Confirmation.Thumbprint || !a.useOnce(proof, now) {
		w.Header().Set("WWW-Authenticate", `DPoP error="invalid_dpop_proof"`)
		http.Error(w, "The access token needs a fresh proof signed by its bound key.", http.StatusUnauthorized)
		return
	}
	if !slices.Contains(claims.Scope, scope) {
		w.Header().Set("WWW-Authenticate", `DPoP error="insufficient_scope", scope="billing.read"`)
		http.Error(w, "This access token lacks the billing.read scope.", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, `{"balance_cents":4200,"currency":"USD"}`)
}

func (a *balanceAPI) useOnce(proof dpop.Proof, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	maps.DeleteFunc(a.proofs, func(_ proofKey, expires time.Time) bool { return expires.Before(now) })
	key := proofKey{thumbprint: proof.Thumbprint, id: proof.ID}
	if _, exists := a.proofs[key]; exists || len(a.proofs) >= 10_000 {
		return false
	}
	a.proofs[key] = proof.ExpiresAt
	return true
}

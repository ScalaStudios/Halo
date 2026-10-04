package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Example app</title>
<style>body{font:15px/1.6 system-ui,sans-serif;max-width:640px;margin:64px auto;padding:0 20px;background:#111;color:#fff}a{color:#fdb07c}pre{background:#0c0b0a;border:1px solid #2a2724;border-radius:8px;padding:16px;overflow:auto}</style></head>
<body>
<h1>Example app</h1>
{{if .}}<p>Signed in through Halo as <strong>{{.Name}}</strong> ({{.Email}}).</p><pre id="claims">{{.Claims}}</pre><p><a href="/">Start over</a></p>
{{else}}<p>This app trusts Halo for sign-in.</p><p><a id="sign-in" href="/login">Sign in with Halo</a></p>{{end}}
</body></html>`))

type view struct {
	Name, Email, Claims string
}

func main() {
	issuer := env("HALO_ISSUER", "http://localhost:3200")
	redirect := env("REDIRECT_URI", "http://localhost:9000/callback")
	listen := env("LISTEN", "localhost:9000")
	clientID, clientSecret := os.Getenv("CLIENT_ID"), os.Getenv("CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("set CLIENT_ID and CLIENT_SECRET from the Halo application you registered")
	}

	key := random(32)
	cookies := httphelper.NewCookieHandler(key, key, httphelper.WithUnsecure())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := rp.NewRelyingPartyOIDC(ctx, issuer, clientID, clientSecret, redirect,
		[]string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, "groups"},
		rp.WithCookieHandler(cookies), rp.WithPKCE(cookies), rp.WithVerifierOpts(rp.WithIssuedAtOffset(5*time.Second)))
	if err != nil {
		log.Fatalf("discover %s: %v", issuer, err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_ = page.Execute(w, nil)
	})
	http.Handle("/login", rp.AuthURLHandler(func() string { return base64.RawURLEncoding.EncodeToString(random(16)) }, provider))
	http.Handle("/callback", rp.CodeExchangeHandler(rp.UserinfoCallback(func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, _ rp.RelyingParty, info *oidc.UserInfo) {
		claims, _ := json.MarshalIndent(tokens.IDTokenClaims, "", "  ")
		_ = page.Execute(w, view{Name: info.Name, Email: info.Email, Claims: string(claims)})
	}), provider))

	log.Printf("example app on http://%s, signing in through %s", listen, issuer)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

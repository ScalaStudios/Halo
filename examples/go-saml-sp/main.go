package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/crewjam/saml"
)

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Example SAML app</title>
<style>body{font:15px/1.6 system-ui,sans-serif;max-width:640px;margin:64px auto;padding:0 20px;background:#111;color:#fff}a{color:#fdb07c}pre{background:#0c0b0a;border:1px solid #2a2724;border-radius:8px;padding:16px;overflow:auto}</style></head>
<body>
<h1>Example SAML app</h1>
<p>Signed in through Halo as <strong id="subject">{{.Subject}}</strong>.</p>
<p>Groups: <span id="groups">{{range $i, $g := .Groups}}{{if $i}}, {{end}}{{$g}}{{end}}</span></p>
<pre id="attributes">{{.Attributes}}</pre>
<p><a href="/">Sign in again</a></p>
</body></html>`))

type view struct {
	Subject, Attributes string
	Groups              []string
}

func main() {
	halo := env("HALO_URL", "http://localhost:3200")
	base, err := url.Parse(env("BASE_URL", "http://localhost:9100"))
	if err != nil {
		log.Fatalf("BASE_URL: %v", err)
	}
	res, err := http.Get(halo + "/saml/metadata")
	if err != nil {
		log.Fatalf("fetch Halo metadata: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	idp := &saml.EntityDescriptor{}
	if err := xml.Unmarshal(raw, idp); err != nil {
		log.Fatalf("read %s/saml/metadata: %v", halo, err)
	}
	sp := &saml.ServiceProvider{EntityID: os.Getenv("ENTITY_ID"), MetadataURL: *base.JoinPath("saml/metadata"), AcsURL: *base.JoinPath("saml/acs"), IDPMetadata: idp}
	var pending sync.Map

	http.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		req, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		relayState := random()
		pending.Store(relayState, req.ID)
		redirect, err := req.Redirect(relayState, sp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, redirect.String(), http.StatusFound)
	})
	http.HandleFunc("POST /saml/acs", func(w http.ResponseWriter, r *http.Request) {
		requestID, _ := pending.LoadAndDelete(r.FormValue("RelayState"))
		id, _ := requestID.(string)
		assertion, err := sp.ParseResponse(r, []string{id})
		if err != nil {
			http.Error(w, "Rejected the SAML response from Halo: "+err.Error(), http.StatusForbidden)
			return
		}
		attributes := map[string][]string{}
		for _, statement := range assertion.AttributeStatements {
			for _, a := range statement.Attributes {
				for _, v := range a.Values {
					attributes[a.Name] = append(attributes[a.Name], v.Value)
				}
			}
		}
		pretty, _ := json.MarshalIndent(attributes, "", "  ")
		_ = page.Execute(w, view{Subject: assertion.Subject.NameID.Value, Groups: attributes["groups"], Attributes: string(pretty)})
	})
	http.HandleFunc("GET /saml/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		_ = xml.NewEncoder(w).Encode(sp.Metadata())
	})

	listen := env("LISTEN", "localhost:9100")
	log.Printf("example SAML app on %s (entity ID %s), signing in through %s", base, sp.Metadata().EntityID, halo)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func random() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

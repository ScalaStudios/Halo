package compliance

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

func setup(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	authn := &httpx.Auth{Store: st, Public: public, Dev: true}
	mux := http.NewServeMux()
	if err := Register(mux, httpx.Deps{Store: st, Auth: authn}); err != nil {
		t.Fatal(err)
	}
	return st, authn.Resolve(authn.SameOrigin(mux))
}

func person(t *testing.T, st *store.Store, email string, roles ...string) (store.User, string) {
	t.Helper()
	u, err := st.CreateUser(context.Background(), store.NewUser{Email: email, Name: strings.Split(email, "@")[0], Status: "active", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := st.CreateSession(context.Background(), store.NewSession{UserID: u.ID, Method: "passkey"})
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	var v T
	if rec.Code != status || json.Unmarshal(rec.Body.Bytes(), &v) != nil {
		t.Fatalf("want %d, got %d: %s", status, rec.Code, rec.Body)
	}
	return v
}

func fakeGraph(t *testing.T) (*atomic.Int32, *atomic.Bool) {
	t.Helper()
	var hits atomic.Int32
	var retired atomic.Bool
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case r.URL.Path == "/contoso.onmicrosoft.com/oauth2/v2.0/token":
			if r.FormValue("client_secret") != "s3cret" || r.FormValue("scope") != srv.URL+"/.default" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided.\r\nTrace ID: 1"}`))
				return
			}
			w.Write([]byte(`{"access_token":"graph-token"}`))
		case r.Header.Get("Authorization") != "Bearer graph-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/v1.0/deviceManagement/managedDevices":
			devices := []map[string]any{{"id": "mdm-2", "deviceName": "ADA-PC", "userPrincipalName": "ada@example.com", "complianceState": "noncompliant", "operatingSystem": "Windows", "osVersion": "10.0.26100"}}
			if !retired.Load() {
				devices = append(devices, map[string]any{"id": "mdm-1", "deviceName": "ADA-MBP", "userPrincipalName": "ada@example.com", "complianceState": "compliant", "operatingSystem": "macOS", "osVersion": "15.1", "isEncrypted": true})
			}
			json.NewEncoder(w).Encode(map[string]any{"value": devices, "@odata.nextLink": srv.URL + "/v1.0/page2"})
		case r.URL.Path == "/v1.0/page2":
			w.Write([]byte(`{"value":[{"id":"mdm-3","deviceName":"BOB-PC","userPrincipalName":"bob@example.com","complianceState":"configManager"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	loginURL, graphURL = srv.URL+"/", srv.URL+"/"
	t.Cleanup(func() { loginURL, graphURL = "https://login.microsoftonline.com/", "https://graph.microsoft.com/" })
	return &hits, &retired
}

func TestIntuneSource(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	sam, security := person(t, st, "sam@example.com", "security_admin")
	_, auditor := person(t, st, "aud@example.com", "auditor")
	hits, retired := fakeGraph(t)
	valid := map[string]any{"name": "Intune", "kind": "intune", "tenantId": "contoso.onmicrosoft.com", "clientId": "0f6a-halo", "clientSecret": "s3cret"}

	if rec := call(t, h, auditor, "POST", "/api/v1/compliance-sources", valid); rec.Code != 403 {
		t.Fatalf("auditor create: %d", rec.Code)
	}
	for _, bad := range []map[string]any{
		{"name": "Intune", "kind": "jamf"},
		{"name": "Intune", "kind": "intune", "tenantId": "contoso/../evil", "clientId": "halo", "clientSecret": "s3cret"},
		{"name": "Intune", "kind": "intune", "tenantId": "contoso", "clientId": "halo"},
		{"name": "Intune", "kind": "intune", "tenantId": "contoso", "clientId": "halo", "clientSecret": "s3cret", "maxAgeMinutes": 5},
	} {
		if rec := call(t, h, security, "POST", "/api/v1/compliance-sources", bad); rec.Code != 422 {
			t.Fatalf("invalid source %v: %d %s", bad, rec.Code, rec.Body)
		}
	}

	created := decode[saved](t, call(t, h, security, "POST", "/api/v1/compliance-sources", valid), 201)
	if created.Secret != "" || !strings.Contains(created.Source.SyncError, "private") || hits.Load() != 0 {
		t.Fatalf("the guarded client must refuse the loopback test server: %+v, %d requests", created, hits.Load())
	}
	src, err := st.GetComplianceSource(ctx, created.Source.ID)
	if err != nil || bytes.Contains(src.Sealed, []byte("s3cret")) {
		t.Fatalf("client secret stored in the clear: %v", err)
	}
	if plain, err := st.Sealer.Open(src.Sealed); err != nil || string(plain) != "s3cret" {
		t.Fatalf("sealed client secret: %q %v", plain, err)
	}

	guarded := outbound
	outbound = http.DefaultClient
	t.Cleanup(func() { outbound = guarded })
	synced := decode[saved](t, call(t, h, security, "POST", "/api/v1/compliance-sources/"+src.ID+"/sync", nil), 200)
	if synced.Source.SyncError != "" || synced.Source.Records != 3 || synced.Source.ReportedAt == nil {
		t.Fatalf("sync: %+v", synced.Source)
	}
	listed := call(t, h, auditor, "GET", "/api/v1/compliance-sources", nil)
	if listed.Code != 200 || strings.Contains(listed.Body.String(), "s3cret") || strings.Contains(listed.Body.String(), "ealed") {
		t.Fatalf("list leaks the secret: %s", listed.Body)
	}
	records := decode[[]store.ComplianceRecord](t, call(t, h, auditor, "GET", "/api/v1/compliance-sources/"+src.ID+"/devices?q=ADA-pc", nil), 200)
	if len(records) != 1 || records[0].ExternalID != "mdm-2" || records[0].State != "non-compliant" || records[0].OSVersion != "Windows 10.0.26100" || records[0].Encrypted != nil {
		t.Fatalf("records: %+v", records)
	}
	if bob, err := st.GetComplianceRecord(ctx, src.ID, "mdm-3"); err != nil || bob.State != "unknown" {
		t.Fatalf("configManager state: %+v %v", bob, err)
	}
	retired.Store(true)
	decode[saved](t, call(t, h, security, "POST", "/api/v1/compliance-sources/"+src.ID+"/sync", nil), 200)
	if gone, err := st.GetComplianceRecord(ctx, src.ID, "mdm-1"); err != nil || gone.State != "unknown" {
		t.Fatalf("a device missing from Intune must become unknown: %+v %v", gone, err)
	}
	if kept, err := st.GetComplianceRecord(ctx, src.ID, "mdm-2"); err != nil || kept.State != "non-compliant" {
		t.Fatalf("a device still in Intune keeps its state: %+v %v", kept, err)
	}

	deviceID, err := st.TouchDevice(ctx, sam.ID, secret.Hash("laptop"), "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0", "198.51.100.7", true)
	if err != nil {
		t.Fatal(err)
	}
	if rec := call(t, h, security, "PUT", "/api/v1/devices/"+deviceID+"/compliance", map[string]any{"sourceId": src.ID, "externalId": "mdm-404"}); rec.Code != 422 {
		t.Fatalf("link to a missing record: %d %s", rec.Code, rec.Body)
	}
	linked := decode[store.Device](t, call(t, h, security, "PUT", "/api/v1/devices/"+deviceID+"/compliance", map[string]any{"sourceId": src.ID, "externalId": "mdm-2"}), 200)
	if linked.Compliance == nil || linked.Compliance.State != "non-compliant" || linked.Compliance.Record != "ADA-PC" || linked.Compliance.Source != "Intune" {
		t.Fatalf("linked device: %+v", linked.Compliance)
	}
	audit, err := st.ListAudit(ctx, 1)
	if err != nil || audit[0].Action != "device.link" || audit[0].Summary != "Linked Firefox on Linux computer to Intune device “ADA-PC”, reported non-compliant" {
		t.Fatalf("link audit: %+v %v", audit, err)
	}

	broken := decode[saved](t, call(t, h, security, "PUT", "/api/v1/compliance-sources/"+src.ID, map[string]any{"name": "Intune", "tenantId": "contoso.onmicrosoft.com", "clientId": "0f6a-halo", "clientSecret": "wrong"}), 200)
	if broken.Source.SyncError != "Microsoft Entra refused the app credentials: AADSTS7000215: Invalid client secret provided." {
		t.Fatalf("sync error: %q", broken.Source.SyncError)
	}
	if device, _ := st.GetDevice(ctx, deviceID); device.Compliance == nil || device.Compliance.State != "non-compliant" {
		t.Fatalf("a failed sync must keep the last report: %+v", device.Compliance)
	}
	if rec := call(t, h, security, "POST", "/api/v1/compliance-sources/"+src.ID+"/reports", nil); rec.Code != 401 {
		t.Fatalf("intune source accepted a push: %d", rec.Code)
	}

	unlinked := decode[store.Device](t, call(t, h, security, "PUT", "/api/v1/devices/"+deviceID+"/compliance", map[string]any{"sourceId": nil}), 200)
	if unlinked.Compliance != nil {
		t.Fatalf("unlinked device: %+v", unlinked.Compliance)
	}
}

func signature(key string, at time.Time, body string) string {
	stamp := strconv.FormatInt(at.Unix(), 10)
	return "t=" + stamp + ",v1=" + hex.EncodeToString(secret.MAC([]byte(key), stamp+"."+body))
}

func push(h http.Handler, sourceID, header, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/v1/compliance-sources/"+sourceID+"/reports", strings.NewReader(body))
	req.Header.Set("Halo-Signature", header)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func signed(h http.Handler, key, sourceID string, at time.Time, body string) *httptest.ResponseRecorder {
	return push(h, sourceID, signature(key, at, body), body)
}

func TestAgentReports(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, security := person(t, st, "sam@example.com", "security_admin")
	created := decode[saved](t, call(t, h, security, "POST", "/api/v1/compliance-sources", map[string]any{"name": "Fleet agent", "kind": "agent", "maxAgeMinutes": 120}), 201)
	if !strings.HasPrefix(created.Secret, "hcs_") || created.Source.Kind != "agent" || created.Source.MaxAgeMinutes != 120 {
		t.Fatalf("agent source: %+v", created)
	}
	id := created.Source.ID
	report := `{"devices":[{"id":"C02XK1","name":"Ada's MacBook","owner":"ada@example.com","state":"compliant","osVersion":"macOS 15.1","diskEncrypted":true,"patchLevel":"2026-11-01"}]}`

	accepted := decode[map[string]int](t, signed(h, created.Secret, id, time.Now(), report), 200)
	if accepted["accepted"] != 1 {
		t.Fatalf("accepted: %v", accepted)
	}
	rec, err := st.GetComplianceRecord(ctx, id, "C02XK1")
	if err != nil || rec.State != "compliant" || rec.Encrypted == nil || !*rec.Encrypted || rec.PatchLevel != "2026-11-01" || rec.Owner != "ada@example.com" {
		t.Fatalf("record: %+v %v", rec, err)
	}

	for name, r := range map[string]*httptest.ResponseRecorder{
		"wrong secret":   signed(h, "hcs_wrong", id, time.Now(), report),
		"old timestamp":  signed(h, created.Secret, id, time.Now().Add(-10*time.Minute), report),
		"unknown source": signed(h, created.Secret, "cmp_missing", time.Now(), report),
	} {
		if r.Code != 401 {
			t.Fatalf("%s: %d %s", name, r.Code, r.Body)
		}
	}
	if r := push(h, id, signature(created.Secret, time.Now(), report), strings.Replace(report, `"compliant"`, `"unknown"`, 1)); r.Code != 401 {
		t.Fatalf("tampered body: %d", r.Code)
	}
	if r := signed(h, created.Secret, id, time.Now(), `{"devices":[{"id":"C02XK1","state":"managed"}]}`); r.Code != 422 {
		t.Fatalf("invalid state: %d %s", r.Code, r.Body)
	}

	rotated := decode[saved](t, call(t, h, security, "PUT", "/api/v1/compliance-sources/"+id, map[string]any{"name": "Fleet agent", "maxAgeMinutes": 120, "rotateSecret": true}), 200)
	if !strings.HasPrefix(rotated.Secret, "hcs_") || rotated.Secret == created.Secret {
		t.Fatalf("rotated secret: %+v", rotated)
	}
	if r := signed(h, created.Secret, id, time.Now(), report); r.Code != 401 {
		t.Fatalf("old secret after rotation: %d", r.Code)
	}
	if r := signed(h, rotated.Secret, id, time.Now(), report); r.Code != 200 {
		t.Fatalf("new secret after rotation: %d %s", r.Code, r.Body)
	}
	if rec := call(t, h, security, "POST", "/api/v1/compliance-sources/"+id+"/sync", nil); rec.Code != 422 {
		t.Fatalf("agent sync: %d", rec.Code)
	}
	if rec := call(t, h, security, "DELETE", "/api/v1/compliance-sources/"+id, nil); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if _, err := st.GetComplianceRecord(ctx, id, "C02XK1"); err == nil {
		t.Fatal("records must go with their source")
	}
}

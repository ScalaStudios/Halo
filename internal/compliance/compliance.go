package compliance

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

var (
	readers      = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers      = []string{"security_admin"}
	kinds        = map[string]string{"intune": "Microsoft Intune", "agent": "device agent"}
	states       = []string{"compliant", "non-compliant", "unknown"}
	identifier   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,99}$`)
	outbound     = httpx.GuardedClient()
	loginURL     = "https://login.microsoftonline.com/"
	graphURL     = "https://graph.microsoft.com/"
	errSignature = httpx.Fail(http.StatusUnauthorized, "ERR_SIGNATURE", "The Halo-Signature header is missing, older than 5 minutes or does not match the source's signing secret.")
)

type handlers struct{ st *store.Store }

type handler func(http.ResponseWriter, *http.Request, store.User) error

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store}
	route := func(pattern string, roles []string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	route("GET /api/v1/compliance-sources", readers, h.list)
	route("POST /api/v1/compliance-sources", writers, h.create)
	route("PUT /api/v1/compliance-sources/{id}", writers, h.update)
	route("DELETE /api/v1/compliance-sources/{id}", writers, h.remove)
	route("POST /api/v1/compliance-sources/{id}/sync", writers, h.syncNow)
	route("GET /api/v1/compliance-sources/{id}/devices", readers, h.records)
	route("PUT /api/v1/devices/{id}/compliance", writers, h.link)
	mux.Handle("POST /api/v1/compliance-sources/{id}/reports", httpx.Handle(h.report))
	if d.Jobs != nil {
		d.Jobs.Every("sync compliance sources", 30*time.Minute, h.syncAll)
	}
	return nil
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func (h *handlers) source(ctx context.Context, sourceID string) (store.ComplianceSource, error) {
	src, err := h.st.GetComplianceSource(ctx, sourceID)
	if errors.Is(err, store.ErrNotFound) {
		return src, httpx.NotFound("This compliance source")
	}
	return src, err
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	sources, err := h.st.ListComplianceSources(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sources)
}

type input struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	TenantID      string `json:"tenantId"`
	ClientID      string `json:"clientId"`
	ClientSecret  string `json:"clientSecret"`
	MaxAgeMinutes int    `json:"maxAgeMinutes"`
	RotateSecret  bool   `json:"rotateSecret"`
}

func (in input) validate(src store.ComplianceSource) (store.ComplianceSource, error) {
	src.Name, src.MaxAgeMinutes = strings.TrimSpace(in.Name), cmp.Or(in.MaxAgeMinutes, 1440)
	if src.Kind == "intune" {
		src.TenantID, src.ClientID = strings.TrimSpace(in.TenantID), strings.TrimSpace(in.ClientID)
	}
	switch {
	case src.Name == "" || len(src.Name) > 100:
		return src, httpx.Invalid("Name the source in up to 100 characters, for example Intune.")
	case src.MaxAgeMinutes < 60 || src.MaxAgeMinutes > 43200:
		return src, httpx.Invalid("Set how long a report stays valid to between 60 minutes and 30 days.")
	case src.Kind == "intune" && (!identifier.MatchString(src.TenantID) || !identifier.MatchString(src.ClientID)):
		return src, httpx.Invalid("Enter the directory (tenant) ID and the application (client) ID from the app registration in Microsoft Entra.")
	case src.Kind == "intune" && src.ID == "" && strings.TrimSpace(in.ClientSecret) == "":
		return src, httpx.Invalid("Enter the client secret of the app registration.")
	case len(in.ClientSecret) > 1024:
		return src, httpx.Invalid("The client secret is longer than 1,024 characters. Paste the secret value, not its ID.")
	}
	return src, nil
}

type saved struct {
	Source store.ComplianceSource `json:"source"`
	Secret string                 `json:"secret,omitempty"`
}

func (h *handlers) create(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if _, ok := kinds[in.Kind]; !ok {
		return httpx.Invalid(`Choose the kind of source: "intune" or "agent".`)
	}
	src, err := in.validate(store.ComplianceSource{Kind: in.Kind})
	if err != nil {
		return err
	}
	out := saved{}
	plain := strings.TrimSpace(in.ClientSecret)
	if src.Kind == "agent" {
		out.Secret = "hcs_" + secret.Token(24)
		plain = out.Secret
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if src.ID, err = tx.CreateComplianceSource(r.Context(), src, plain); err != nil {
			return h.taken(err, src.Name)
		}
		return record(r, tx, actor, store.AuditEvent{Action: "compliance_source.create", Summary: "Added " + kinds[src.Kind] + " compliance source", TargetType: "compliance_source", TargetID: src.ID, TargetLabel: src.Name})
	})
	if err != nil {
		return err
	}
	return h.respond(w, r.Context(), src.ID, out, http.StatusCreated)
}

func (h *handlers) taken(err error, name string) error {
	if errors.Is(err, store.ErrConflict) {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "A compliance source named "+name+" already exists. Pick a different name.")
	}
	return err
}

func (h *handlers) respond(w http.ResponseWriter, ctx context.Context, sourceID string, out saved, status int) error {
	src, err := h.st.GetComplianceSource(ctx, sourceID)
	if err != nil {
		return err
	}
	if src.Kind == "intune" {
		if err := h.sync(ctx, src); err != nil {
			return err
		}
		if src, err = h.st.GetComplianceSource(ctx, sourceID); err != nil {
			return err
		}
	}
	out.Source = src
	return httpx.JSON(w, status, out)
}

func (h *handlers) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.source(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Kind != "" && in.Kind != old.Kind {
		return httpx.Invalid("A source's kind can't change. Add a new source instead.")
	}
	src, err := in.validate(old)
	if err != nil {
		return err
	}
	out, plain := saved{}, strings.TrimSpace(in.ClientSecret)
	if src.Kind == "agent" {
		plain = ""
		if in.RotateSecret {
			out.Secret = "hcs_" + secret.Token(24)
			plain = out.Secret
		}
	}
	parts := []string{}
	if old.Name != src.Name {
		parts = append(parts, "renamed from “"+old.Name+"”")
	}
	if old.TenantID != src.TenantID || old.ClientID != src.ClientID || src.Kind == "intune" && plain != "" {
		parts = append(parts, "changed the app credentials")
	}
	if out.Secret != "" {
		parts = append(parts, "rotated the signing secret")
	}
	if old.MaxAgeMinutes != src.MaxAgeMinutes {
		parts = append(parts, fmt.Sprintf("reports now stay valid for %d minutes", src.MaxAgeMinutes))
	}
	summary := cmp.Or(strings.Join(parts, ", "), "saved without changes")
	summary = strings.ToUpper(summary[:1]) + summary[1:]
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateComplianceSource(r.Context(), src, plain); err != nil {
			return h.taken(err, src.Name)
		}
		return record(r, tx, actor, store.AuditEvent{Action: "compliance_source.update", Summary: summary, TargetType: "compliance_source", TargetID: src.ID, TargetLabel: src.Name})
	})
	if err != nil {
		return err
	}
	return h.respond(w, r.Context(), src.ID, out, http.StatusOK)
}

func (h *handlers) remove(w http.ResponseWriter, r *http.Request, actor store.User) error {
	src, err := h.source(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	summary := "Deleted " + kinds[src.Kind] + " compliance source"
	if src.Linked > 0 {
		summary += fmt.Sprintf(" and unlinked %d %s", src.Linked, map[bool]string{true: "device", false: "devices"}[src.Linked == 1])
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteComplianceSource(r.Context(), src.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "compliance_source.delete", Summary: summary, TargetType: "compliance_source", TargetID: src.ID, TargetLabel: src.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) syncNow(w http.ResponseWriter, r *http.Request, _ store.User) error {
	src, err := h.source(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	if src.Kind != "intune" {
		return httpx.Invalid("Device agents send their reports to Halo. Only Microsoft Intune sources sync.")
	}
	return h.respond(w, r.Context(), src.ID, saved{}, http.StatusOK)
}

func (h *handlers) syncAll(ctx context.Context) error {
	sources, err := h.st.ListComplianceSources(ctx)
	if err != nil {
		return err
	}
	for _, src := range sources {
		if src.Kind == "intune" {
			if err := h.sync(ctx, src); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *handlers) sync(ctx context.Context, src store.ComplianceSource) error {
	plain, err := h.st.Sealer.Open(src.Sealed)
	if err != nil {
		return err
	}
	records, err := h.intune(ctx, src, string(plain))
	if err != nil {
		message := err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		return h.st.SetComplianceSyncError(ctx, src.ID, message)
	}
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.ExternalID
	}
	return h.st.Tx(ctx, func(tx *store.Store) error {
		gone, err := tx.ComplianceRecordsExcept(ctx, src.ID, ids)
		if err != nil {
			return err
		}
		for i := range gone {
			gone[i].State = "unknown"
		}
		return tx.ReportCompliance(ctx, src, append(records, gone...), time.Now())
	})
}

func intuneState(state string) string {
	switch state {
	case "compliant":
		return "compliant"
	case "noncompliant", "conflict", "error", "inGracePeriod":
		return "non-compliant"
	}
	return "unknown"
}

func (h *handlers) call(req *http.Request, out any) (int, error) {
	res, err := outbound.Do(req)
	if err != nil {
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return 0, fmt.Errorf("Halo could not reach %s: %w", req.URL.Host, err)
	}
	defer res.Body.Close()
	err = json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out)
	if res.StatusCode == http.StatusOK && err != nil {
		return res.StatusCode, fmt.Errorf("%s answered with a body Halo could not read", req.URL.Host)
	}
	return res.StatusCode, nil
}

func (h *handlers) intune(ctx context.Context, src store.ComplianceSource, clientSecret string) ([]store.ComplianceRecord, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {src.ClientID}, "client_secret": {clientSecret}, "scope": {graphURL + ".default"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL+src.TenantID+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token struct {
		AccessToken string `json:"access_token"`
		Description string `json:"error_description"`
	}
	status, err := h.call(req, &token)
	switch {
	case err != nil:
		return nil, err
	case status != http.StatusOK || token.AccessToken == "":
		return nil, fmt.Errorf("Microsoft Entra refused the app credentials: %s", cmp.Or(strings.SplitN(token.Description, "\r\n", 2)[0], http.StatusText(status)))
	}
	records := []store.ComplianceRecord{}
	next := graphURL + "v1.0/deviceManagement/managedDevices?$select=id,deviceName,userPrincipalName,complianceState,operatingSystem,osVersion,isEncrypted"
	for page := 0; next != ""; page++ {
		if page == 500 || !strings.HasPrefix(next, graphURL) {
			return nil, errors.New("Microsoft Graph returned more pages than Halo reads, or a next page outside Microsoft Graph")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		var body struct {
			Value []struct {
				ID, DeviceName, UserPrincipalName, ComplianceState, OperatingSystem, OSVersion string
				IsEncrypted                                                                    *bool
			}
			Next string `json:"@odata.nextLink"`
		}
		status, err := h.call(req, &body)
		switch {
		case err != nil:
			return nil, err
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return nil, fmt.Errorf("Microsoft Graph answered %d. Grant the app the DeviceManagementManagedDevices.Read.All application permission and admin consent", status)
		case status != http.StatusOK:
			return nil, fmt.Errorf("Microsoft Graph answered %d", status)
		}
		for _, d := range body.Value {
			records = append(records, store.ComplianceRecord{ExternalID: d.ID, Name: d.DeviceName, Owner: d.UserPrincipalName, State: intuneState(d.ComplianceState),
				OSVersion: strings.TrimSpace(d.OperatingSystem + " " + d.OSVersion), Encrypted: d.IsEncrypted})
		}
		next = body.Next
	}
	return records, nil
}

func (h *handlers) report(w http.ResponseWriter, r *http.Request) error {
	src, err := h.st.GetComplianceSource(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || err == nil && src.Kind != "agent" {
		return errSignature
	}
	if err != nil {
		return err
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return httpx.Invalid("Send at most 1 MB in one report. Split larger reports.")
	}
	key, err := h.st.Sealer.Open(src.Sealed)
	if err != nil {
		return err
	}
	if !secret.VerifySignature(key, r.Header.Get("Halo-Signature"), body, time.Now()) {
		return errSignature
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var in struct {
		Devices []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Owner         string `json:"owner"`
			State         string `json:"state"`
			OSVersion     string `json:"osVersion"`
			DiskEncrypted *bool  `json:"diskEncrypted"`
			PatchLevel    string `json:"patchLevel"`
		} `json:"devices"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Devices) == 0 || len(in.Devices) > 1000 {
		return httpx.Invalid("Report between 1 and 1,000 devices at a time.")
	}
	records := make([]store.ComplianceRecord, len(in.Devices))
	for i, d := range in.Devices {
		if d.ID == "" || len(d.ID) > 200 || len(d.Name) > 200 || len(d.Owner) > 320 || len(d.OSVersion) > 100 || len(d.PatchLevel) > 100 || !slices.Contains(states, d.State) {
			return httpx.Invalid(fmt.Sprintf("Device %d needs an id of up to 200 characters and a state of compliant, non-compliant or unknown. Keep name under 200 characters and osVersion and patchLevel under 100.", i+1))
		}
		records[i] = store.ComplianceRecord{ExternalID: d.ID, Name: d.Name, Owner: d.Owner, State: d.State, OSVersion: d.OSVersion, Encrypted: d.DiskEncrypted, PatchLevel: d.PatchLevel}
	}
	if err := h.st.Tx(r.Context(), func(tx *store.Store) error { return tx.ReportCompliance(r.Context(), src, records, time.Now()) }); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]int{"accepted": len(records)})
}

func (h *handlers) records(w http.ResponseWriter, r *http.Request, _ store.User) error {
	src, err := h.source(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	records, err := h.st.ListComplianceRecords(r.Context(), src.ID, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, records)
}

func (h *handlers) link(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	d, err := h.st.GetDevice(ctx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound("This device")
	}
	if err != nil {
		return err
	}
	var in struct {
		SourceID   *string `json:"sourceId"`
		ExternalID string  `json:"externalId"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var externalID *string
	summary := "Unlinked " + d.Name + " from its compliance record"
	if in.SourceID != nil {
		src, err := h.st.GetComplianceSource(ctx, *in.SourceID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Invalid("That compliance source no longer exists. Refresh the page and choose again.")
		}
		if err != nil {
			return err
		}
		rec, err := h.st.GetComplianceRecord(ctx, src.ID, in.ExternalID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Invalid(src.Name + " has not reported a device with that ID. Sync the source or wait for the next report, then choose again.")
		}
		if err != nil {
			return err
		}
		externalID = &rec.ExternalID
		summary = "Linked " + d.Name + " to " + src.Name + " device “" + cmp.Or(rec.Name, rec.ExternalID) + "”, reported " + rec.State
	}
	owner, err := h.st.GetUser(ctx, d.UserID)
	if err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.LinkDeviceCompliance(ctx, d.ID, in.SourceID, externalID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "device.link", Summary: summary, TargetType: "user", TargetID: owner.ID, TargetLabel: owner.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetDevice(ctx, d.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

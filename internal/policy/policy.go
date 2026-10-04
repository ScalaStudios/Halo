package policy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

var (
	readers     = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers     = []string{"security_admin"}
	effectNames = map[string]string{"allow": "allow", "require-phishing-resistant": "require a passkey or security key", "block": "block"}
	riskTitles  = map[string]string{"risky-network": "Sign-in from a risky network", "failed-sign-ins": "Repeated failed sign-ins", "new-device": "Sign-in from a new device"}
	methodNames = strings.Join(store.MethodKinds[:len(store.MethodKinds)-1], ", ") + " or " + store.MethodKinds[len(store.MethodKinds)-1]
)

type handlers struct {
	st  *store.Store
	eng *engine
}

type handler func(http.ResponseWriter, *http.Request, store.User) error

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store, eng: &engine{st: d.Store}}
	route := func(pattern string, roles []string, fn handler) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}

	route("GET /api/v1/policies", readers, h.listPolicies)
	route("GET /api/v1/policies/{id}", readers, h.getPolicy)
	route("POST /api/v1/policies", writers, h.createPolicy)
	route("PUT /api/v1/policies/order", writers, h.reorderPolicies)
	route("PUT /api/v1/policies/{id}", writers, h.updatePolicy)
	route("DELETE /api/v1/policies/{id}", writers, h.deletePolicy)
	route("POST /api/v1/policies/simulate", readers, h.simulate)

	route("GET /api/v1/network-zones", readers, h.listZones)
	route("POST /api/v1/network-zones", writers, h.createZone)
	route("PUT /api/v1/network-zones/{id}", writers, h.updateZone)
	route("DELETE /api/v1/network-zones/{id}", writers, h.deleteZone)

	route("GET /api/v1/methods", readers, h.listMethods)
	mux.Handle("GET /api/v1/auth/methods", httpx.Handle(h.enabledMethods))
	route("PUT /api/v1/methods/{method}", writers, h.setMethod)

	route("GET /api/v1/devices", readers, h.listDevices)
	route("PUT /api/v1/devices/{id}", writers, h.setDeviceTrust)
	mux.Handle("GET /api/v1/me/devices", httpx.Handle(h.myDevices))

	route("GET /api/v1/risk-events", readers, h.listRiskEvents)
	route("POST /api/v1/risk-events/{id}/resolve", writers, h.setRiskStatus("resolved"))
	route("POST /api/v1/risk-events/{id}/dismiss", writers, h.setRiskStatus("dismissed"))

	if d.Jobs != nil {
		d.Jobs.Every("purge policy evaluations", 24*time.Hour, h.st.PurgePolicyEvaluations)
	}
	return nil
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func found(err error, what string) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.NotFound(what)
	}
	return err
}

func conflict(message string) error {
	return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", message)
}

func capitalize(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

type policyView struct {
	store.AccessPolicy
	Activity store.PolicyActivity `json:"activity"`
}

func (h *handlers) listPolicies(w http.ResponseWriter, r *http.Request, _ store.User) error {
	policies, err := h.st.ListAccessPolicies(r.Context())
	if err != nil {
		return err
	}
	activity, err := h.st.PolicyActivitySince(r.Context(), time.Now().Add(-24*time.Hour))
	if err != nil {
		return err
	}
	views := make([]policyView, len(policies))
	for i, p := range policies {
		views[i] = policyView{p, activity[p.ID]}
	}
	return httpx.JSON(w, http.StatusOK, views)
}

func (h *handlers) getPolicy(w http.ResponseWriter, r *http.Request, _ store.User) error {
	p, err := h.st.GetAccessPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This policy")
	}
	return httpx.JSON(w, http.StatusOK, p)
}

func (h *handlers) readPolicy(r *http.Request) (store.AccessPolicy, error) {
	var in struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Enabled     bool                   `json:"enabled"`
		Mode        string                 `json:"mode"`
		Effect      string                 `json:"effect"`
		Conditions  store.PolicyConditions `json:"conditions"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.AccessPolicy{}, err
	}
	p := store.AccessPolicy{Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), Enabled: in.Enabled, Mode: in.Mode, Effect: in.Effect, Conditions: in.Conditions}
	c := p.Conditions
	_, effect := severity[p.Effect]
	switch {
	case p.Name == "":
		return p, httpx.Invalid("Enter a policy name, for example “Block high-risk sign-ins”.")
	case len(p.Name) > 200 || len(p.Description) > 2000:
		return p, httpx.Invalid("Keep the name under 200 characters and the description under 2,000.")
	case p.Mode != "enforce" && p.Mode != "report":
		return p, httpx.Invalid(`Choose a mode: "enforce" applies the policy, "report" only records what it would do.`)
	case !effect:
		return p, httpx.Invalid(`Choose an effect: "allow", "require-phishing-resistant" or "block".`)
	case !c.AllUsers && len(c.GroupIDs)+len(c.UserIDs) == 0:
		return p, httpx.Invalid("Choose who the policy applies to: all users, or at least one group or person.")
	case !c.AllApps && len(c.AppIDs) == 0:
		return p, httpx.Invalid("Choose which applications the policy covers: all applications, or at least one.")
	case !slices.Contains([]string{"", "any", "in", "not-in"}, c.Network):
		return p, httpx.Invalid(`Choose a network condition: "any", "in" or "not-in".`)
	case (c.Network == "in" || c.Network == "not-in") && len(c.ZoneIDs) == 0:
		return p, httpx.Invalid("Choose at least one network for the network condition, or set it to any network.")
	case !slices.Contains([]string{"", "any", "trusted", "untrusted"}, c.Device):
		return p, httpx.Invalid(`Choose a device condition: "any", "trusted" or "untrusted".`)
	case slices.ContainsFunc(c.Risk, func(v string) bool { return !slices.Contains(riskLevels, v) }):
		return p, httpx.Invalid("Risk levels must be none, low, medium or high.")
	case slices.ContainsFunc(c.Methods, func(v string) bool { return !slices.Contains(store.MethodKinds, v) }):
		return p, httpx.Invalid("Sign-in methods must be " + methodNames + ".")
	}
	apps := slices.DeleteFunc(slices.Clone(c.AppIDs), func(v string) bool { return v == haloApp })
	for _, ref := range []struct {
		table, noun string
		ids         []string
	}{
		{"groups", "groups", append(slices.Clone(c.GroupIDs), c.ExcludeGroupIDs...)},
		{"users", "people", append(slices.Clone(c.UserIDs), c.ExcludeUserIDs...)},
		{"applications", "applications", apps},
		{"network_zones", "networks", c.ZoneIDs},
	} {
		existing, err := h.st.ExistingIDs(r.Context(), ref.table, ref.ids)
		if err != nil {
			return p, err
		}
		if slices.ContainsFunc(ref.ids, func(v string) bool { return !existing[v] }) {
			return p, httpx.Invalid("One of the chosen " + ref.noun + " no longer exists. Refresh the page and choose again.")
		}
	}
	return p, nil
}

func (h *handlers) createPolicy(w http.ResponseWriter, r *http.Request, actor store.User) error {
	p, err := h.readPolicy(r)
	if err != nil {
		return err
	}
	summary := "Created enforced policy"
	if p.Mode == "report" {
		summary = "Created policy in report-only mode"
	}
	var created store.AccessPolicy
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		created, err = tx.CreateAccessPolicy(r.Context(), p)
		if errors.Is(err, store.ErrConflict) {
			return conflict("A policy named " + p.Name + " already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "policy.create", Summary: summary, TargetType: "policy", TargetID: created.ID, TargetLabel: created.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, policyView{AccessPolicy: created})
}

func changes(old, updated store.AccessPolicy) string {
	parts := []string{}
	if old.Name != updated.Name {
		parts = append(parts, "renamed from “"+old.Name+"”")
	}
	if old.Enabled != updated.Enabled {
		parts = append(parts, map[bool]string{true: "turned on", false: "turned off"}[updated.Enabled])
	}
	if old.Mode != updated.Mode {
		parts = append(parts, map[string]string{"report": "switched to report-only", "enforce": "switched to enforced"}[updated.Mode])
	}
	if old.Effect != updated.Effect {
		parts = append(parts, "changed the effect to "+effectNames[updated.Effect])
	}
	if !reflect.DeepEqual(old.Conditions, updated.Conditions) {
		parts = append(parts, "changed the conditions")
	}
	if old.Description != updated.Description {
		parts = append(parts, "changed the description")
	}
	if len(parts) == 0 {
		return "Saved without changes"
	}
	return capitalize(strings.Join(parts, ", "))
}

func (h *handlers) updatePolicy(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.st.GetAccessPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This policy")
	}
	p, err := h.readPolicy(r)
	if err != nil {
		return err
	}
	p.ID = old.ID
	var updated store.AccessPolicy
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		updated, err = tx.UpdateAccessPolicy(r.Context(), p)
		if errors.Is(err, store.ErrConflict) {
			return conflict("A policy named " + p.Name + " already exists. Pick a different name.")
		}
		if err != nil {
			return found(err, "This policy")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "policy.update", Summary: changes(old, updated), TargetType: "policy", TargetID: updated.ID, TargetLabel: updated.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, policyView{AccessPolicy: updated})
}

func (h *handlers) deletePolicy(w http.ResponseWriter, r *http.Request, actor store.User) error {
	p, err := h.st.GetAccessPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This policy")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteAccessPolicy(r.Context(), p.ID); err != nil {
			return found(err, "This policy")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "policy.delete", Summary: "Deleted policy", TargetType: "policy", TargetID: p.ID, TargetLabel: p.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) reorderPolicies(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	policies, err := h.st.ListAccessPolicies(r.Context())
	if err != nil {
		return err
	}
	current := make([]string, len(policies))
	for i, p := range policies {
		current[i] = p.ID
	}
	if !slices.Equal(slices.Sorted(slices.Values(in.IDs)), slices.Sorted(slices.Values(current))) {
		return httpx.Invalid("The list of policies changed while you were reordering. Reload the page and try again.")
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.ReorderAccessPolicies(r.Context(), in.IDs); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "policy.reorder", Summary: "Changed the order policies are listed and evaluated in", TargetType: "policy", TargetLabel: "Policy order"})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) simulate(w http.ResponseWriter, r *http.Request, _ store.User) error {
	var in struct {
		UserID      string `json:"userId"`
		AppID       string `json:"appId"`
		IP          string `json:"ip"`
		Method      string `json:"method"`
		DeviceTrust string `json:"deviceTrust"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	u, err := h.st.GetUser(ctx, in.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.Invalid("Choose the person to simulate a sign-in for.")
	}
	if err != nil {
		return err
	}
	var app *store.Application
	if in.AppID != haloApp {
		a, err := h.st.GetApplication(ctx, in.AppID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.Invalid("Choose the application they sign in to, or Halo for the console and account portal.")
		}
		if err != nil {
			return err
		}
		app = &a
	}
	switch {
	case !validIP(in.IP):
		return httpx.Invalid("Enter an IP address such as 203.0.113.7 or 2001:db8::1.")
	case !slices.Contains(store.MethodKinds, in.Method):
		return httpx.Invalid("Choose a sign-in method: " + methodNames + ".")
	case !slices.Contains([]string{"unknown", "trusted", "blocked"}, in.DeviceTrust):
		return httpx.Invalid(`Choose the device trust: "unknown", "trusted" or "blocked".`)
	}
	sim, err := h.eng.simulate(ctx, u, app, in.Method, strings.TrimSpace(in.IP), in.DeviceTrust)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sim)
}

func validIP(ip string) bool {
	_, err := netip.ParseAddr(strings.TrimSpace(ip))
	return err == nil
}

func parseRange(raw string) (netip.Prefix, bool) {
	if prefix, err := netip.ParsePrefix(raw); err == nil {
		return prefix.Masked(), true
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, false
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), true
}

func readZone(r *http.Request) (store.NetworkZone, error) {
	var in struct {
		Name  string   `json:"name"`
		Kind  string   `json:"kind"`
		CIDRs []string `json:"cidrs"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.NetworkZone{}, err
	}
	z := store.NetworkZone{Name: strings.TrimSpace(in.Name), Kind: in.Kind, CIDRs: []string{}}
	switch {
	case z.Name == "":
		return z, httpx.Invalid("Enter a name for the network, for example “Office”.")
	case len(z.Name) > 200:
		return z, httpx.Invalid("Keep the network name under 200 characters.")
	case z.Kind != "trusted" && z.Kind != "risky":
		return z, httpx.Invalid(`Choose whether the network is "trusted" or "risky".`)
	}
	for _, raw := range in.CIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, ok := parseRange(raw)
		if !ok {
			return z, httpx.Invalid(fmt.Sprintf("“%s” is not an IP address or range. Write ranges like 203.0.113.0/24 or 2001:db8::/48.", raw))
		}
		if !slices.Contains(z.CIDRs, prefix.String()) {
			z.CIDRs = append(z.CIDRs, prefix.String())
		}
	}
	switch {
	case len(z.CIDRs) == 0:
		return z, httpx.Invalid("Add at least one IP address or range, such as 203.0.113.0/24.")
	case len(z.CIDRs) > 500:
		return z, httpx.Invalid("A network holds up to 500 ranges. Split larger lists into several networks.")
	}
	return z, nil
}

func (h *handlers) zone(ctx context.Context, zoneID string) (store.NetworkZone, error) {
	zones, err := h.st.ListNetworkZones(ctx)
	if err != nil {
		return store.NetworkZone{}, err
	}
	for _, z := range zones {
		if z.ID == zoneID {
			return z, nil
		}
	}
	return store.NetworkZone{}, httpx.NotFound("This network")
}

func (h *handlers) listZones(w http.ResponseWriter, r *http.Request, _ store.User) error {
	zones, err := h.st.ListNetworkZones(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, zones)
}

func (h *handlers) saveZone(w http.ResponseWriter, r *http.Request, actor store.User, z store.NetworkZone, action, summary string, status int) error {
	var saved store.NetworkZone
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		saved, err = tx.SaveNetworkZone(r.Context(), z)
		if errors.Is(err, store.ErrConflict) {
			return conflict("A network named " + z.Name + " already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary, TargetType: "network_zone", TargetID: saved.ID, TargetLabel: saved.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
}

func (h *handlers) createZone(w http.ResponseWriter, r *http.Request, actor store.User) error {
	z, err := readZone(r)
	if err != nil {
		return err
	}
	return h.saveZone(w, r, actor, z, "network_zone.create", fmt.Sprintf("Added %s network with %s", z.Kind, plural(len(z.CIDRs), "range", "ranges")), http.StatusCreated)
}

func (h *handlers) updateZone(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.zone(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	z, err := readZone(r)
	if err != nil {
		return err
	}
	z.ID = old.ID
	return h.saveZone(w, r, actor, z, "network_zone.update", fmt.Sprintf("Updated %s network, now %s", z.Kind, plural(len(z.CIDRs), "range", "ranges")), http.StatusOK)
}

func (h *handlers) deleteZone(w http.ResponseWriter, r *http.Request, actor store.User) error {
	z, err := h.zone(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	policies, err := h.st.ListAccessPolicies(r.Context())
	if err != nil {
		return err
	}
	users := []string{}
	for _, p := range policies {
		if slices.Contains(p.Conditions.ZoneIDs, z.ID) {
			users = append(users, "“"+p.Name+"”")
		}
	}
	if len(users) > 0 {
		those := map[bool]string{true: "that policy", false: "those policies"}[len(users) == 1]
		return conflict(fmt.Sprintf("“%s” is used by %s: %s. Remove it from %s first.", z.Name, plural(len(users), "policy", "policies"), strings.Join(users, ", "), those))
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteNetworkZone(r.Context(), z.ID); err != nil {
			return found(err, "This network")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "network_zone.delete", Summary: "Deleted " + z.Kind + " network", TargetType: "network_zone", TargetID: z.ID, TargetLabel: z.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type methodView struct {
	store.MethodSetting
	Users     int `json:"users"`
	Exclusive int `json:"exclusive"`
}

func (h *handlers) enabledMethods(w http.ResponseWriter, r *http.Request) error {
	settings, err := h.st.ListMethodSettings(r.Context())
	if err != nil {
		return err
	}
	enabled := []string{}
	for _, m := range settings {
		if m.Enabled {
			enabled = append(enabled, m.Method)
		}
	}
	return httpx.JSON(w, http.StatusOK, map[string][]string{"methods": enabled})
}

func (h *handlers) listMethods(w http.ResponseWriter, r *http.Request, _ store.User) error {
	settings, err := h.st.ListMethodSettings(r.Context())
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	enabled := map[string]bool{}
	for _, m := range settings {
		enabled[m.Method] = m.Enabled
	}
	usage, exclusive := map[string]int{}, map[string]int{}
	for _, u := range users {
		if u.Status != "active" {
			continue
		}
		has := map[string]bool{}
		for _, m := range u.Methods {
			has[m.Kind] = true
		}
		for kind := range has {
			usage[kind]++
		}
		primary := []string{}
		for _, kind := range []string{"passkey", "security-key", "totp"} {
			if has[kind] && enabled[kind] {
				primary = append(primary, kind)
			}
		}
		if !has["passkey"] && !has["security-key"] && !has["totp"] {
			usage["magic-link"]++
		}
		switch len(primary) {
		case 0:
			exclusive["magic-link"]++
		case 1:
			exclusive[primary[0]]++
		}
	}
	views := make([]methodView, len(settings))
	for i, m := range settings {
		views[i] = methodView{m, usage[m.Method], exclusive[m.Method]}
	}
	return httpx.JSON(w, http.StatusOK, views)
}

func (h *handlers) setMethod(w http.ResponseWriter, r *http.Request, actor store.User) error {
	method := r.PathValue("method")
	if method == "federated" {
		return httpx.Invalid("Federated sign-in is controlled per identity provider. Turn each provider on or off under Identity providers instead.")
	}
	if !slices.Contains(store.MethodKinds, method) {
		return httpx.NotFound("This sign-in method")
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	settings, err := h.st.ListMethodSettings(r.Context())
	if err != nil {
		return err
	}
	remaining := slices.ContainsFunc(settings, func(m store.MethodSetting) bool {
		return m.Enabled && m.Method != method && m.Method != "recovery-codes"
	})
	if !in.Enabled && method != "recovery-codes" && !remaining {
		return conflict("At least one of passkeys, security keys, authenticator apps or magic links must stay on, otherwise nobody could sign in.")
	}
	summary := "Turned off sign-in with " + methodPlurals[method]
	if in.Enabled {
		summary = "Turned on sign-in with " + methodPlurals[method]
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetMethodEnabled(r.Context(), method, in.Enabled); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "method.update", Summary: summary, TargetType: "method", TargetID: method, TargetLabel: capitalize(methodPlurals[method])})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) listDevices(w http.ResponseWriter, r *http.Request, _ store.User) error {
	devices, err := h.st.ListDevices(r.Context(), r.URL.Query().Get("user"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, devices)
}

func (h *handlers) setDeviceTrust(w http.ResponseWriter, r *http.Request, actor store.User) error {
	d, err := h.st.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This device")
	}
	var in struct {
		Trust string `json:"trust"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	summary, ok := map[string]string{"trusted": "Marked " + d.Name + " as trusted", "blocked": "Blocked " + d.Name, "unknown": "Reset trust for " + d.Name}[in.Trust]
	if !ok {
		return httpx.Invalid(`Choose the device trust: "trusted", "blocked" or "unknown".`)
	}
	owner, err := h.st.GetUser(r.Context(), d.UserID)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetDeviceTrust(r.Context(), d.ID, in.Trust); err != nil {
			return found(err, "This device")
		}
		if in.Trust == "blocked" {
			ended, err := tx.RevokeDeviceSessions(r.Context(), d.UserID, d.TokenHash)
			if err != nil {
				return err
			}
			if ended > 0 {
				summary += " and ended " + plural(ended, "session", "sessions")
			}
		}
		return record(r, tx, actor, store.AuditEvent{Action: "device.trust", Summary: summary, TargetType: "user", TargetID: owner.ID, TargetLabel: owner.Name})
	})
	if err != nil {
		return err
	}
	d.Trust = in.Trust
	return httpx.JSON(w, http.StatusOK, d)
}

func (h *handlers) myDevices(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	devices, err := h.st.ListDevices(r.Context(), u.ID)
	if err != nil {
		return err
	}
	if token := httpx.DeviceToken(r); token != "" {
		hash := secret.Hash(token)
		for i := range devices {
			devices[i].Current = secret.Equal(devices[i].TokenHash, hash)
		}
	}
	return httpx.JSON(w, http.StatusOK, devices)
}

func (h *handlers) listRiskEvents(w http.ResponseWriter, r *http.Request, _ store.User) error {
	events, err := h.st.ListRiskEvents(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, events)
}

func (h *handlers) setRiskStatus(status string) handler {
	verb := map[string]string{"resolved": "Resolved", "dismissed": "Dismissed"}[status]
	action := map[string]string{"resolved": "risk_event.resolve", "dismissed": "risk_event.dismiss"}[status]
	return func(w http.ResponseWriter, r *http.Request, actor store.User) error {
		e, err := h.st.GetRiskEvent(r.Context(), r.PathValue("id"))
		if err != nil {
			return found(err, "This risk event")
		}
		label := "Unknown user"
		if e.UserID != nil {
			u, err := h.st.GetUser(r.Context(), *e.UserID)
			if err != nil {
				return err
			}
			label = u.Name
		}
		err = h.st.Tx(r.Context(), func(tx *store.Store) error {
			if err := tx.SetRiskEventStatus(r.Context(), e.ID, status, actor.ID); err != nil {
				return found(err, "This risk event")
			}
			return record(r, tx, actor, store.AuditEvent{Action: action, Summary: verb + " risk event: " + strings.ToLower(riskTitles[e.Type]), TargetType: "risk_event", TargetID: e.ID, TargetLabel: label})
		})
		if err != nil {
			return err
		}
		updated, err := h.st.GetRiskEvent(r.Context(), e.ID)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, updated)
	}
}

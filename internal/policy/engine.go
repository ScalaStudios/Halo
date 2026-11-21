package policy

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"halo/internal/access"
	"halo/internal/id"
	"halo/internal/secret"
	"halo/internal/store"
)

const haloApp = "halo"

var (
	riskLevels    = []string{"none", "low", "medium", "high"}
	severity      = map[string]int{access.Allow: 0, access.RequireTerms: 1, access.RequirePhishingResistant: 2, access.Block: 3}
	methodUsing   = map[string]string{"passkey": "a passkey", "security-key": "a security key", "totp": "an authenticator app", "magic-link": "a magic link", "recovery-codes": "a recovery code", "password": "a password", "federated": "an identity provider", "access-pass": "a temporary access pass", "certificate": "a client certificate"}
	methodPlurals = map[string]string{"passkey": "passkeys", "security-key": "security keys", "totp": "authenticator apps", "magic-link": "magic links", "recovery-codes": "recovery codes", "password": "passwords", "federated": "identity providers", "certificate": "client certificates"}
	effectVerbs   = map[string]string{access.Allow: "Allows", access.RequirePhishingResistant: "Requires a passkey or security key for", access.Block: "Blocks"}
)

type engine struct{ st *store.Store }

func NewEngine(st *store.Store) access.Engine {
	return &engine{st: st}
}

type signal struct {
	Type   string `json:"type"`
	Level  string `json:"level"`
	Detail string `json:"detail"`
}

type attempt struct {
	user     store.User
	appID    string
	appName  string
	method   string
	ip       string
	addr     netip.Addr
	trust    string
	risk     string
	signals  []signal
	accepted map[string]int
}

type state struct {
	policies []store.AccessPolicy
	zones    []store.NetworkZone
	methods  map[string]bool
	groups   map[string]string
	terms    map[string]store.Terms
	org      store.Settings
}

type result struct {
	policy  store.AccessPolicy
	matched bool
	outcome string
	detail  string
}

func newAttempt(u store.User, app *store.Application, method, ip string) attempt {
	a := attempt{user: u, appID: haloApp, appName: "Halo", method: method, ip: ip, trust: "unknown", risk: "none"}
	if app != nil {
		a.appID, a.appName = app.ID, app.Name
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil {
		a.addr = addr.Unmap()
	}
	return a
}

func (e *engine) load(ctx context.Context) (state, error) {
	s := state{methods: map[string]bool{}, groups: map[string]string{}, terms: map[string]store.Terms{}}
	var err error
	if s.policies, err = e.st.ListAccessPolicies(ctx); err != nil {
		return s, err
	}
	if slices.ContainsFunc(s.policies, func(p store.AccessPolicy) bool { return p.Effect == access.RequireTerms }) {
		terms, err := e.st.ListTerms(ctx)
		if err != nil {
			return s, err
		}
		for _, t := range terms {
			s.terms[t.ID] = t
		}
	}
	if s.zones, err = e.st.ListNetworkZones(ctx); err != nil {
		return s, err
	}
	settings, err := e.st.ListMethodSettings(ctx)
	if err != nil {
		return s, err
	}
	for _, m := range settings {
		s.methods[m.Method] = m.Enabled
	}
	groups, err := e.st.ListGroupsRaw(ctx)
	if err != nil {
		return s, err
	}
	for _, g := range groups {
		s.groups[g.ID] = g.Name
	}
	s.org, err = e.st.Settings(ctx)
	return s, err
}

func (e *engine) acceptances(ctx context.Context, s state, a *attempt) error {
	a.accepted = map[string]int{}
	if len(s.terms) == 0 {
		return nil
	}
	accepted, err := e.st.TermsAcceptances(ctx, a.user.ID)
	for _, t := range accepted {
		a.accepted[t.TermsID] = max(a.accepted[t.TermsID], t.Version)
	}
	return err
}

func (e *engine) Evaluate(ctx context.Context, in access.Input) (access.Decision, error) {
	s, err := e.load(ctx)
	if err != nil {
		return access.Decision{}, err
	}
	a := newAttempt(in.User, in.App, in.Method, in.IP)
	var hash []byte
	others := 0
	if in.DeviceToken != "" {
		hash = secret.Hash(in.DeviceToken)
		device, known, err := e.st.FindDevice(ctx, a.user.ID, hash)
		if err != nil {
			return access.Decision{}, err
		}
		if device != nil {
			a.trust = device.Trust
		} else {
			others = known
		}
	}
	if err := e.assess(ctx, &a, s.zones, others); err != nil {
		return access.Decision{}, err
	}
	if err := e.acceptances(ctx, s, &a); err != nil {
		return access.Decision{}, err
	}
	results, decision := decide(s, a)
	decision.Risk, decision.EventID = a.risk, id.New("evt")
	err = e.st.Tx(ctx, func(tx *store.Store) error {
		var deviceID *string
		if hash != nil {
			signedIn := outcome(decision.Effect, a.method) == "allow" && len(decision.Terms) == 0
			touched, err := tx.TouchDevice(ctx, a.user.ID, hash, in.UserAgent, a.ip, signedIn)
			if err != nil {
				return err
			}
			deviceID = &touched
		}
		matches := []store.PolicyMatch{}
		for _, r := range results {
			if r.matched {
				matches = append(matches, store.PolicyMatch{PolicyID: r.policy.ID, Mode: r.policy.Mode, Outcome: r.outcome})
			}
		}
		if err := tx.RecordPolicyEvaluation(ctx, store.PolicyEvaluation{SignInID: decision.EventID, Risk: a.risk, Effect: decision.Effect, Matches: matches}); err != nil {
			return err
		}
		for _, sig := range a.signals {
			if sig.Level == "medium" || sig.Level == "high" {
				if err := tx.RecordRiskEvent(ctx, store.RiskEvent{Type: sig.Type, Level: sig.Level, UserID: &a.user.ID, IP: a.ip, DeviceID: deviceID, SignInID: &decision.EventID, Detail: sig.Detail}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return decision, err
}

func (e *engine) assess(ctx context.Context, a *attempt, zones []store.NetworkZone, otherDevices int) error {
	add := func(kind, level, detail string) {
		a.signals = append(a.signals, signal{Type: kind, Level: level, Detail: detail})
	}
	for _, z := range zones {
		if z.Kind == "risky" && inZone(a.addr, z) {
			add("risky-network", "high", a.ip+" is in the risky network “"+z.Name+"”.")
		}
	}
	failures, err := e.st.RecentFailures(ctx, a.user.ID, a.user.Email, store.MethodKinds, time.Now().Add(-15*time.Minute))
	if err != nil {
		return err
	}
	if failures >= 5 {
		add("failed-sign-ins", "high", fmt.Sprintf("%d failed sign-ins in the last 15 minutes.", failures))
	}
	if otherDevices > 0 {
		add("new-device", "medium", fmt.Sprintf("First sign-in from this device. The account already used %s.", plural(otherDevices, "other device", "other devices")))
	}
	if a.method == "password" {
		add("password", "low", "Password sign-in. Passwords can be guessed, phished or reused from another site.")
	}
	if a.addr.IsValid() {
		ips, err := e.st.RecentSignInIPs(ctx, a.user.ID)
		if err != nil {
			return err
		}
		prefix := network(a.addr)
		seen := slices.ContainsFunc(ips, func(ip string) bool {
			addr, err := netip.ParseAddr(ip)
			return err == nil && network(addr.Unmap()) == prefix
		})
		if len(ips) > 0 && !seen {
			add("new-network", "low", "First successful sign-in from "+prefix.String()+".")
		}
	}
	for _, sig := range a.signals {
		if slices.Index(riskLevels, sig.Level) > slices.Index(riskLevels, a.risk) {
			a.risk = sig.Level
		}
	}
	return nil
}

func network(addr netip.Addr) netip.Prefix {
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	prefix, _ := addr.Prefix(bits)
	return prefix
}

func inZone(addr netip.Addr, z store.NetworkZone) bool {
	return addr.IsValid() && slices.ContainsFunc(z.CIDRs, func(cidr string) bool {
		prefix, err := netip.ParsePrefix(cidr)
		return err == nil && prefix.Contains(addr)
	})
}

func phishingResistant(method string) bool {
	return method == "passkey" || method == "security-key"
}

func outcome(effect, method string) string {
	switch {
	case effect == access.Block:
		return "block"
	case effect == access.RequirePhishingResistant && !phishingResistant(method):
		return "require"
	}
	return "allow"
}

func decide(s state, a attempt) ([]result, access.Decision) {
	results := []result{}
	for _, p := range s.policies {
		if !p.Enabled {
			continue
		}
		r := result{policy: p}
		if r.matched, r.detail = s.match(p, a); r.matched {
			r.outcome = outcome(p.Effect, a.method)
			if t, ok := s.document(p); ok && a.accepted[t.ID] < t.Version {
				r.outcome = "terms"
			}
		}
		results = append(results, r)
	}
	pending := []store.Terms{}
	for _, r := range results {
		t, _ := s.document(r.policy)
		if r.outcome == "terms" && r.policy.Mode == "enforce" && !slices.ContainsFunc(pending, func(p store.Terms) bool { return p.ID == t.ID }) {
			pending = append(pending, t)
		}
	}
	var winner *result
	for i, r := range results {
		if r.matched && r.policy.Mode == "enforce" && (winner == nil || severity[r.policy.Effect] > severity[winner.policy.Effect]) {
			winner = &results[i]
		}
	}
	enabled, known := s.methods[a.method]
	switch {
	case a.trust == "blocked":
		return results, access.Decision{Effect: access.Block, Reason: "An administrator blocked this device."}
	case known && !enabled:
		return results, access.Decision{Effect: access.Block, Reason: "Sign-in with " + methodPlurals[a.method] + " is turned off for this organization."}
	case a.method == "password" && !s.org.PasswordAllowed(a.user):
		groups := []string{}
		for _, g := range s.org.PasswordGroupIDs {
			groups = append(groups, s.group(g))
		}
		return results, access.Decision{Effect: access.Block, Reason: "Sign-in with passwords is limited to members of " + strings.Join(groups, " or ") + "."}
	case winner != nil:
		return results, access.Decision{Effect: winner.policy.Effect, Policy: winner.policy.Name, Reason: winner.detail, Terms: pending}
	}
	return results, access.Decision{Effect: access.Allow, Terms: pending}
}

func (s state) document(p store.AccessPolicy) (store.Terms, bool) {
	if p.Effect != access.RequireTerms || p.TermsID == nil {
		return store.Terms{}, false
	}
	t, ok := s.terms[*p.TermsID]
	return t, ok
}

func (s state) verb(p store.AccessPolicy) string {
	if t, ok := s.document(p); ok {
		return fmt.Sprintf("Requires accepting “%s” version %d for", t.Name, t.Version)
	}
	return effectVerbs[p.Effect]
}

func (s state) group(groupID string) string {
	if name, ok := s.groups[groupID]; ok {
		return name
	}
	return "a deleted group"
}

func (s state) zone(zoneIDs []string, addr netip.Addr) *store.NetworkZone {
	for i, z := range s.zones {
		if slices.Contains(zoneIDs, z.ID) && inZone(addr, z) {
			return &s.zones[i]
		}
	}
	return nil
}

func (s state) zoneNames(zoneIDs []string) string {
	names := []string{}
	for _, z := range s.zones {
		if slices.Contains(zoneIDs, z.ID) {
			names = append(names, "“"+z.Name+"”")
		}
	}
	return strings.Join(names, " or ")
}

func shared(a, b []string) string {
	for _, v := range a {
		if slices.Contains(b, v) {
			return v
		}
	}
	return ""
}

func (s state) match(p store.AccessPolicy, a attempt) (bool, string) {
	c := p.Conditions
	scope := []string{}
	switch {
	case c.AllUsers:
	case slices.Contains(c.UserIDs, a.user.ID):
		scope = append(scope, "by "+a.user.Name)
	default:
		g := shared(c.GroupIDs, a.user.GroupIDs)
		if g == "" {
			return false, a.user.Name + " isn't one of the people or groups it applies to."
		}
		scope = append(scope, "by members of "+s.group(g))
	}
	if slices.Contains(c.ExcludeUserIDs, a.user.ID) {
		return false, a.user.Name + " is excluded."
	}
	if g := shared(c.ExcludeGroupIDs, a.user.GroupIDs); g != "" {
		return false, a.user.Name + " is excluded as a member of " + s.group(g) + "."
	}
	if p.Effect == access.Block && slices.Contains(s.org.EmergencyAccessIDs, a.user.ID) {
		return false, a.user.Name + " is an emergency access account, which blocking policies never apply to."
	}
	if !c.AllApps {
		if !slices.Contains(c.AppIDs, a.appID) {
			return false, "It doesn't cover " + a.appName + "."
		}
		scope = append(scope, "to "+a.appName)
	}
	switch c.Network {
	case "in":
		z := s.zone(c.ZoneIDs, a.addr)
		if z == nil {
			return false, a.ip + " isn't in " + s.zoneNames(c.ZoneIDs) + "."
		}
		scope = append(scope, "from “"+z.Name+"”")
	case "not-in":
		if z := s.zone(c.ZoneIDs, a.addr); z != nil {
			return false, a.ip + " is in “" + z.Name + "”."
		}
		scope = append(scope, "from outside "+s.zoneNames(c.ZoneIDs))
	}
	switch {
	case c.Device == "trusted" && a.trust != "trusted":
		return false, "The device isn't trusted."
	case c.Device == "trusted":
		scope = append(scope, "on trusted devices")
	case c.Device == "untrusted" && a.trust == "trusted":
		return false, "The device is trusted."
	case c.Device == "untrusted":
		scope = append(scope, "on devices that aren't trusted")
	}
	if len(c.Risk) > 0 {
		if !slices.Contains(c.Risk, a.risk) {
			return false, "The sign-in risk was " + a.risk + "."
		}
		scope = append(scope, "with "+a.risk+" risk")
	}
	if len(c.Methods) > 0 {
		if !slices.Contains(c.Methods, a.method) {
			return false, "It doesn't cover sign-in with " + methodUsing[a.method] + "."
		}
		scope = append(scope, "using "+methodUsing[a.method])
	}
	if len(scope) == 0 {
		return true, s.verb(p) + " every sign-in."
	}
	reason := s.verb(p) + " sign-ins " + strings.Join(scope, " ") + "."
	if len(c.Risk) > 0 && a.risk != "none" {
		evidence := []string{}
		for _, sig := range a.signals {
			if sig.Level == a.risk {
				evidence = append(evidence, sig.Detail)
			}
		}
		reason += " This sign-in was " + a.risk + " risk: " + strings.Join(evidence, " ")
	}
	return true, reason
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

type simulatedPolicy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Effect  string `json:"effect"`
	Matched bool   `json:"matched"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
}

type simulation struct {
	Effect   string            `json:"effect"`
	Outcome  string            `json:"outcome"`
	Policy   string            `json:"policy"`
	Reason   string            `json:"reason"`
	Risk     string            `json:"risk"`
	Signals  []signal          `json:"signals"`
	Policies []simulatedPolicy `json:"policies"`
}

func (e *engine) lockedOut(ctx context.Context, actorID, actorMethod, ip, trust string) (bool, error) {
	s, err := e.load(ctx)
	if err != nil {
		return false, err
	}
	users, err := e.st.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	checked := false
	for _, u := range users {
		if u.Status != "active" || !u.HasRole("global_admin") && u.ID != actorID && !slices.Contains(s.org.EmergencyAccessIDs, u.ID) {
			continue
		}
		checked = true
		methods := []string{"magic-link"}
		for _, m := range u.Methods {
			methods = append(methods, m.Kind)
		}
		if u.ID == actorID && actorMethod != "" {
			methods = append(methods, actorMethod)
		}
		for _, method := range methods {
			a := newAttempt(u, nil, method, ip)
			a.trust = trust
			if err := e.assess(ctx, &a, s.zones, 0); err != nil {
				return false, err
			}
			if _, d := decide(s, a); outcome(d.Effect, method) == "allow" {
				return false, nil
			}
		}
	}
	return checked, nil
}

func (e *engine) simulate(ctx context.Context, u store.User, app *store.Application, method, ip, trust string) (simulation, error) {
	s, err := e.load(ctx)
	if err != nil {
		return simulation{}, err
	}
	a := newAttempt(u, app, method, ip)
	a.trust = trust
	if err := e.assess(ctx, &a, s.zones, 0); err != nil {
		return simulation{}, err
	}
	if err := e.acceptances(ctx, s, &a); err != nil {
		return simulation{}, err
	}
	results, d := decide(s, a)
	sim := simulation{Effect: d.Effect, Outcome: outcome(d.Effect, method), Policy: d.Policy, Reason: d.Reason, Risk: a.risk, Signals: append([]signal{}, a.signals...), Policies: []simulatedPolicy{}}
	if sim.Outcome == "allow" && len(d.Terms) > 0 {
		sim.Outcome = "terms"
	}
	for _, r := range results {
		sim.Policies = append(sim.Policies, simulatedPolicy{ID: r.policy.ID, Name: r.policy.Name, Mode: r.policy.Mode, Effect: r.policy.Effect, Matched: r.matched, Outcome: r.outcome, Detail: r.detail})
	}
	return sim, nil
}

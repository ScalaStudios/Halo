package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

const (
	demoDay    = 60 * 24
	demoNever  = -1
	demoWindow = 60 * 48
)

type demoRand struct{ state uint32 }

func (r *demoRand) next() float64 {
	r.state += 0x6d2b79f5
	t := r.state
	t = (t ^ t>>15) * (t | 1)
	t ^= t + (t^t>>7)*(t|61)
	return float64(t^t>>14) / 4294967296
}

func (r *demoRand) between(min, max int) int {
	return int(math.Floor(r.next()*float64(max-min+1))) + min
}

func (r *demoRand) chance(p float64) bool {
	return r.next() < p
}

func pick[T any](r *demoRand, items []T) T {
	return items[int(r.next()*float64(len(items)))]
}

type demoMethod struct {
	kind, label string
	addedDays   int
	usedMinutes int
	remaining   int
}

type demoUser struct {
	id, key, name, email, title, department, location, status, source, manager string
	roles, groups                                                              []string
	methods                                                                    []demoMethod
	created, lastSignIn                                                        int
}

type demoGroup struct{ key, name, description, kind, rule, source string }

type demoCredential struct {
	kind, label, hint                     string
	createdDays, expiresDays, usedMinutes int
}

type demoApp struct {
	key, name, description, protocol, appType, status, clientID, homepage, owner, guide string
	redirects, postLogout, scopes, groups                                               []string
	credentials                                                                         []demoCredential
	createdDays                                                                         int
	ttl                                                                                 [3]int
	rotation                                                                            bool
}

type demoLocation struct{ label, prefix string }

type demoClient struct{ label, agent string }

var demoGroups = []demoGroup{
	{"everyone", "Everyone", "Every active person in the organization.", "dynamic", `user.status == "active"`, "Halo"},
	{"engineering", "Engineering", "Product and platform engineering.", "dynamic", `user.department == "Engineering"`, "SCIM · Workday"},
	{"infra", "Infrastructure", "Owns clusters, networking and the deploy pipeline.", "dynamic", `user.department == "Infrastructure"`, "SCIM · Workday"},
	{"oncall", "Infrastructure on-call", "Current on-call rotation. Grants break-glass access to production.", "assigned", "", "Halo"},
	{"security", "Security team", "Incident response and identity operations.", "assigned", "", "Halo"},
	{"halo_admins", "Halo administrators", "People with an administrative role in Halo.", "assigned", "", "Halo"},
	{"grafana_editors", "Grafana editors", "Can create and edit dashboards in Grafana.", "assigned", "", "Halo"},
	{"forgejo_maintainers", "Forgejo maintainers", "Repository administrators on git.example.com.", "assigned", "", "Halo"},
	{"prod_access", "Production access", "Time-bound shell and database access to production.", "assigned", "", "Halo"},
	{"k8s_admins", "Kubernetes cluster admins", "Mapped to the cluster-admin ClusterRole on prod-eu-1.", "assigned", "", "Halo"},
	{"support", "Support", "Customer support and success.", "dynamic", `user.department == "Support"`, "SCIM · Workday"},
	{"finance", "Finance", "Finance and accounting.", "dynamic", `user.department == "Finance"`, "SCIM · Workday"},
	{"design", "Design", "Product and brand design.", "dynamic", `user.department == "Design"`, "SCIM · Workday"},
	{"contractors", "Contractors", "External contractors. Access expires with the contract.", "assigned", "", "LDAP · corp.example.com"},
}

func demoCurated() []demoUser {
	return []demoUser{
		{key: "luna", name: "Luna", email: "luna@example.com", title: "Platform lead", department: "Infrastructure", location: "Amsterdam", status: "active", source: "Halo",
			roles:  []string{"global_admin"},
			groups: []string{"everyone", "infra", "oncall", "halo_admins", "grafana_editors", "forgejo_maintainers", "prod_access", "k8s_admins"},
			methods: []demoMethod{
				{"passkey", "MacBook Pro · Touch ID", 212, 12, 0},
				{"security-key", "YubiKey 5C NFC", 340, 60 * 26, 0},
				{"passkey", "Pixel 9 · screen lock", 96, 60 * 30, 0},
				{"recovery-codes", "Recovery codes", 340, demoNever, 8},
			},
			created: 412 * demoDay, lastSignIn: 12},
		{key: "priya", name: "Priya Raman", email: "priya.raman@example.com", title: "Security engineer", department: "Security", location: "London", status: "active", source: "SCIM · Workday", manager: "luna",
			roles:  []string{"security_admin"},
			groups: []string{"everyone", "security", "halo_admins", "prod_access"},
			methods: []demoMethod{
				{"security-key", "YubiKey 5Ci", 290, 44, 0},
				{"passkey", "ThinkPad X1 · Windows Hello", 120, 60 * 20, 0},
				{"recovery-codes", "Recovery codes", 290, demoNever, 10},
			},
			created: 301 * demoDay, lastSignIn: 44},
		{key: "marcus", name: "Marcus Okafor", email: "marcus.okafor@example.com", title: "IT administrator", department: "Operations", location: "Lagos", status: "active", source: "SCIM · Workday", manager: "luna",
			roles:  []string{"user_admin"},
			groups: []string{"everyone", "halo_admins"},
			methods: []demoMethod{
				{"totp", "Authenticator app · 1Password", 188, 95, 0},
				{"magic-link", "Email magic link", 188, 60 * 72, 0},
			},
			created: 190 * demoDay, lastSignIn: 95},
		{key: "elena", name: "Elena Vasquez", email: "elena.vasquez@example.com", title: "Head of support", department: "Support", location: "Madrid", status: "active", source: "SCIM · Workday",
			roles:   []string{"helpdesk_admin"},
			groups:  []string{"everyone", "support", "halo_admins"},
			methods: []demoMethod{{"totp", "Authenticator app · Google Authenticator", 250, 60 * 5, 0}},
			created: 260 * demoDay, lastSignIn: 60 * 5},
		{key: "tomasz", name: "Tomasz Nowak", email: "tomasz.nowak@example.com", title: "Finance systems lead", department: "Finance", location: "Warsaw", status: "active", source: "SCIM · Workday",
			roles:  []string{"app_admin"},
			groups: []string{"everyone", "finance", "halo_admins"},
			methods: []demoMethod{
				{"totp", "Authenticator app · Microsoft Authenticator", 140, 60 * 26, 0},
				{"magic-link", "Email magic link", 140, 60 * 50, 0},
			},
			created: 150 * demoDay, lastSignIn: 60 * 26},
		{key: "hana", name: "Hana Kobayashi", email: "hana.kobayashi@example.com", title: "Product designer", department: "Design", location: "Tokyo", status: "active", source: "SCIM · Workday",
			groups:  []string{"everyone", "design"},
			methods: []demoMethod{{"passkey", "iPhone 16 · Face ID", 80, 60 * 9, 0}},
			created: 85 * demoDay, lastSignIn: 60 * 9},
		{key: "daniel", name: "Daniel Mensah", email: "daniel.mensah@example.com", title: "Backend engineer", department: "Engineering", location: "Accra", status: "suspended", source: "SCIM · Workday", manager: "grace",
			groups:  []string{"everyone", "engineering"},
			methods: []demoMethod{{"passkey", "MacBook Air · Touch ID", 300, 12 * demoDay, 0}},
			created: 305 * demoDay, lastSignIn: 12 * demoDay},
		{key: "grace", name: "Grace Liu", email: "grace.liu@example.com", title: "Engineering manager", department: "Engineering", location: "Toronto", status: "active", source: "SCIM · Workday",
			roles:  []string{"auditor"},
			groups: []string{"everyone", "engineering", "grafana_editors", "forgejo_maintainers"},
			methods: []demoMethod{
				{"passkey", "MacBook Pro · Touch ID", 230, 60 * 3, 0},
				{"totp", "Authenticator app · 1Password", 230, 40 * demoDay, 0},
			},
			created: 240 * demoDay, lastSignIn: 60 * 3},
		{key: "sofia", name: "Sofia Lindqvist", email: "sofia.lindqvist@example.com", title: "Site reliability engineer", department: "Infrastructure", location: "Stockholm", status: "invited", source: "SCIM · Workday", manager: "luna",
			groups:  []string{"everyone", "infra"},
			created: 2 * demoDay, lastSignIn: demoNever},
		{key: "arjun", name: "Arjun Mehta", email: "arjun.mehta@example.com", title: "Frontend engineer", department: "Engineering", location: "Bengaluru", status: "active", source: "LDAP · corp.example.com", manager: "grace",
			groups:  []string{"everyone", "engineering", "contractors"},
			methods: []demoMethod{{"magic-link", "Email magic link", 60, 60 * 7, 0}},
			created: 62 * demoDay, lastSignIn: 60 * 7},
	}
}

var (
	demoFirst = []string{"Amara", "Ben", "Chen", "Dara", "Emil", "Farah", "Gabriel", "Ines", "Jonas", "Kofi", "Leila", "Mateo", "Nadia", "Oskar", "Paula", "Quinn", "Rafael", "Selin", "Theo", "Uma", "Viktor", "Wen", "Yusuf", "Zoe", "Aiko", "Bilal", "Clara", "Diego", "Esther", "Felix", "Greta", "Hugo", "Ivy", "Jae", "Kiran", "Lucia", "Milan", "Noor", "Omar", "Pia", "Ravi", "Sara", "Tariq", "Vera", "Will", "Ximena", "Yara", "Zain", "Anouk", "Bruno", "Camille", "Dev", "Eitan", "Freya"}
	demoLast  = []string{"Adeyemi", "Bergström", "Castillo", "Dubois", "Eriksen", "Fischer", "García", "Haddad", "Ivanova", "Jansen", "Kowalski", "Lambert", "Moreau", "Nakamura", "Olsen", "Petrov", "Quraishi", "Rossi", "Silva", "Tanaka", "Ueda", "Vargas", "Weber", "Xu", "Yilmaz", "Zhang", "Andersen", "Bauer", "Costa", "Demir", "Esposito", "Ferreira", "Gómez", "Horvat", "Iqbal", "Jovanović", "Kim", "Larsen", "Murphy", "Novak"}
	demoDepts = []struct {
		name   string
		titles []string
		group  string
	}{
		{"Engineering", []string{"Backend engineer", "Frontend engineer", "Staff engineer", "Mobile engineer", "Engineer"}, "engineering"},
		{"Infrastructure", []string{"Site reliability engineer", "Network engineer", "Platform engineer"}, "infra"},
		{"Support", []string{"Support specialist", "Support engineer", "Customer success manager"}, "support"},
		{"Design", []string{"Product designer", "Brand designer", "UX researcher"}, "design"},
		{"Finance", []string{"Accountant", "Financial analyst", "Controller"}, "finance"},
		{"Product", []string{"Product manager", "Technical writer"}, ""},
		{"People", []string{"People partner", "Recruiter"}, ""},
		{"Sales", []string{"Account executive", "Solutions engineer"}, ""},
	}
	demoCities   = []string{"Amsterdam", "Berlin", "Lisbon", "London", "Toronto", "Austin", "Nairobi", "Singapore", "São Paulo", "Copenhagen", "Melbourne", "Seoul", "Dublin", "Remote"}
	demoPasskeys = []string{"MacBook Pro · Touch ID", "MacBook Air · Touch ID", "iPhone 16 · Face ID", "Pixel 9 · screen lock", "ThinkPad X1 · Windows Hello", "Surface Laptop · Windows Hello", "Galaxy S25 · fingerprint"}
	demoKeys     = []string{"YubiKey 5C NFC", "YubiKey 5 NFC", "YubiKey Bio", "Google Titan Key", "Nitrokey 3C NFC"}
	demoTOTP     = []string{"1Password", "Google Authenticator", "Aegis", "Bitwarden", "Microsoft Authenticator"}
	demoAccents  = strings.NewReplacer("á", "a", "à", "a", "ã", "a", "é", "e", "è", "e", "í", "i", "ó", "o", "ö", "o", "ú", "u", "ü", "u", "ç", "c", "ć", "c", "č", "c", "ñ", "n", "š", "s", "ž", "z")

	demoLocations = []demoLocation{
		{"Amsterdam, NL", "145.53.112."}, {"London, GB", "81.2.69."}, {"Berlin, DE", "93.184.21."}, {"Toronto, CA", "142.112.40."},
		{"Lisbon, PT", "94.60.12."}, {"Lagos, NG", "102.89.33."}, {"Tokyo, JP", "126.15.80."}, {"Bengaluru, IN", "49.207.51."},
		{"Austin, US", "70.114.9."}, {"Singapore, SG", "116.88.4."}, {"Madrid, ES", "88.12.4."}, {"Warsaw, PL", "83.22.17."},
		{"Accra, GH", "154.160.2."}, {"Stockholm, SE", "78.82.40."}, {"Nairobi, KE", "105.163.1."}, {"São Paulo, BR", "177.92.30."},
		{"Copenhagen, DK", "87.49.44."}, {"Melbourne, AU", "101.181.9."}, {"Seoul, KR", "211.36.142."}, {"Dublin, IE", "86.44.21."},
	}
	demoClients = []demoClient{
		{"Firefox 143 · macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:143.0) Gecko/20100101 Firefox/143.0"},
		{"Chrome 141 · Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"},
		{"Safari 26 · iOS", "Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/605.1.15"},
		{"Chrome 141 · Android", "Mozilla/5.0 (Linux; Android 16; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36"},
		{"Firefox 143 · Linux", "Mozilla/5.0 (X11; Linux x86_64; rv:143.0) Gecko/20100101 Firefox/143.0"},
		{"kubelogin 1.34 · Linux", "kubelogin/1.34.0 (Linux x86_64)"},
		{"Edge 141 · Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0"},
	}
	demoFailReasons = []string{
		"Passkey assertion was cancelled on the device.",
		"Authenticator code did not match. 2 attempts remaining.",
		"User is not assigned to this application.",
		"Blocked by policy “Require trusted device for production”.",
		"Magic link expired after 10 minutes.",
		"Security key not registered to this account.",
	}
)

var (
	demoTokens = [3]int{900, 28800, 900}
	demoSAML   = [3]int{3600, 0, 0}
)

var demoApps = []demoApp{
	{key: "grafana", name: "Grafana", description: "Metrics dashboards and alerting for every cluster.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZB4QK7M2XWD9N3TRFV6CHA", homepage: "https://grafana.example.com",
		redirects: []string{"https://grafana.example.com/login/generic_oauth"}, postLogout: []string{"https://grafana.example.com/login"},
		scopes: []string{"openid", "profile", "email", "groups"}, groups: []string{"engineering", "infra", "grafana_editors", "security"},
		credentials: []demoCredential{{"secret", "Production", "hls_…Q7dX", 356, 9, 3}},
		owner:       "luna", createdDays: 356, ttl: demoTokens, rotation: true, guide: "grafana"},
	{key: "forgejo", name: "Forgejo", description: "Source code, reviews and CI at git.example.com.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZB7RT2C9QW4KM8XD3NV6PE", homepage: "https://git.example.com",
		redirects: []string{"https://git.example.com/user/oauth2/halo/callback"},
		scopes:    []string{"openid", "profile", "email", "groups"}, groups: []string{"engineering", "infra", "forgejo_maintainers"},
		credentials: []demoCredential{{"secret", "Production", "hls_…mK2v", 120, 245, 8}},
		owner:       "grace", createdDays: 380, ttl: demoTokens, rotation: true, guide: "forgejo"},
	{key: "kubernetes", name: "Kubernetes · prod-eu-1", description: "kubectl sign-in through kubelogin. Groups map to ClusterRoleBindings.", protocol: "oidc", appType: "native", status: "active",
		clientID: "hl_01J8ZB9MW3B7KT2XQ6NV8RF4HD", homepage: "https://k8s.example.com",
		redirects: []string{"http://localhost:8000", "http://localhost:18000"},
		scopes:    []string{"openid", "profile", "email", "groups", "offline_access"}, groups: []string{"infra", "k8s_admins", "oncall"},
		owner: "luna", createdDays: 300, ttl: [3]int{600, 43200, 600}, rotation: true, guide: "kubernetes"},
	{key: "proxmox", name: "Proxmox VE", description: "Hypervisor cluster for the lab and staging environments.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZBC4TQ9X2MW7KD3BN6VC8R", homepage: "https://pve.example.com:8006",
		redirects: []string{"https://pve.example.com:8006"},
		scopes:    []string{"openid", "profile", "email"}, groups: []string{"infra"},
		credentials: []demoCredential{{"secret", "Cluster", "hls_…9fPa", 344, 21, 60 * 5}},
		owner:       "luna", createdDays: 344, ttl: demoTokens, rotation: true, guide: "generic-oidc"},
	{key: "outline", name: "Outline", description: "Company handbook, runbooks and meeting notes.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZBF8NB2R6WQ9TX4KM3DH7V", homepage: "https://wiki.example.com",
		redirects: []string{"https://wiki.example.com/auth/oidc.callback"},
		scopes:    []string{"openid", "profile", "email"}, groups: []string{"everyone"},
		credentials: []demoCredential{{"secret", "Production", "hls_…Tq4s", 60, 305, 2}},
		owner:       "marcus", createdDays: 398, ttl: demoTokens, rotation: true, guide: "generic-oidc"},
	{key: "nextcloud", name: "Nextcloud", description: "Files, calendars and shared drives.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZBG3KD7W2QX9MV4TN8BR6C", homepage: "https://cloud.example.com",
		redirects: []string{"https://cloud.example.com/apps/user_oidc/code"}, postLogout: []string{"https://cloud.example.com"},
		scopes: []string{"openid", "profile", "email", "groups"}, groups: []string{"everyone"},
		credentials: []demoCredential{{"secret", "Production", "hls_…b81Z", 200, 165, 14}},
		owner:       "marcus", createdDays: 360, ttl: demoTokens, rotation: true, guide: "generic-oidc"},
	{key: "headscale", name: "Headscale", description: "Self-hosted Tailscale control server for the corporate tailnet.", protocol: "oidc", appType: "web", status: "active",
		clientID: "hl_01J8ZBH6QX2N9TW4KB7MD3RV8F", homepage: "https://hs.example.com",
		redirects: []string{"https://hs.example.com/oidc/callback"},
		scopes:    []string{"openid", "profile", "email"}, groups: []string{"infra", "engineering"},
		credentials: []demoCredential{{"secret", "Control server", "hls_…Lw0e", 90, 275, 60 * 2}},
		owner:       "luna", createdDays: 210, ttl: demoTokens, rotation: true, guide: "generic-oidc"},
	{key: "aws", name: "AWS IAM Identity Center", description: "Federated console and CLI access to 6 AWS accounts.", protocol: "saml", appType: "web", status: "active",
		clientID: "urn:amazon:webservices", homepage: "https://example.awsapps.com/start",
		redirects:   []string{"https://eu-west-1.signin.aws.amazon.com/platform/saml/acs/7c1d0b4e"},
		groups:      []string{"infra", "security", "prod_access"},
		credentials: []demoCredential{{"certificate", "SAML signing certificate", "SHA-256 7A:1F:…:C4:09", 700, 395, 60 * 6}},
		owner:       "priya", createdDays: 700, ttl: demoSAML, guide: "generic-saml"},
	{key: "slack", name: "Slack", description: "Company chat. SAML sign-in with SCIM provisioning.", protocol: "saml", appType: "web", status: "active",
		clientID: "https://slack.com", homepage: "https://fernway.slack.com",
		redirects:   []string{"https://fernway.slack.com/sso/saml"},
		groups:      []string{"everyone"},
		credentials: []demoCredential{{"certificate", "SAML signing certificate", "SHA-256 B2:90:…:5E:11", 500, 595, 20}},
		owner:       "marcus", createdDays: 500, ttl: demoSAML, guide: "generic-saml"},
	{key: "zammad", name: "Zammad", description: "Support ticketing for help@example.com.", protocol: "saml", appType: "web", status: "active",
		clientID: "https://support.example.com/auth/saml/metadata", homepage: "https://support.example.com",
		redirects:   []string{"https://support.example.com/auth/saml/callback"},
		groups:      []string{"support"},
		credentials: []demoCredential{{"certificate", "SAML signing certificate", "SHA-256 0C:44:…:9A:F2", 400, 330, 60 * 4}},
		owner:       "elena", createdDays: 400, ttl: demoSAML, guide: "generic-saml"},
	{key: "billing_api", name: "Billing API", description: "Internal API for invoices and usage. Client credentials only.", protocol: "oauth", appType: "service", status: "active",
		clientID: "hl_01J8ZBK2BX8R3NW7TQ4MD9VH6C", homepage: "https://billing.internal.example.com",
		scopes: []string{"billing:read", "billing:write", "usage:report"},
		credentials: []demoCredential{
			{"secret", "invoice-worker", "hls_…Pp3r", 30, 60, 1},
			{"secret", "usage-reporter", "hls_…xe7N", 30, 60, 6},
		},
		owner: "tomasz", createdDays: 140, ttl: [3]int{300, 0, 0}, guide: "generic-oidc"},
	{key: "legacy_wiki", name: "MediaWiki (legacy)", description: "Read-only archive of the old wiki. Scheduled for removal.", protocol: "saml", appType: "web", status: "disabled",
		clientID: "https://old-wiki.example.com/saml", homepage: "https://old-wiki.example.com",
		redirects:   []string{"https://old-wiki.example.com/Special:PluggableAuthLogin"},
		credentials: []demoCredential{{"certificate", "SAML signing certificate", "SHA-256 E1:07:…:22:BD", 900, -12, 140 * demoDay}},
		owner:       "marcus", createdDays: 900, ttl: demoSAML, guide: "generic-saml"},
}

func demoDirectory() []demoUser {
	users := demoCurated()
	r := &demoRand{state: 1729}
	used := map[string]bool{}
	for _, u := range users {
		used[u.name] = true
	}
	for generated := 0; generated < 58; {
		first, last := pick(r, demoFirst), pick(r, demoLast)
		name := first + " " + last
		if used[name] {
			continue
		}
		used[name] = true
		generated++
		dept := pick(r, demoDepts)
		roll := r.next()
		age := r.between(20, 700)
		for range 26 {
			r.next()
		}
		var methods []demoMethod
		switch {
		case roll < 0.62:
			methods = append(methods, demoMethod{"passkey", pick(r, demoPasskeys), r.between(5, age), r.between(10, 6*demoDay), 0})
			if r.chance(0.35) {
				methods = append(methods, demoMethod{"security-key", pick(r, demoKeys), r.between(5, age), r.between(60, 30*demoDay), 0})
			}
		case roll < 0.9:
			methods = append(methods, demoMethod{"totp", "Authenticator app · " + pick(r, demoTOTP), r.between(5, age), r.between(30, 8*demoDay), 0})
		default:
			methods = append(methods, demoMethod{"magic-link", "Email magic link", r.between(5, age), r.between(30, 8*demoDay), 0})
		}
		if roll < 0.9 && r.chance(0.5) {
			added := r.between(5, age)
			methods = append(methods, demoMethod{"recovery-codes", "Recovery codes", added, demoNever, r.between(4, 10)})
		}
		status := "active"
		if statusRoll := r.next(); statusRoll >= 0.95 {
			status = "deprovisioned"
		} else if statusRoll >= 0.9 {
			status = "suspended"
		}
		groups := []string{"everyone"}
		if dept.group != "" {
			groups = append(groups, dept.group)
		}
		if dept.name == "Infrastructure" && r.chance(0.3) {
			groups = append(groups, "oncall")
		}
		if dept.name == "Engineering" && r.chance(0.2) {
			groups = append(groups, "forgejo_maintainers")
		}
		if r.chance(0.12) {
			groups = append(groups, "grafana_editors")
		}
		if r.chance(0.08) {
			groups = append(groups, "contractors")
		}
		title := pick(r, dept.titles)
		location := pick(r, demoCities)
		source := "Halo"
		if r.chance(0.85) {
			source = "SCIM · Workday"
		}
		var lastSignIn int
		if status == "deprovisioned" {
			lastSignIn = r.between(30, 90) * demoDay
		} else {
			lastSignIn = r.between(5, 9*demoDay)
		}
		email := demoAccents.Replace(strings.ToLower(first + "." + last + "@example.com"))
		users = append(users, demoUser{key: email, name: name, email: email, title: title, department: dept.name, location: location, status: status, source: source,
			groups: groups, methods: methods, created: age * demoDay, lastSignIn: lastSignIn})
	}
	return users
}

func demoHome(u *demoUser) demoLocation {
	for _, l := range demoLocations {
		if strings.HasPrefix(l.label, u.location+",") {
			return l
		}
	}
	sum := 0
	for _, c := range u.email {
		sum += int(c)
	}
	return demoLocations[sum%len(demoLocations)]
}

func demoPrimaryMethod(u *demoUser) string {
	for _, m := range u.methods {
		if m.kind != "recovery-codes" {
			return m.kind
		}
	}
	return "magic-link"
}

func demoBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func (s *Store) SeedDemo(ctx context.Context) error {
	return s.Tx(ctx, func(tx *Store) error {
		var exists bool
		if err := tx.db.QueryRow(ctx, `select exists (select 1 from users)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errors.New("the database already has users, so the demo organization was not loaded. Reset the development database with `docker compose -f compose.dev.yml down -v && docker compose -f compose.dev.yml up -d`, then run halo seed-demo again")
		}
		if err := tx.seedDemo(ctx, time.Now()); err != nil {
			return err
		}
		for _, seed := range []func(context.Context) error{tx.SeedPolicies, tx.SeedSAML, tx.SeedProvisioning, tx.SeedGovernance, tx.SeedSettings, tx.SeedFederation, tx.SeedInfra} {
			if err := seed(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) seedDemo(ctx context.Context, now time.Time) error {
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute) }
	agoPtr := func(minutes int) *time.Time {
		if minutes == demoNever {
			return nil
		}
		t := ago(minutes)
		return &t
	}
	list := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}

	groupIDs, assigned := map[string]string{}, map[string]bool{}
	for i, g := range demoGroups {
		groupIDs[g.key], assigned[g.key] = id.New("grp"), g.kind == "assigned"
		var rule *string
		if g.kind == "dynamic" {
			rule = &g.rule
		}
		if _, err := s.db.Exec(ctx, `insert into groups (id, name, description, kind, rule, source, created_at) values ($1, $2, $3, $4, $5, $6, $7)`,
			groupIDs[g.key], g.name, g.description, g.kind, rule, g.source, ago(500*demoDay-i)); err != nil {
			return err
		}
	}

	users := demoDirectory()
	byKey := map[string]*demoUser{}
	for i := range users {
		u := &users[i]
		u.id = id.New("usr")
		byKey[u.key] = u
		if _, err := s.db.Exec(ctx, `insert into users (id, email, name, title, department, location, status, source, created_at, updated_at, last_sign_in_at, email_verified) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10, $11)`,
			u.id, u.email, u.name, u.title, u.department, u.location, u.status, u.source, ago(u.created), agoPtr(u.lastSignIn), u.lastSignIn != demoNever); err != nil {
			return fmt.Errorf("seed %s: %w", u.email, err)
		}
		for _, role := range u.roles {
			if _, err := s.db.Exec(ctx, `insert into user_roles (user_id, role) values ($1, $2)`, u.id, role); err != nil {
				return err
			}
		}
		for _, key := range u.groups {
			if !assigned[key] {
				continue
			}
			if _, err := s.db.Exec(ctx, `insert into group_members (group_id, user_id) values ($1, $2)`, groupIDs[key], u.id); err != nil {
				return err
			}
		}
		for _, m := range u.methods {
			if err := s.seedMethod(ctx, u.id, m, ago(m.addedDays*demoDay), agoPtr(m.usedMinutes)); err != nil {
				return err
			}
		}
	}
	for _, u := range users {
		if u.manager == "" {
			continue
		}
		if _, err := s.db.Exec(ctx, `update users set manager_id = $2 where id = $1`, u.id, byKey[u.manager].id); err != nil {
			return err
		}
	}

	appIDs := map[string]string{}
	for _, a := range demoApps {
		appID := id.New("app")
		appIDs[a.key] = appID
		if _, err := s.db.Exec(ctx, `insert into applications (id, name, description, protocol, app_type, status, client_id, homepage, redirect_uris, post_logout_uris, scopes,
			access_token_ttl, refresh_token_ttl, id_token_ttl, refresh_rotation, setup_guide, owner_id, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18)`,
			appID, a.name, a.description, a.protocol, a.appType, a.status, a.clientID, a.homepage, list(a.redirects), list(a.postLogout), list(a.scopes),
			a.ttl[0], a.ttl[1], a.ttl[2], a.rotation, a.guide, byKey[a.owner].id, ago(a.createdDays*demoDay)); err != nil {
			return err
		}
		for _, key := range a.groups {
			if _, err := s.db.Exec(ctx, `insert into app_groups (app_id, group_id) values ($1, $2)`, appID, groupIDs[key]); err != nil {
				return err
			}
		}
		for _, c := range a.credentials {
			var hash []byte
			if c.kind == "secret" {
				hash = secret.Hash(secret.Token(32))
			}
			if _, err := s.db.Exec(ctx, `insert into app_credentials (id, app_id, kind, label, secret_hash, hint, created_at, expires_at, last_used_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				id.New("crd"), appID, c.kind, c.label, hash, c.hint, ago(c.createdDays*demoDay), ago(-c.expiresDays*demoDay), ago(c.usedMinutes)); err != nil {
				return err
			}
		}
	}

	decorated, err := s.ListUsers(ctx)
	if err != nil {
		return err
	}
	appsOf := map[string][]string{}
	for _, u := range decorated {
		appsOf[u.ID] = u.AppIDs
	}

	r := &demoRand{state: 9001}
	var events []SignInEvent
	event := func(u *demoUser, minutes int, result string) SignInEvent {
		loc := demoHome(u)
		if result != "success" && !r.chance(0.5) {
			loc = pick(r, demoLocations)
		}
		appID := appIDs["outline"]
		if apps := appsOf[u.id]; len(apps) > 0 {
			appID = pick(r, apps)
		}
		e := SignInEvent{Time: ago(minutes), UserID: &u.id, Email: u.email, AppID: &appID, Result: result, Method: demoPrimaryMethod(u),
			IP: fmt.Sprintf("%s%d", loc.prefix, r.between(2, 250)), Location: loc.label, Device: pick(r, demoClients).label, Risk: "none"}
		if result == "success" {
			if r.chance(0.06) {
				e.Risk = "low"
			}
			return e
		}
		e.Risk = pick(r, []string{"low", "low", "medium"})
		e.Reason = "User closed the sign-in window before completing MFA."
		if result == "failure" {
			e.Reason = pick(r, demoFailReasons)
		}
		return e
	}
	for i := range users {
		u := &users[i]
		if u.status != "active" || u.lastSignIn == demoNever || u.lastSignIn > demoWindow {
			continue
		}
		events = append(events, event(u, u.lastSignIn, "success"))
		for n := r.between(0, 3); n > 0; n-- {
			events = append(events, event(u, r.between(u.lastSignIn+5, demoWindow), "success"))
		}
		if r.chance(0.18) {
			minutes := r.between(5, demoWindow)
			result := "interrupted"
			if r.chance(0.75) {
				result = "failure"
			}
			events = append(events, event(u, minutes, result))
		}
	}
	elena, zammad := byKey["elena"], appIDs["zammad"]
	events = append(events, SignInEvent{Time: ago(31), UserID: &elena.id, Email: elena.email, AppID: &zammad, Result: "failure", Method: "totp",
		IP: "185.220.101.47", Location: "Unknown · Tor exit node", Device: "Chrome 141 · Windows", Risk: "high", Reason: "Authenticator code did not match. Sign-in blocked by risk policy."})

	agents := map[string]string{}
	for _, c := range demoClients {
		agents[c.label] = c.agent
	}
	for _, e := range events {
		if err := s.RecordSignIn(ctx, e); err != nil {
			return err
		}
		if e.Result != "success" || e.Time.Before(now.Add(-SessionLifetime)) {
			continue
		}
		lastActive := ago(r.between(0, int(now.Sub(e.Time).Minutes())))
		if _, err := s.db.Exec(ctx, `insert into sessions (id, token_hash, user_id, method, ip, location, user_agent, created_at, last_active_at, expires_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id.New("ses"), secret.Hash(secret.Token(32)), *e.UserID, e.Method, e.IP, e.Location, agents[e.Device], e.Time, lastActive, e.Time.Add(SessionLifetime)); err != nil {
			return err
		}
	}

	resetID, resetLabel := "", "Customer success manager"
	for _, u := range users {
		if u.title == resetLabel && u.status == "active" {
			resetID, resetLabel = u.id, u.name
			break
		}
	}
	audit := func(minutes int, actor, action, summary, targetType, targetID, label, ip string) AuditEvent {
		e := AuditEvent{Time: ago(minutes), Action: action, Summary: summary, TargetType: targetType, TargetID: targetID, TargetLabel: label, IP: ip}
		if actor != "" {
			e.ActorID = &byKey[actor].id
		}
		return e
	}
	for _, e := range []AuditEvent{
		audit(18, "luna", "application.secret.rotate", "Rotated client secret “Staging”", "application", appIDs["forgejo"], "Forgejo", "145.53.112.18"),
		audit(52, "priya", "policy.update", "Added Tor exit nodes to the high-risk network list", "policy", "", "Block high-risk sign-ins", "81.2.69.14"),
		audit(60*3, "", "user.provision", "Provisioned from Workday", "user", byKey["sofia"].id, "Sofia Lindqvist", ""),
		audit(60*5, "marcus", "group.member.add", "Added 2 members", "group", groupIDs["grafana_editors"], "Grafana editors", "102.89.33.20"),
		audit(60*9, "luna", "user.suspend", "Suspended after offboarding ticket OPS-2291", "user", byKey["daniel"].id, "Daniel Mensah", "145.53.112.18"),
		audit(60*20, "priya", "policy.create", "Created policy in report-only mode", "policy", "", "Require trusted device for production", "81.2.69.14"),
		audit(60*26, "tomasz", "application.create", "Registered OAuth service application", "application", appIDs["billing_api"], "Billing API", "93.184.21.77"),
		audit(60*31, "", "session.revoke", "Revoked 3 sessions after password-less migration", "user", byKey["arjun"].id, "Arjun Mehta", ""),
		audit(60*44, "luna", "role.assign", "Assigned Auditor role", "user", byKey["grace"].id, "Grace Liu", "145.53.112.18"),
		audit(60*50, "elena", "user.authentication.reset", "Reset authentication methods", "user", resetID, resetLabel, "88.12.4.9"),
	} {
		if err := s.RecordAudit(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedMethod(ctx context.Context, userID string, m demoMethod, added time.Time, used *time.Time) error {
	var err error
	switch m.kind {
	case "passkey", "security-key":
		_, err = s.db.Exec(ctx, `insert into webauthn_credentials (id, user_id, kind, label, credential_id, public_key, created_at, last_used_at) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id.New("mth"), userID, m.kind, m.label, demoBytes(32), demoBytes(77), added, used)
	case "totp":
		_, err = s.db.Exec(ctx, `insert into totp_secrets (id, user_id, label, secret_sealed, confirmed, created_at, last_used_at) values ($1, $2, $3, $4, true, $5, $6)`,
			id.New("mth"), userID, m.label, s.Sealer.Seal(demoBytes(20)), added, used)
	case "recovery-codes":
		for i := 0; i < 10 && err == nil; i++ {
			var usedAt *time.Time
			if i >= m.remaining {
				t := added.Add(time.Duration(i) * time.Hour)
				usedAt = &t
			}
			_, err = s.db.Exec(ctx, `insert into recovery_codes (id, user_id, code_hash, created_at, used_at) values ($1, $2, $3, $4, $5)`,
				id.New("rec"), userID, secret.Hash(secret.Token(16)), added, usedAt)
		}
	}
	return err
}

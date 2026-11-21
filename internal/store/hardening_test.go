package store_test

import (
	"context"
	"crypto/rand"
	"slices"
	"testing"
	"time"

	"halo/internal/secret"
	"halo/internal/store"
	"halo/internal/testdb"
)

func TestRotateSecretKey(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	u, err := st.CreateUser(ctx, store.NewUser{Email: "ada@example.com", Name: "Ada", Status: "active"})
	must(err)
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}})
	must(err)
	_, err = st.CreateTOTPSecret(ctx, u.ID, "Phone", st.Sealer.Seal([]byte("totp-seed")))
	must(err)
	_, err = st.CreateSigningKey(ctx, "RS256", []byte("oidc-key"), "")
	must(err)
	_, err = st.QueueMail(ctx, "ada@example.com", "Hello", "mail body", "<p>mail body</p>", false)
	must(err)
	must(st.CreateSAMLKey(ctx, []byte("saml-key"), []byte("saml-cert"), ""))
	must(st.SaveAppProvisioning(ctx, app.ID, "https://wiki.example.com/scim", st.Sealer.Seal([]byte("scim-token")), true))
	idp, err := st.SaveIdentityProvider(ctx, store.IdentityProvider{Kind: "oidc", Name: "Acme", Issuer: "https://idp.example.com", ClientID: "halo", Scopes: []string{}, AllowedDomains: []string{}, JITGroupIDs: []string{}}, "client-secret")
	must(err)
	webhookID, _, err := st.CreateWebhook(ctx, store.NewWebhook{URL: "https://hooks.example.com", Events: []string{"user.create"}, Enabled: true})
	must(err)
	ruleID, _, err := st.CreateNotificationRule(ctx, store.NewNotificationRule{Name: "Admins", Destination: "slack", URL: "https://hooks.example.com/rule", Host: "hooks.example.com", Events: []string{"role.update"}, Threshold: 1, WindowMinutes: 10, QuietMinutes: 15})
	must(err)
	must(st.SaveApplicationHook(ctx, store.ApplicationHook{AppID: app.ID, Host: "checks.example.com", URLSealed: st.Sealer.Seal([]byte("https://checks.example.com/hook")), SecretSealed: st.Sealer.Seal([]byte("hook-secret")), Claims: []string{}}))
	must(st.CreateSSHAuthority(ctx, false, []byte("ssh-seed"), "ssh-ed25519 AAAA", ""))
	must(st.CreateSSHAuthority(ctx, true, []byte("host-seed"), "ssh-ed25519 BBBB", ""))
	must(st.ReplaceRecoveryCodes(ctx, u.ID, [][]byte{st.RecoveryCodeHash("abcd")}))
	approvalID, err := st.CreateApprovalRequest(ctx, store.ApprovalRequest{Action: "global-admin", Summary: "Set roles", TargetType: "user", TargetLabel: "Ada", Method: "PUT", Path: "/api/v1/users/x/roles", Body: []byte("approval-body"), State: "null", Requester: store.NamedRef{ID: u.ID}, ExpiresAt: time.Now().Add(time.Hour)})
	must(err)
	must(st.SetBotProtectionSecret(ctx, "bot-secret"))
	must(st.SaveLogExport(ctx, store.LogExport{Protocol: "otlp-http", Endpoint: "https://otel.example.com/v1/logs"}, "Authorization=Bearer otel"))
	nas, err := st.CreateApplication(ctx, store.NewApplication{Name: "NAS", Protocol: "ldap", Type: "web"})
	must(err)
	bindPassword, err := st.RotateLDAPSecret(ctx, nas.ID)
	must(err)
	sourceID, err := st.CreateComplianceSource(ctx, store.ComplianceSource{Name: "Intune", Kind: "intune", TenantID: "contoso", ClientID: "halo", MaxAgeMinutes: 1440}, "intune-secret")
	must(err)
	directoryID, err := st.CreateLDAPDirectory(ctx, store.LDAPDirectory{Name: "Corp", URL: "ldaps://dc1.example.com", Security: "ldaps", BindDN: "cn=halo", UserBaseDN: "dc=example", UserFilter: "(mail=*)", IntervalMinutes: 60}, "bind-password")
	must(err)
	receiver, err := st.CreateApplication(ctx, store.NewApplication{Name: "Signals", Protocol: "oauth", Type: "service"})
	must(err)
	must(st.SaveSignalReceiver(ctx, receiver.ID, "iss_sub"))
	_, err = st.CreateSignalStream(ctx, store.SignalStream{AppID: receiver.ID, DeliveryMethod: store.PushDelivery, EndpointURL: "https://receiver.example.com/events", AuthorizationSealed: st.Sealer.Seal([]byte("Bearer receiver"))})
	must(err)
	vpn, err := st.CreateApplication(ctx, store.NewApplication{Name: "VPN", Protocol: "radius", Type: "web", RedirectURIs: []string{"10.0.0.0/24"}})
	must(err)
	_, shared, err := st.AddSharedSecret(ctx, vpn.ID, "Initial", time.Hour)
	must(err)
	must(st.SaveKerberosSource(ctx, store.KerberosSource{ServicePrincipal: "HTTP/auth.example.com@CORP.EXAMPLE.COM", Realms: []store.KerberosRealm{}, ZoneIDs: []string{}, Keytab: []byte("keytab")}))
	office, _, err := st.CreateOutpost(ctx, "Office")
	must(err)
	host, err := st.CreateApplication(ctx, store.NewApplication{Name: "Database host", Protocol: "remote", Type: "web"})
	must(err)
	hostPassword := "host-password"
	must(st.SaveRemoteTarget(ctx, store.RemoteTarget{AppID: host.ID, OutpostID: office.ID, Protocol: "ssh", Host: "db1.internal", Port: 22, Username: "deploy", Auth: "password", SessionLimit: 3600}, &hostPassword))
	session, err := st.CreateRemoteSession(ctx, store.RemoteSession{AppID: &host.ID, AppName: host.Name, UserID: &u.ID, Email: u.Email, SessionID: "ses_x", OutpostID: &office.ID, Protocol: "ssh", Address: "db1.internal:22", Username: "deploy", LimitSeconds: 3600})
	must(err)
	must(st.EndRemoteSession(ctx, session.ID, "The shell on the host exited.", 1, 2, []byte("recording")))

	key := make([]byte, 32)
	_, _ = rand.Read(key)
	next, err := secret.NewSealer(key)
	must(err)
	rotation, err := st.RotateSecretKey(ctx, next)
	must(err)
	if len(rotation.Resealed) != 25 || slices.ContainsFunc(rotation.Resealed, func(c store.ResealedColumn) bool { return c.Rows != 1 }) {
		t.Fatalf("resealed: %+v", rotation.Resealed)
	}
	if !slices.Equal(rotation.RecoveryCodeUsers, []string{"ada@example.com"}) {
		t.Fatalf("recovery code users: %v", rotation.RecoveryCodeUsers)
	}
	if _, err := st.ListSigningKeys(ctx); err == nil {
		t.Fatal("the old key must no longer open signing keys")
	}

	rotated, err := st.WithKey(key)
	must(err)
	totp, err := rotated.ListTOTPSecrets(ctx, u.ID)
	must(err)
	keys, err := rotated.ListSigningKeys(ctx)
	must(err)
	mail, err := rotated.ListMail(ctx, 10)
	must(err)
	saml, err := rotated.ActiveSAMLKey(ctx)
	must(err)
	provisioning, err := rotated.GetAppProvisioning(ctx, app.ID)
	must(err)
	provider, err := rotated.GetIdentityProvider(ctx, idp.ID)
	must(err)
	webhook, err := rotated.GetWebhook(ctx, webhookID)
	must(err)
	rule, err := rotated.GetNotificationRule(ctx, ruleID)
	must(err)
	hook, err := rotated.GetApplicationHook(ctx, app.ID)
	must(err)
	ssh, err := rotated.GetSSHAuthority(ctx, false)
	must(err)
	hostCA, err := rotated.GetSSHAuthority(ctx, true)
	must(err)
	pending, err := rotated.GetApprovalRequest(ctx, approvalID)
	must(err)
	bot, err := rotated.BotProtectionSecret(ctx)
	must(err)
	source, err := rotated.GetComplianceSource(ctx, sourceID)
	must(err)
	export, err := rotated.GetLogExport(ctx)
	must(err)
	directory, err := rotated.GetLDAPDirectory(ctx, directoryID)
	must(err)
	kerberos, err := rotated.KerberosSource(ctx)
	must(err)
	if clients, err := rotated.RADIUSClients(ctx); err != nil || len(clients) != 1 || string(clients[0].Secrets[0].Value) != shared {
		t.Fatalf("RADIUS shared secret with the new key: %+v %v", clients, err)
	}
	for want, sealed := range map[string][]byte{"totp-seed": totp[0].Sealed, "Authorization=Bearer otel": export.HeadersSealed, "scim-token": provisioning.TokenSealed, "client-secret": provider.SecretSealed, "intune-secret": source.Sealed, "bind-password": directory.Sealed,
		"https://checks.example.com/hook": hook.URLSealed, "hook-secret": hook.SecretSealed} {
		if plain, err := rotated.Sealer.Open(sealed); err != nil || string(plain) != want {
			t.Fatalf("open %s with the new key: %q %v", want, plain, err)
		}
	}
	if _, err := rotated.Sealer.Open(webhook.Sealed); err != nil {
		t.Fatalf("webhook secret: %v", err)
	}
	if plain, err := rotated.Sealer.Open(rule.URLSealed); err != nil || string(plain) != "https://hooks.example.com/rule" {
		t.Fatalf("notification rule URL: %q %v", plain, err)
	}
	if string(keys[0].PrivateKey) != "oidc-key" || mail[0].Text != "mail body" || string(saml.PrivateKey) != "saml-key" || string(ssh.Seed) != "ssh-seed" || string(hostCA.Seed) != "host-seed" || string(pending.Body) != "approval-body" || bot != "bot-secret" || string(kerberos.Keytab) != "keytab" {
		t.Fatalf("rotated values: %q %q %q %q %q %q %q", keys[0].PrivateKey, mail[0].Text, saml.PrivateKey, ssh.Seed, hostCA.Seed, pending.Body, bot)
	}
	if matched, err := rotated.LDAPSecretMatches(ctx, nas.ID, bindPassword); err != nil || !matched {
		t.Fatalf("LDAP bind password: %v %v", matched, err)
	}
	if plain, err := rotated.RemoteCredential(ctx, host.ID); err != nil || plain != hostPassword {
		t.Fatalf("remote host password: %q %v", plain, err)
	}
	if plain, err := rotated.RemoteRecording(ctx, session.ID); err != nil || string(plain) != "recording" {
		t.Fatalf("remote session recording: %q %v", plain, err)
	}
	if _, err := st.RotateSecretKey(ctx, next); err == nil {
		t.Fatal("rotating again with the old key must fail without changing anything")
	}
	if _, err := rotated.ListSigningKeys(ctx); err != nil {
		t.Fatalf("a failed second rotation must leave the values sealed with the new key: %v", err)
	}
}

func TestSigningKeyLifecycle(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	first, err := st.CreateSigningKey(ctx, "RS256", []byte("first"), "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateSigningKey(ctx, "RS256", []byte("second"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RetireSigningKeys(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	info, err := st.ListSigningKeyInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != 2 || info[0].ID != second.ID || !info[0].Active || info[0].RetiresAt != nil || info[1].ID != first.ID || info[1].Active || info[1].RetiredAt != nil ||
		info[1].RetiresAt == nil || !info[1].RetiresAt.Equal(info[0].CreatedAt.Add(store.SigningKeyOverlap)) {
		t.Fatalf("keys during the overlap: %+v", info)
	}
	if keys, err := st.ListSigningKeys(ctx); err != nil || len(keys) != 2 || keys[0].ID != second.ID {
		t.Fatalf("published keys during the overlap: %+v %v", keys, err)
	}
	if err := st.RetireSigningKeys(ctx, time.Now().Add(store.SigningKeyOverlap+time.Minute)); err != nil {
		t.Fatal(err)
	}
	if keys, err := st.ListSigningKeys(ctx); err != nil || len(keys) != 1 || keys[0].ID != second.ID {
		t.Fatalf("published keys after the overlap: %+v %v", keys, err)
	}
	info, err = st.ListSigningKeyInfo(ctx)
	if err != nil || len(info) != 2 || info[1].RetiredAt == nil || info[1].RetiresAt != nil {
		t.Fatalf("retired key: %+v %v", info, err)
	}

	ec, err := st.CreateSigningKey(ctx, "ES256", []byte("ec"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RetireSigningKeys(ctx, time.Now().Add(2*store.SigningKeyOverlap)); err != nil {
		t.Fatal(err)
	}
	if keys, err := st.ListSigningKeys(ctx); err != nil || len(keys) != 2 || keys[0].ID != ec.ID || keys[1].ID != second.ID {
		t.Fatalf("a key of another algorithm must not supersede the RS256 key: %+v %v", keys, err)
	}
	if info, err = st.ListSigningKeyInfo(ctx); err != nil || !info[0].Active || !info[1].Active || info[1].RetiresAt != nil {
		t.Fatalf("each algorithm has its own active key: %+v %v", info, err)
	}
}

func TestSetBasedCounts(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	rule := `user.department == "engineering"`
	eng, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: &rule})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := st.CreateGroup(ctx, store.NewGroup{Name: "Ops", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	for i, u := range []store.NewUser{
		{Email: "a@example.com", Name: "A", Department: "Engineering", Status: "active", GroupIDs: []string{ops.ID}},
		{Email: "b@example.com", Name: "B", Department: "ENGINEERING", Status: "active"},
		{Email: "c@example.com", Name: "C", Department: "Sales", Status: "active", GroupIDs: []string{ops.ID}},
		{Email: "d@example.com", Name: "D", Department: "Engineering", Status: "deprovisioned", GroupIDs: []string{ops.ID}},
	} {
		created, err := st.CreateUser(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		want := map[int][]string{0: {ops.ID, eng.ID}, 1: {eng.ID}, 2: {ops.ID}, 3: {ops.ID, eng.ID}}[i]
		if !slices.Equal(created.GroupIDs, want) {
			t.Fatalf("%s groups: %v, want %v", u.Email, created.GroupIDs, want)
		}
	}
	app, err := st.CreateApplication(ctx, store.NewApplication{Name: "Wiki", Protocol: "oidc", Type: "web", RedirectURIs: []string{"https://wiki.example.com/cb"}, GroupIDs: []string{eng.ID, ops.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if app.UserCount != 3 {
		t.Fatalf("user count: %d, want 3", app.UserCount)
	}
	groups, err := st.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range groups {
		if want := map[string]int{eng.ID: 2, ops.ID: 2}[g.ID]; g.MemberCount != want {
			t.Fatalf("%s members: %d, want %d", g.Name, g.MemberCount, want)
		}
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(users, func(u store.User) bool { return u.Email == "b@example.com" && slices.Equal(u.AppIDs, []string{app.ID}) }) {
		t.Fatalf("app assignment through a dynamic group: %+v", users)
	}
}

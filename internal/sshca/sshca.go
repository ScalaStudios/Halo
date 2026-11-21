package sshca

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"halo/internal/httpx"
	"halo/internal/store"
)

const clockSkew = 5 * time.Minute

var (
	readers          = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers          = []string{"security_admin"}
	principalPattern = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)
	hostnamePattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	extensions       = map[string]string{"permit-X11-forwarding": "", "permit-agent-forwarding": "", "permit-port-forwarding": "", "permit-pty": "", "permit-user-rc": ""}
	errNoPrincipals  = httpx.Fail(http.StatusForbidden, "ERR_NO_PRINCIPALS", "None of your groups maps to a server account, so Halo can't issue an SSH certificate. Ask a security administrator to add a principal mapping for one of your groups.")
	errNoHost        = httpx.Fail(http.StatusNotFound, "ERR_NOT_FOUND", "This host was not found or was revoked. Register it again to sign a key for it.")
	errInvalidToken  = httpx.Fail(http.StatusUnauthorized, "ERR_INVALID_TOKEN", "This enrollment token is not valid. A token works once, for an hour. Create a new one for the host on the Infrastructure page of the Halo console.")
	errUnknownHost   = httpx.Fail(http.StatusForbidden, "ERR_UNKNOWN_HOST", "Halo has no host with this key, or the host was revoked. Enroll it with a new token from the Halo console.")
	errBadSignature  = httpx.Fail(http.StatusUnauthorized, "ERR_INVALID_SIGNATURE", "The renewal was not signed by this host key in the last 5 minutes. Check the host's clock, then try again.")
	errKeyInUse      = httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "This key already belongs to another host in Halo, or to a revoked one. Generate new host keys, then try again.")
)

type handlers struct {
	st *store.Store
}

func Register(mux *http.ServeMux, d httpx.Deps) error {
	h := &handlers{st: d.Store}
	route := func(pattern string, roles []string, fn func(http.ResponseWriter, *http.Request, store.User) error) {
		mux.Handle(pattern, httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
			actor, err := httpx.RequireRole(r, roles...)
			if err != nil {
				return err
			}
			return fn(w, r, actor)
		}))
	}
	mux.Handle("GET /api/v1/ssh/ca.pub", httpx.Handle(h.publicKey))
	mux.Handle("GET /api/v1/ssh/known_hosts", httpx.Handle(h.knownHostsFile))
	mux.Handle("GET /api/v1/ssh/revoked-hosts.krl", httpx.Handle(h.revokedHosts))
	mux.Handle("POST /api/v1/ssh/hosts/enroll", httpx.Handle(h.enrollHost))
	mux.Handle("POST /api/v1/ssh/hosts/renew", httpx.Handle(h.renewHost))
	mux.Handle("POST /api/v1/ssh/hosts/directory", httpx.Handle(h.directory))
	mux.Handle("POST /api/v1/me/ssh/certificates", httpx.Handle(h.issue))
	route("GET /api/v1/ssh/authority", readers, h.authority)
	route("PUT /api/v1/ssh/authority", writers, h.setLifetime)
	route("GET /api/v1/ssh/host-authority", readers, h.hostAuthority)
	route("PUT /api/v1/ssh/host-authority", writers, h.setHostLifetime)
	route("GET /api/v1/ssh/hosts", readers, h.listHosts)
	route("POST /api/v1/ssh/hosts", writers, h.createHost)
	route("POST /api/v1/ssh/hosts/{id}/enrollment-token", writers, h.hostToken)
	route("POST /api/v1/ssh/hosts/{id}/certificate", writers, h.signHost)
	route("POST /api/v1/ssh/hosts/{id}/revoke", writers, h.revokeHost)
	route("PUT /api/v1/ssh/hosts/{id}/host-group", writers, h.setHostGroup)
	route("GET /api/v1/ssh/host-groups", readers, h.listHostGroups)
	route("POST /api/v1/ssh/host-groups", writers, h.createHostGroup)
	route("PUT /api/v1/ssh/host-groups/{id}", writers, h.updateHostGroup)
	route("DELETE /api/v1/ssh/host-groups/{id}", writers, h.deleteHostGroup)
	route("GET /api/v1/ssh/posix", readers, h.posixSettings)
	route("PUT /api/v1/ssh/posix", writers, h.savePOSIXSettings)
	route("GET /api/v1/ssh/principal-mappings", readers, h.listMappings)
	route("POST /api/v1/ssh/principal-mappings", writers, h.createMapping)
	route("PUT /api/v1/ssh/principal-mappings/{id}", writers, h.updateMapping)
	route("DELETE /api/v1/ssh/principal-mappings/{id}", writers, h.deleteMapping)
	route("GET /api/v1/ssh/certificates", readers, h.listCertificates)
	return nil
}

func (h *handlers) ca(ctx context.Context, host bool) (store.SSHAuthority, error) {
	a, err := h.st.GetSSHAuthority(ctx, host)
	if !errors.Is(err, store.ErrNotFound) {
		return a, err
	}
	var seed []byte
	var ref string
	var public crypto.PublicKey
	if h.st.Keys.External() {
		algorithm := "EdDSA"
		if h.st.Keys.Supports(algorithm) != nil {
			algorithm = "ES256"
		}
		if ref, err = h.st.Keys.CreateKey(ctx, algorithm); err != nil {
			return a, err
		}
		signer, err := h.st.Keys.Signer(ctx, ref)
		if err != nil {
			return a, err
		}
		public = signer.Public()
	} else {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return a, err
		}
		seed, public = priv.Seed(), pub
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		return a, err
	}
	if err := h.st.CreateSSHAuthority(ctx, host, seed, authorizedKey(key), ref); err != nil {
		return a, err
	}
	return h.st.GetSSHAuthority(ctx, host)
}

func authorizedKey(key ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

func fingerprint(authorizedKey string) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func (h *handlers) publicKey(w http.ResponseWriter, r *http.Request) error {
	a, err := h.ca(r.Context(), false)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err = fmt.Fprintln(w, a.PublicKey)
	return err
}

func describe(a store.SSHAuthority) map[string]any {
	return map[string]any{"publicKey": a.PublicKey, "fingerprint": fingerprint(a.PublicKey), "createdAt": a.CreatedAt, "certificateLifetime": a.CertificateLifetime}
}

func (h *handlers) authority(w http.ResponseWriter, r *http.Request, _ store.User) error {
	a, err := h.ca(r.Context(), false)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, describe(a))
}

func (h *handlers) hostAuthority(w http.ResponseWriter, r *http.Request, _ store.User) error {
	a, err := h.ca(r.Context(), true)
	if err != nil {
		return err
	}
	body := describe(a)
	if body["knownHosts"], err = h.knownHosts(r.Context()); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, body)
}

func (h *handlers) knownHosts(ctx context.Context) (string, error) {
	a, err := h.st.GetSSHAuthority(ctx, true)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	domains, err := h.st.ListDomains(ctx)
	if err != nil {
		return "", err
	}
	patterns := []string{}
	for _, d := range domains {
		if d.VerifiedAt != nil {
			patterns = append(patterns, d.Name, "*."+d.Name)
		}
	}
	if len(patterns) == 0 {
		return "", nil
	}
	return "@cert-authority " + strings.Join(patterns, ",") + " " + a.PublicKey, nil
}

func (h *handlers) knownHostsFile(w http.ResponseWriter, r *http.Request) error {
	line, err := h.knownHosts(r.Context())
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if line != "" {
		_, err = fmt.Fprintln(w, line)
	}
	return err
}

func (h *handlers) setLifetime(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if err := h.saveLifetime(r, actor, false); err != nil {
		return err
	}
	return h.authority(w, r, actor)
}

func (h *handlers) setHostLifetime(w http.ResponseWriter, r *http.Request, actor store.User) error {
	if err := h.saveLifetime(r, actor, true); err != nil {
		return err
	}
	return h.hostAuthority(w, r, actor)
}

func (h *handlers) saveLifetime(r *http.Request, actor store.User, host bool) error {
	var in struct {
		CertificateLifetime int `json:"certificateLifetime"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	low, high, invalid := 300, 86400, "Set the certificate lifetime between 5 minutes and 24 hours. Short lifetimes are the main control, because sshd cannot check revocation online."
	e := store.AuditEvent{Action: "ssh.lifetime", Summary: "Set the SSH certificate lifetime to ", TargetType: "ssh_authority", TargetID: "ssh", TargetLabel: "SSH certificate authority"}
	if host {
		low, high, invalid = 86400, 31536000, "Set the host certificate lifetime between 1 day and 365 days. Hosts renew theirs with halo ssh-host renew before it ends."
		e = store.AuditEvent{Action: "ssh.host.lifetime", Summary: "Set the SSH host certificate lifetime to ", TargetType: "ssh_host_authority", TargetID: "ssh_host", TargetLabel: "SSH host certificate authority"}
	}
	if in.CertificateLifetime < low || in.CertificateLifetime > high {
		return httpx.Invalid(invalid)
	}
	if _, err := h.ca(r.Context(), host); err != nil {
		return err
	}
	e.Summary += lifetime(in.CertificateLifetime)
	return h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetSSHCertificateLifetime(r.Context(), host, in.CertificateLifetime); err != nil {
			return err
		}
		return record(r, tx, actor, e)
	})
}

func lifetime(seconds int) string {
	d := time.Duration(seconds) * time.Second
	if d%(24*time.Hour) == 0 {
		return plural(int(d/(24*time.Hour)), "day", "days")
	}
	if d%time.Hour == 0 {
		return plural(int(d/time.Hour), "hour", "hours")
	}
	return plural(int(d/time.Minute), "minute", "minutes")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func (h *handlers) listMappings(w http.ResponseWriter, r *http.Request, _ store.User) error {
	mappings, err := h.st.ListSSHPrincipalMappings(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, mappings)
}

func (h *handlers) readMapping(r *http.Request) (store.SSHPrincipalMapping, store.Group, error) {
	var in struct {
		GroupID    string   `json:"groupId"`
		Principals []string `json:"principals"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return store.SSHPrincipalMapping{}, store.Group{}, err
	}
	m := store.SSHPrincipalMapping{GroupID: in.GroupID, Principals: []string{}}
	group, err := h.st.GetGroup(r.Context(), in.GroupID)
	if errors.Is(err, store.ErrNotFound) {
		return m, group, httpx.Invalid("Choose the group whose members get these principals.")
	}
	if err != nil {
		return m, group, err
	}
	for _, p := range in.Principals {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(m.Principals, p) {
			continue
		}
		if !principalPattern.MatchString(p) {
			return m, group, httpx.Invalid(fmt.Sprintf("“%s” is not a valid unix account name. Use up to 32 lowercase letters, digits, _ . or -, starting with a letter or _.", p))
		}
		m.Principals = append(m.Principals, p)
	}
	if len(m.Principals) == 0 || len(m.Principals) > 32 {
		return m, group, httpx.Invalid("Add between 1 and 32 principals, the unix account names people may log in as, such as ops.")
	}
	return m, group, nil
}

func (h *handlers) saveMapping(w http.ResponseWriter, r *http.Request, actor store.User, m store.SSHPrincipalMapping, group store.Group, action string, status int) error {
	var saved store.SSHPrincipalMapping
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if saved, err = tx.SaveSSHPrincipalMapping(r.Context(), m); errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", group.Name+" already has a principal mapping. Edit that one instead.")
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: "Mapped " + group.Name + " to " + strings.Join(m.Principals, ", "), TargetType: "group", TargetID: group.ID, TargetLabel: group.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, saved)
}

func (h *handlers) mapping(r *http.Request) (store.SSHPrincipalMapping, error) {
	mappings, err := h.st.ListSSHPrincipalMappings(r.Context())
	if err != nil {
		return store.SSHPrincipalMapping{}, err
	}
	for _, m := range mappings {
		if m.ID == r.PathValue("id") {
			return m, nil
		}
	}
	return store.SSHPrincipalMapping{}, httpx.NotFound("This principal mapping")
}

func (h *handlers) createMapping(w http.ResponseWriter, r *http.Request, actor store.User) error {
	m, group, err := h.readMapping(r)
	if err != nil {
		return err
	}
	return h.saveMapping(w, r, actor, m, group, "ssh.mapping.create", http.StatusCreated)
}

func (h *handlers) updateMapping(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.mapping(r)
	if err != nil {
		return err
	}
	m, group, err := h.readMapping(r)
	if err != nil {
		return err
	}
	m.ID = old.ID
	return h.saveMapping(w, r, actor, m, group, "ssh.mapping.update", http.StatusOK)
}

func (h *handlers) deleteMapping(w http.ResponseWriter, r *http.Request, actor store.User) error {
	m, err := h.mapping(r)
	if err != nil {
		return err
	}
	group, err := h.st.GetGroup(r.Context(), m.GroupID)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteSSHPrincipalMapping(r.Context(), m.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.mapping.delete", Summary: "Removed the SSH principals " + strings.Join(m.Principals, ", "), TargetType: "group", TargetID: group.ID, TargetLabel: group.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) listCertificates(w http.ResponseWriter, r *http.Request, _ store.User) error {
	certs, err := h.st.ListSSHCertificates(r.Context(), 200)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, certs)
}

func (h *handlers) principals(ctx context.Context, u store.User) ([]string, error) {
	mappings, err := h.st.ListSSHPrincipalMappings(ctx)
	if err != nil {
		return nil, err
	}
	principals := []string{}
	for _, m := range mappings {
		if !slices.Contains(u.GroupIDs, m.GroupID) {
			continue
		}
		for _, p := range m.Principals {
			if !slices.Contains(principals, p) {
				principals = append(principals, p)
			}
		}
	}
	slices.Sort(principals)
	return principals, nil
}

func parseKey(raw string) (ssh.PublicKey, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return nil, httpx.Invalid("Send an OpenSSH public key, such as the contents of ~/.ssh/id_ed25519.pub.")
	}
	if _, isCert := key.(*ssh.Certificate); isCert {
		return nil, httpx.Invalid("That is a certificate, not a public key. Send the public key it belongs to, such as ~/.ssh/id_ed25519.pub.")
	}
	if crypto, ok := key.(ssh.CryptoPublicKey); ok {
		if rsaKey, ok := crypto.CryptoPublicKey().(*rsa.PublicKey); ok && rsaKey.N.BitLen() < 2048 {
			return nil, httpx.Invalid("RSA keys must have at least 2048 bits. Generate an Ed25519 key with ssh-keygen -t ed25519 instead.")
		}
	}
	return key, nil
}

func (h *handlers) issue(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	var in struct {
		PublicKey string `json:"publicKey"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key, err := parseKey(in.PublicKey)
	if err != nil {
		return err
	}
	ctx := r.Context()
	principals, err := h.principals(ctx, u)
	if err != nil {
		return err
	}
	if p, err := h.hostPrincipal(r, u); err != nil {
		return err
	} else if p != "" {
		principals = append(principals, p)
	}
	if len(principals) == 0 {
		return errNoPrincipals
	}
	authority, err := h.ca(ctx, false)
	if err != nil {
		return err
	}
	var cert *ssh.Certificate
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		serial, err := tx.NextSSHSerial(ctx)
		if err != nil {
			return err
		}
		cert = &ssh.Certificate{Key: key, Serial: serial, CertType: ssh.UserCert, KeyId: fmt.Sprintf("halo:%s:%d", u.ID, serial), ValidPrincipals: principals, Permissions: ssh.Permissions{Extensions: extensions}}
		if err := h.sign(ctx, cert, authority); err != nil {
			return err
		}
		c := store.SSHCertificate{Serial: serial, KeyID: cert.KeyId, UserID: &u.ID, Principals: principals, Fingerprint: ssh.FingerprintSHA256(key), KeyType: key.Type(), IP: httpx.ClientIP(r),
			ValidAfter: time.Unix(int64(cert.ValidAfter), 0), ValidBefore: time.Unix(int64(cert.ValidBefore), 0)}
		if err := tx.RecordSSHCertificate(ctx, c); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &u.ID, Action: "ssh.certificate.issue", Summary: fmt.Sprintf("Issued SSH certificate %d for %s, valid %s", serial, strings.Join(principals, ", "), lifetime(authority.CertificateLifetime)),
			TargetType: "user", TargetID: u.ID, TargetLabel: u.Name, IP: c.IP})
	})
	if err != nil {
		return err
	}
	body := issued(cert)
	if body["knownHosts"], err = h.knownHosts(ctx); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, body)
}

func (h *handlers) sign(ctx context.Context, cert *ssh.Certificate, authority store.SSHAuthority) error {
	var key crypto.Signer
	var err error
	if authority.ExternalKey != "" {
		if key, err = h.st.Keys.Signer(ctx, authority.ExternalKey); err != nil {
			return err
		}
	} else {
		key = ed25519.NewKeyFromSeed(authority.Seed)
	}
	signer, err := ssh.NewSignerFromSigner(key)
	if err != nil {
		return err
	}
	now := time.Now()
	cert.ValidAfter = uint64(now.Add(-clockSkew).Unix())
	cert.ValidBefore = uint64(now.Add(time.Duration(authority.CertificateLifetime) * time.Second).Unix())
	return cert.SignCert(rand.Reader, signer)
}

func issued(cert *ssh.Certificate) map[string]any {
	return map[string]any{
		"certificate": authorizedKey(cert), "serial": cert.Serial, "keyId": cert.KeyId, "principals": cert.ValidPrincipals,
		"fingerprint": ssh.FingerprintSHA256(cert.Key), "validAfter": time.Unix(int64(cert.ValidAfter), 0).UTC(), "validBefore": time.Unix(int64(cert.ValidBefore), 0).UTC(),
	}
}

func RenewalMessage(timestamp int64) []byte {
	return fmt.Appendf(nil, "halo ssh host certificate renewal %d", timestamp)
}

func (h *handlers) listHosts(w http.ResponseWriter, r *http.Request, _ store.User) error {
	hosts, err := h.st.ListSSHHosts(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, hosts)
}

func (h *handlers) createHost(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		Hostnames []string `json:"hostnames"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	hostnames := []string{}
	for _, name := range in.Hostnames {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || slices.Contains(hostnames, name) {
			continue
		}
		if !hostnamePattern.MatchString(name) && net.ParseIP(name) == nil {
			return httpx.Invalid(fmt.Sprintf("“%s” is not a hostname or IP address. Use the names people type after ssh, such as web1.example.com, without wildcards or ports.", name))
		}
		hostnames = append(hostnames, name)
	}
	if len(hostnames) == 0 || len(hostnames) > 32 {
		return httpx.Invalid("Add between 1 and 32 hostnames, the names people connect to this host with, such as web1.example.com.")
	}
	return h.enrollmentToken(w, r, actor, http.StatusCreated, "ssh.host.create", "Registered the SSH host ", func(tx *store.Store) (string, error) {
		host, err := tx.CreateSSHHost(r.Context(), hostnames)
		return host.ID, err
	})
}

func (h *handlers) hostToken(w http.ResponseWriter, r *http.Request, actor store.User) error {
	return h.enrollmentToken(w, r, actor, http.StatusOK, "ssh.host.token", "Created an enrollment token for the SSH host ", func(*store.Store) (string, error) {
		return r.PathValue("id"), nil
	})
}

func (h *handlers) enrollmentToken(w http.ResponseWriter, r *http.Request, actor store.User, status int, action, summary string, target func(*store.Store) (string, error)) error {
	var host store.SSHHost
	var token string
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		hostID, err := target(tx)
		if err != nil {
			return err
		}
		if host, token, err = tx.IssueSSHHostToken(r.Context(), hostID); errors.Is(err, store.ErrNotFound) {
			return errNoHost
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: action, Summary: summary + strings.Join(host.Hostnames, ", "), TargetType: "ssh_host", TargetID: host.ID, TargetLabel: host.Hostnames[0]})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, map[string]any{"host": host, "token": token})
}

func (h *handlers) revokeHost(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var host store.SSHHost
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if host, err = tx.RevokeSSHHost(r.Context(), r.PathValue("id")); errors.Is(err, store.ErrNotFound) {
			return errNoHost
		} else if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.host.revoke", Summary: "Revoked the SSH host " + strings.Join(host.Hostnames, ", "), TargetType: "ssh_host", TargetID: host.ID, TargetLabel: host.Hostnames[0]})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, host)
}

func (h *handlers) signHost(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		PublicKey string `json:"publicKey"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key, err := parseKey(in.PublicKey)
	if err != nil {
		return err
	}
	return h.certifyHost(w, r, &actor, key, "for a key signed in the console", errNoHost, func(tx *store.Store) (store.SSHHost, error) {
		return tx.GetSSHHost(r.Context(), r.PathValue("id"))
	})
}

func (h *handlers) enrollHost(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token     string `json:"token"`
		PublicKey string `json:"publicKey"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key, err := parseKey(in.PublicKey)
	if err != nil {
		return err
	}
	return h.certifyHost(w, r, nil, key, "at enrollment", errInvalidToken, func(tx *store.Store) (store.SSHHost, error) {
		return tx.ConsumeSSHHostToken(r.Context(), in.Token)
	})
}

func (h *handlers) renewHost(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		PublicKey string `json:"publicKey"`
		Timestamp int64  `json:"timestamp"`
		Signature string `json:"signature"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	key, err := parseKey(in.PublicKey)
	if err != nil {
		return err
	}
	var sig ssh.Signature
	blob, err := base64.StdEncoding.DecodeString(in.Signature)
	if err != nil || ssh.Unmarshal(blob, &sig) != nil || time.Since(time.Unix(in.Timestamp, 0)).Abs() > clockSkew || key.Verify(RenewalMessage(in.Timestamp), &sig) != nil {
		return errBadSignature
	}
	return h.certifyHost(w, r, nil, key, "at renewal", errUnknownHost, func(tx *store.Store) (store.SSHHost, error) {
		return tx.SSHHostByKey(r.Context(), authorizedKey(key))
	})
}

func (h *handlers) certifyHost(w http.ResponseWriter, r *http.Request, actor *store.User, key ssh.PublicKey, how string, missing error, find func(*store.Store) (store.SSHHost, error)) error {
	ctx := r.Context()
	authority, err := h.ca(ctx, true)
	if err != nil {
		return err
	}
	var cert *ssh.Certificate
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		host, err := find(tx)
		if errors.Is(err, store.ErrNotFound) {
			return missing
		} else if err != nil {
			return err
		}
		serial, err := tx.NextSSHSerial(ctx)
		if err != nil {
			return err
		}
		cert = &ssh.Certificate{Key: key, Serial: serial, CertType: ssh.HostCert, KeyId: fmt.Sprintf("halo:%s:%d", host.ID, serial), ValidPrincipals: host.Hostnames}
		if err := h.sign(ctx, cert, authority); err != nil {
			return err
		}
		if err := tx.CertifySSHHost(ctx, host.ID, authorizedKey(key), ssh.FingerprintSHA256(key), key.Type(), serial, time.Unix(int64(cert.ValidBefore), 0)); errors.Is(err, store.ErrConflict) {
			return errKeyInUse
		} else if err != nil {
			return err
		}
		e := store.AuditEvent{Action: "ssh.host.certificate.issue", Summary: fmt.Sprintf("Issued SSH host certificate %d for %s %s, valid %s", serial, strings.Join(host.Hostnames, ", "), how, lifetime(authority.CertificateLifetime)),
			TargetType: "ssh_host", TargetID: host.ID, TargetLabel: host.Hostnames[0], IP: httpx.ClientIP(r)}
		if actor != nil {
			e.ActorID = &actor.ID
		}
		return tx.RecordAudit(ctx, e)
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, issued(cert))
}

func (h *handlers) revokedHosts(w http.ResponseWriter, r *http.Request) error {
	hosts, err := h.st.ListSSHHosts(r.Context())
	if err != nil {
		return err
	}
	var keys []byte
	var version int64
	for _, host := range hosts {
		if host.RevokedAt == nil || host.PublicKey == nil {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(*host.PublicKey))
		if err != nil {
			return err
		}
		keys = append(keys, ssh.Marshal(struct{ Key []byte }{key.Marshal()})...)
		version = max(version, host.RevokedAt.Unix())
	}
	krl := ssh.Marshal(struct {
		Magic                     uint64
		Format                    uint32
		Version, Generated, Flags uint64
		Reserved, Comment         string
	}{0x5353484b524c0a00, 1, uint64(version), uint64(time.Now().Unix()), 0, "", ""})
	if len(keys) > 0 {
		krl = append(krl, ssh.Marshal(struct {
			Section uint8
			Keys    []byte
		}{2, keys})...)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write(krl)
	return err
}

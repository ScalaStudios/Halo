package sshca

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
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
	extensions       = map[string]string{"permit-X11-forwarding": "", "permit-agent-forwarding": "", "permit-port-forwarding": "", "permit-pty": "", "permit-user-rc": ""}
	errNoPrincipals  = httpx.Fail(http.StatusForbidden, "ERR_NO_PRINCIPALS", "None of your groups maps to a server account, so Halo can't issue an SSH certificate. Ask a security administrator to add a principal mapping for one of your groups.")
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
	mux.Handle("POST /api/v1/me/ssh/certificates", httpx.Handle(h.issue))
	route("GET /api/v1/ssh/authority", readers, h.authority)
	route("PUT /api/v1/ssh/authority", writers, h.setLifetime)
	route("GET /api/v1/ssh/principal-mappings", readers, h.listMappings)
	route("POST /api/v1/ssh/principal-mappings", writers, h.createMapping)
	route("PUT /api/v1/ssh/principal-mappings/{id}", writers, h.updateMapping)
	route("DELETE /api/v1/ssh/principal-mappings/{id}", writers, h.deleteMapping)
	route("GET /api/v1/ssh/certificates", readers, h.listCertificates)
	return nil
}

func (h *handlers) ca(ctx context.Context) (store.SSHAuthority, error) {
	a, err := h.st.GetSSHAuthority(ctx)
	if !errors.Is(err, store.ErrNotFound) {
		return a, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return a, err
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		return a, err
	}
	if err := h.st.CreateSSHAuthority(ctx, priv.Seed(), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))); err != nil {
		return a, err
	}
	return h.st.GetSSHAuthority(ctx)
}

func fingerprint(authorizedKey string) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func (h *handlers) publicKey(w http.ResponseWriter, r *http.Request) error {
	a, err := h.ca(r.Context())
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err = fmt.Fprintln(w, a.PublicKey)
	return err
}

func (h *handlers) authority(w http.ResponseWriter, r *http.Request, _ store.User) error {
	a, err := h.ca(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"publicKey": a.PublicKey, "fingerprint": fingerprint(a.PublicKey), "createdAt": a.CreatedAt, "certificateLifetime": a.CertificateLifetime})
}

func (h *handlers) setLifetime(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in struct {
		CertificateLifetime int `json:"certificateLifetime"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.CertificateLifetime < 300 || in.CertificateLifetime > 86400 {
		return httpx.Invalid("Set the certificate lifetime between 5 minutes and 24 hours. Short lifetimes are the main control, because sshd cannot check revocation online.")
	}
	if _, err := h.ca(r.Context()); err != nil {
		return err
	}
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetSSHCertificateLifetime(r.Context(), in.CertificateLifetime); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ssh.lifetime", Summary: "Set the SSH certificate lifetime to " + lifetime(in.CertificateLifetime), TargetType: "ssh_authority", TargetID: "ssh", TargetLabel: "SSH certificate authority"})
	})
	if err != nil {
		return err
	}
	return h.authority(w, r, actor)
}

func lifetime(seconds int) string {
	d := time.Duration(seconds) * time.Second
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
	if len(principals) == 0 {
		return errNoPrincipals
	}
	authority, err := h.ca(ctx)
	if err != nil {
		return err
	}
	signer, err := ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(authority.Seed))
	if err != nil {
		return err
	}
	var cert *ssh.Certificate
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		serial, err := tx.NextSSHSerial(ctx)
		if err != nil {
			return err
		}
		now := time.Now()
		cert = &ssh.Certificate{
			Key:             key,
			Serial:          serial,
			CertType:        ssh.UserCert,
			KeyId:           fmt.Sprintf("halo:%s:%d", u.ID, serial),
			ValidPrincipals: principals,
			ValidAfter:      uint64(now.Add(-clockSkew).Unix()),
			ValidBefore:     uint64(now.Add(time.Duration(authority.CertificateLifetime) * time.Second).Unix()),
			Permissions:     ssh.Permissions{Extensions: extensions},
		}
		if err := cert.SignCert(rand.Reader, signer); err != nil {
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
	return httpx.JSON(w, http.StatusCreated, map[string]any{
		"certificate": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert))), "serial": cert.Serial, "keyId": cert.KeyId, "principals": principals,
		"fingerprint": ssh.FingerprintSHA256(key), "validAfter": time.Unix(int64(cert.ValidAfter), 0).UTC(), "validBefore": time.Unix(int64(cert.ValidBefore), 0).UTC(),
	})
}

package ldapsync

import (
	"cmp"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	readers    = []string{"security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"}
	writers    = []string{"user_admin"}
	securities = []string{"ldaps", "starttls", "none"}
	attribute  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,63}$`)
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
	route("GET /api/v1/ldap-directories", readers, h.list)
	route("POST /api/v1/ldap-directories", writers, h.create)
	route("PUT /api/v1/ldap-directories/{id}", writers, h.update)
	route("DELETE /api/v1/ldap-directories/{id}", writers, h.remove)
	route("POST /api/v1/ldap-directories/{id}/sync", writers, h.syncNow)
	route("GET /api/v1/ldap-directories/{id}/syncs", readers, h.history)
	if d.Jobs != nil {
		d.Jobs.Every("sync ldap directories", time.Minute, func(ctx context.Context) error { return h.syncDue(ctx, time.Now()) })
	}
	return nil
}

func record(r *http.Request, tx *store.Store, actor store.User, e store.AuditEvent) error {
	e.ActorID, e.IP = &actor.ID, httpx.ClientIP(r)
	return tx.RecordAudit(r.Context(), e)
}

func (h *handlers) directory(ctx context.Context, directoryID string) (store.LDAPDirectory, error) {
	d, err := h.st.GetLDAPDirectory(ctx, directoryID)
	if errors.Is(err, store.ErrNotFound) {
		return d, httpx.NotFound("This LDAP directory")
	}
	return d, err
}

func (h *handlers) list(w http.ResponseWriter, r *http.Request, _ store.User) error {
	directories, err := h.st.ListLDAPDirectories(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, directories)
}

type input struct {
	Name            string            `json:"name"`
	URL             string            `json:"url"`
	Security        string            `json:"security"`
	CACertificate   string            `json:"caCertificate"`
	BindDN          string            `json:"bindDn"`
	Password        string            `json:"password"`
	UserBaseDN      string            `json:"userBaseDn"`
	UserFilter      string            `json:"userFilter"`
	GroupBaseDN     string            `json:"groupBaseDn"`
	GroupFilter     string            `json:"groupFilter"`
	Mapping         store.LDAPMapping `json:"mapping"`
	Enabled         bool              `json:"enabled"`
	IntervalMinutes int               `json:"intervalMinutes"`
}

func validDN(dn string) bool {
	_, err := ldap.ParseDN(dn)
	return dn != "" && len(dn) <= 500 && err == nil
}

func validFilter(filter string) bool {
	_, err := ldap.CompileFilter(filter)
	return len(filter) <= 1000 && err == nil
}

func (h *handlers) validate(ctx context.Context, in input, d store.LDAPDirectory) (store.LDAPDirectory, error) {
	m := in.Mapping
	d.Name, d.URL, d.Security, d.CACertificate, d.BindDN = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL), cmp.Or(in.Security, "ldaps"), strings.TrimSpace(in.CACertificate), strings.TrimSpace(in.BindDN)
	d.UserBaseDN, d.UserFilter, d.GroupBaseDN = strings.TrimSpace(in.UserBaseDN), cmp.Or(strings.TrimSpace(in.UserFilter), "(&(objectClass=person)(mail=*))"), strings.TrimSpace(in.GroupBaseDN)
	d.GroupFilter, d.Enabled, d.IntervalMinutes = strings.TrimSpace(in.GroupFilter), in.Enabled, cmp.Or(in.IntervalMinutes, 60)
	if d.GroupBaseDN != "" {
		d.GroupFilter = cmp.Or(d.GroupFilter, "(|(objectClass=group)(objectClass=groupOfNames))")
		m.GroupName, m.Member = cmp.Or(m.GroupName, "cn"), cmp.Or(m.Member, "member")
	}
	if m.Attributes == nil {
		m.Attributes = map[string]string{}
	}
	d.Mapping = m
	u, err := url.Parse(d.URL)
	scheme := map[bool]string{true: "ldaps", false: "ldap"}[d.Security == "ldaps"]
	pool := x509.NewCertPool()
	switch {
	case d.Name == "" || len(d.Name) > 100:
		return d, httpx.Invalid("Name the directory in up to 100 characters, for example Active Directory.")
	case !slices.Contains(securities, d.Security):
		return d, httpx.Invalid(`Choose how Halo protects the connection: "ldaps", "starttls" or "none".`)
	case err != nil || u.Scheme != scheme || u.Hostname() == "" || u.User != nil || strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || len(d.URL) > 300:
		return d, httpx.Invalid(fmt.Sprintf("Enter the directory server as %s://host or %s://host:port, such as %s://dc1.example.com. LDAPS uses ldaps://, StartTLS and unencrypted connections use ldap://.", scheme, scheme, scheme))
	case d.CACertificate != "" && (len(d.CACertificate) > 65536 || !pool.AppendCertsFromPEM([]byte(d.CACertificate))):
		return d, httpx.Invalid("Paste the certificate authority as PEM, starting with -----BEGIN CERTIFICATE-----, or leave it empty to trust the system's certificate authorities.")
	case !validDN(d.BindDN):
		return d, httpx.Invalid("Enter the bind DN of a read-only account, such as CN=halo-sync,OU=Service Accounts,DC=example,DC=com.")
	case d.ID == "" && in.Password == "":
		return d, httpx.Invalid("Enter the bind account's password.")
	case len(in.Password) > 1024:
		return d, httpx.Invalid("The password is longer than 1,024 characters.")
	case !validDN(d.UserBaseDN):
		return d, httpx.Invalid("Enter the base DN to search for people, such as OU=Staff,DC=example,DC=com.")
	case !validFilter(d.UserFilter):
		return d, httpx.Invalid("The people filter is not a valid LDAP filter. Use one like (&(objectClass=person)(mail=*)).")
	case d.GroupBaseDN != "" && !validDN(d.GroupBaseDN):
		return d, httpx.Invalid("Enter the base DN to search for groups, such as OU=Groups,DC=example,DC=com, or leave it empty to sync people only.")
	case d.GroupBaseDN != "" && !validFilter(d.GroupFilter):
		return d, httpx.Invalid("The group filter is not a valid LDAP filter. Use one like (objectClass=group).")
	case d.IntervalMinutes < 15 || d.IntervalMinutes > 1440:
		return d, httpx.Invalid("Sync between every 15 minutes and once a day (1,440 minutes).")
	}
	if m.ID == "" || m.Email == "" || m.Name == "" {
		return d, httpx.Invalid("Map the entry ID, email and name to directory attributes. Active Directory uses objectGUID, mail and displayName; OpenLDAP uses entryUUID, mail and cn.")
	}
	for _, name := range append([]string{m.ID, m.Email, m.Name, m.Title, m.Department, m.Location, m.Manager, m.GroupName, m.Member}, slices.Collect(maps.Values(m.Attributes))...) {
		if name != "" && !attribute.MatchString(name) {
			return d, httpx.Invalid(fmt.Sprintf("%q is not an LDAP attribute name. Use names like mail, displayName or employeeNumber.", name))
		}
	}
	if len(m.Attributes) == 0 {
		return d, nil
	}
	defined, err := h.st.ListAttributes(ctx)
	if err != nil {
		return d, err
	}
	for name, source := range m.Attributes {
		i := slices.IndexFunc(defined, func(a store.Attribute) bool { return a.Name == name })
		switch {
		case source == "":
			delete(m.Attributes, name)
		case i < 0:
			return d, httpx.Invalid(fmt.Sprintf("There is no user attribute called %q. Add it under Settings, User attributes first.", name))
		case defined[i].SelfService:
			return d, httpx.Invalid(defined[i].Label + " is changed by people themselves, so a directory can't also set it. Turn off self-service for it first.")
		}
	}
	return d, nil
}

func (h *handlers) taken(err error, name string) error {
	if errors.Is(err, store.ErrConflict) {
		return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "An LDAP directory named "+name+" already exists. Pick a different name.")
	}
	return err
}

func (h *handlers) create(w http.ResponseWriter, r *http.Request, actor store.User) error {
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	d, err := h.validate(r.Context(), in, store.LDAPDirectory{})
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if d.ID, err = tx.CreateLDAPDirectory(r.Context(), d, in.Password); err != nil {
			return h.taken(err, d.Name)
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ldap_directory.create", Summary: "Added LDAP directory " + d.URL, TargetType: "ldap_directory", TargetID: d.ID, TargetLabel: d.Name})
	})
	if err != nil {
		return err
	}
	return h.respond(w, r.Context(), d.ID, http.StatusCreated)
}

func (h *handlers) respond(w http.ResponseWriter, ctx context.Context, directoryID string, status int) error {
	d, err := h.st.GetLDAPDirectory(ctx, directoryID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, status, d)
}

func (h *handlers) update(w http.ResponseWriter, r *http.Request, actor store.User) error {
	old, err := h.directory(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	var in input
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	d, err := h.validate(r.Context(), in, old)
	if err != nil {
		return err
	}
	parts := []string{}
	if old.Name != d.Name {
		parts = append(parts, "renamed from “"+old.Name+"”")
	}
	if old.URL != d.URL || old.Security != d.Security || old.CACertificate != d.CACertificate {
		parts = append(parts, "changed the connection")
	}
	if old.BindDN != d.BindDN || in.Password != "" {
		parts = append(parts, "changed the bind account")
	}
	if old.UserBaseDN != d.UserBaseDN || old.UserFilter != d.UserFilter || old.GroupBaseDN != d.GroupBaseDN || old.GroupFilter != d.GroupFilter {
		parts = append(parts, "changed what it searches")
	}
	if !reflect.DeepEqual(old.Mapping, d.Mapping) {
		parts = append(parts, "changed the attribute mapping")
	}
	if old.Enabled != d.Enabled || old.IntervalMinutes != d.IntervalMinutes {
		parts = append(parts, map[bool]string{true: fmt.Sprintf("syncs every %d minutes", d.IntervalMinutes), false: "turned off scheduled syncs"}[d.Enabled])
	}
	summary := cmp.Or(strings.Join(parts, ", "), "saved without changes")
	summary = strings.ToUpper(summary[:1]) + summary[1:]
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateLDAPDirectory(r.Context(), d, in.Password); err != nil {
			return h.taken(err, d.Name)
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ldap_directory.update", Summary: summary, TargetType: "ldap_directory", TargetID: d.ID, TargetLabel: d.Name})
	})
	if err != nil {
		return err
	}
	return h.respond(w, r.Context(), d.ID, http.StatusOK)
}

func (h *handlers) remove(w http.ResponseWriter, r *http.Request, actor store.User) error {
	d, err := h.directory(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("Deleted LDAP directory; %d people and %d groups it synced stay in Halo and can be edited there", d.People, d.Groups)
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteLDAPDirectory(r.Context(), d.ID); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "ldap_directory.delete", Summary: summary, TargetType: "ldap_directory", TargetID: d.ID, TargetLabel: d.Name})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *handlers) syncNow(w http.ResponseWriter, r *http.Request, actor store.User) error {
	d, err := h.directory(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	rep, err := h.sync(r.Context(), d, &actor.ID, r.URL.Query().Get("dryRun") == "true")
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, rep)
}

func (h *handlers) history(w http.ResponseWriter, r *http.Request, _ store.User) error {
	d, err := h.directory(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	syncs, err := h.st.ListLDAPSyncs(r.Context(), d.ID, 50)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, syncs)
}

func (h *handlers) syncDue(ctx context.Context, now time.Time) error {
	directories, err := h.st.ListLDAPDirectories(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range directories {
		if d.Enabled && (d.SyncedAt == nil || now.Sub(*d.SyncedAt) >= time.Duration(d.IntervalMinutes)*time.Minute) {
			_, err := h.sync(ctx, d, nil, false)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

package auth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/types"

	"halo/internal/approval"
	"halo/internal/httpx"
	"halo/internal/store"
)

var kerberosKeyTypes = map[int32]string{17: "aes128-cts-hmac-sha1-96", 18: "aes256-cts-hmac-sha1-96", 19: "aes128-cts-hmac-sha256-128", 20: "aes256-cts-hmac-sha384-192", 23: "rc4-hmac"}

type kerberosKey struct {
	Version   uint32    `json:"version"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"createdAt"`
}

type kerberosView struct {
	store.KerberosSource
	Keys []kerberosKey `json:"keys"`
}

func viewKerberos(k store.KerberosSource) kerberosView {
	v := kerberosView{KerberosSource: k, Keys: []kerberosKey{}}
	kt := keytab.New()
	if kt.Unmarshal(k.Keytab) == nil {
		for _, e := range kt.Entries {
			v.Keys = append(v.Keys, kerberosKey{Version: e.KVNO, Type: kerberosKeyTypes[e.Key.KeyType], CreatedAt: e.Timestamp})
		}
	}
	return v
}

func servicePrincipal(raw string) (string, *httpx.Error) {
	spn, realm := types.ParseSPNString(strings.TrimSpace(raw))
	if len(spn.NameString) != 2 || !strings.EqualFold(spn.NameString[0], "HTTP") || !domainPattern.MatchString(strings.ToLower(spn.NameString[1])) || !domainPattern.MatchString(strings.ToLower(realm)) {
		return "", httpx.Invalid("Enter the service principal as HTTP/<Halo's host name>@<REALM>, for example HTTP/auth.example.com@CORP.EXAMPLE.COM.")
	}
	return "HTTP/" + strings.ToLower(spn.NameString[1]) + "@" + strings.ToUpper(realm), nil
}

func serviceKeys(raw []byte, spn string) ([]byte, *httpx.Error) {
	kt := keytab.New()
	if err := kt.Unmarshal(raw); err != nil {
		return nil, httpx.Invalid("That file isn't a keytab. Export one with ktpass or ktutil and upload the .keytab file.")
	}
	name, realm := types.ParseSPNString(spn)
	kept := kt.Entries[:0]
	for _, e := range kt.Entries {
		if _, ok := kerberosKeyTypes[e.Key.KeyType]; ok && strings.EqualFold(e.Principal.Realm, realm) && slices.EqualFunc(e.Principal.Components, name.NameString, strings.EqualFold) {
			e.Principal.Realm, e.Principal.Components = realm, name.NameString
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		return nil, httpx.Invalid("The keytab has no AES or RC4 key for " + spn + ". Export the keytab for that service principal, or check its spelling and realm.")
	}
	kt.Entries = kept
	out, err := kt.Marshal()
	if err != nil {
		return nil, httpx.Invalid("Halo couldn't read the keys in that keytab. Export it again and upload the new file.")
	}
	return out, nil
}

func (h *handler) getKerberos(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpx.RequireRole(r, providerReaders...); err != nil {
		return err
	}
	k, err := h.st.KerberosSource(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		return httpx.JSON(w, http.StatusOK, nil)
	}
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, viewKerberos(k))
}

func (h *handler) saveKerberos(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	var in struct {
		ServicePrincipal string                `json:"servicePrincipal"`
		Realms           []store.KerberosRealm `json:"realms"`
		ZoneIDs          []string              `json:"zoneIds"`
		Keytab           string                `json:"keytab"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	spn, invalid := servicePrincipal(in.ServicePrincipal)
	if invalid != nil {
		return invalid
	}
	next := store.KerberosSource{ServicePrincipal: spn, Realms: []store.KerberosRealm{}, ZoneIDs: []string{}}
	for _, m := range in.Realms {
		m = store.KerberosRealm{Realm: strings.ToUpper(strings.TrimSpace(m.Realm)), Domain: strings.ToLower(strings.TrimSpace(m.Domain))}
		switch {
		case !domainPattern.MatchString(strings.ToLower(m.Realm)):
			return httpx.Invalid("Enter each realm as its DNS name in capitals, for example CORP.EXAMPLE.COM.")
		case !domainPattern.MatchString(m.Domain):
			return httpx.Invalid("Enter the email domain for " + m.Realm + " without the @, for example example.com.")
		case slices.ContainsFunc(next.Realms, func(x store.KerberosRealm) bool { return x.Realm == m.Realm }):
			return httpx.Invalid(m.Realm + " is listed twice. Map each realm to one email domain.")
		}
		next.Realms = append(next.Realms, m)
	}
	if len(next.Realms) == 0 {
		return httpx.Invalid("Map at least one realm to an email domain, so Halo can match principals to people.")
	}
	zones, err := h.st.ListNetworkZones(ctx)
	if err != nil {
		return err
	}
	for _, id := range in.ZoneIDs {
		if !slices.ContainsFunc(zones, func(z store.NetworkZone) bool { return z.ID == id && z.Kind == "trusted" }) {
			return httpx.Invalid("Automatic Windows sign-in only works from trusted network zones. Refresh the page and choose from the list.")
		}
		if !slices.Contains(next.ZoneIDs, id) {
			next.ZoneIDs = append(next.ZoneIDs, id)
		}
	}
	current, err := h.st.KerberosSource(ctx)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	raw := current.Keytab
	if in.Keytab != "" {
		if raw, err = base64.StdEncoding.DecodeString(in.Keytab); err != nil {
			return httpx.Invalid("Send the keytab file as base64.")
		}
	} else if raw == nil {
		return httpx.Invalid("Upload the keytab for " + spn + ".")
	}
	if next.Keytab, invalid = serviceKeys(raw, spn); invalid != nil {
		return invalid
	}
	summary := "Set up Windows sign-in with the service principal " + spn
	if current.Keytab != nil {
		summary = "Changed Windows sign-in for the service principal " + spn
	}
	if in.Keytab != "" && current.Keytab != nil {
		summary += " and uploaded a new keytab"
	}
	if held, err := h.gate.Hold(w, r, actor, approval.Change{Action: "identity-providers", Summary: summary, TargetType: "kerberos", TargetID: "kerberos", TargetLabel: "Windows sign-in", State: current}); held || err != nil {
		return err
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SaveKerberosSource(ctx, next); err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "kerberos.update", Summary: summary, TargetType: "kerberos", TargetID: "kerberos", TargetLabel: "Windows sign-in", IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	saved, err := h.st.KerberosSource(ctx)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, viewKerberos(saved))
}

func (h *handler) deleteKerberos(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	if held, err := h.gate.Hold(w, r, actor, approval.Change{Action: "identity-providers", Summary: "Remove Windows sign-in and its keytab", TargetType: "kerberos", TargetID: "kerberos", TargetLabel: "Windows sign-in"}); held || err != nil {
		return err
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.DeleteKerberosSource(ctx); errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("Windows sign-in")
		} else if err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "kerberos.delete", Summary: "Removed Windows sign-in and its keytab", TargetType: "kerberos", TargetID: "kerberos", TargetLabel: "Windows sign-in", IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

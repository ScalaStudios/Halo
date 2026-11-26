package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/iana/errorcode"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/messages"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"

	"halo/internal/httpx"
	"halo/internal/store"
)

const (
	kerberosSkew = 5 * time.Minute
	ntlmMech     = "1.3.6.1.4.1.311.2.2.10"
	ntlmReason   = "The browser offered NTLM instead of a Kerberos ticket. Halo only accepts Kerberos, so check that the computer is joined to the domain and the service principal is registered."
)

var (
	errKerberosUnavailable = httpx.Fail(http.StatusNotFound, "ERR_KERBEROS_UNAVAILABLE", "Windows sign-in isn't turned on for this organization. Sign in another way.")
	errKerberosNoTicket    = httpx.Fail(http.StatusUnauthorized, "ERR_KERBEROS_NO_TICKET", "Your browser didn't send a Windows sign-in ticket. Sign in on a computer joined to your organization's domain, or sign in another way.")
	errKerberosRejected    = httpx.Fail(http.StatusForbidden, "ERR_KERBEROS_REJECTED", "Halo couldn't sign you in with Windows. Sign in another way, or ask your administrator to check Windows sign-in.")
)

type principal struct{ name, realm string }

func (p principal) String() string {
	return p.name + "@" + p.realm
}

func (h *handler) kerberosSignIn(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AuthRequest string `json:"authRequest"`
		Automatic   bool   `json:"automatic"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.target(ctx, in.AuthRequest)
	if err != nil {
		return err
	}
	enabled, err := h.st.MethodEnabled(ctx, "kerberos")
	if err != nil {
		return err
	}
	source, err := h.st.KerberosSource(ctx)
	if errors.Is(err, store.ErrNotFound) || err == nil && !enabled {
		return errKerberosUnavailable
	}
	if err != nil {
		return err
	}
	if in.Automatic {
		try, err := h.automatic(ctx, r, source, app)
		if err != nil {
			return err
		}
		if !try {
			return noContent(w)
		}
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Negotiate ")
	if !ok {
		w.Header().Set("WWW-Authenticate", "Negotiate")
		return errKerberosNoTicket
	}
	a := attempt{method: "kerberos", app: app, request: in.AuthRequest}
	p, reason, err := h.acceptTicket(ctx, source, strings.TrimSpace(token), httpx.ClientIP(r))
	if err != nil {
		return err
	}
	if p.name != "" {
		a.email = p.String()
	}
	if reason != "" {
		return h.reject(r, a, reason, errKerberosRejected)
	}
	var link func(*store.Store) error
	userID, err := h.st.KerberosPrincipalUser(ctx, p.String())
	switch {
	case err == nil:
		u, err := h.st.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		a.user = &u
		if i := slices.IndexFunc(source.Realms, func(m store.KerberosRealm) bool { return m.Realm == p.realm }); i >= 0 {
			mapped, err := h.st.GetUserByEmail(ctx, p.name+"@"+source.Realms[i].Domain)
			if err == nil && mapped.ID != u.ID {
				return h.reject(r, a, "The principal "+p.String()+" is linked to "+u.Email+" but its realm now maps it to "+mapped.Email+". Remove the old link before either person signs in with Windows.", errKerberosRejected)
			}
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
	case errors.Is(err, store.ErrNotFound):
		i := slices.IndexFunc(source.Realms, func(m store.KerberosRealm) bool { return m.Realm == p.realm })
		a.email = p.name + "@" + source.Realms[i].Domain
		u, err := h.st.GetUserByEmail(ctx, a.email)
		if errors.Is(err, store.ErrNotFound) || err == nil && u.Kind == "service" {
			return h.reject(r, a, "No person's email address is "+a.email+", where the realm "+p.realm+" maps the principal "+p.String()+".", errKerberosRejected)
		}
		if err != nil {
			return err
		}
		a.user = &u
		if err := h.refuseEmailMatch(r, a, u, errKerberosRejected); err != nil {
			return err
		}
		link = func(tx *store.Store) error {
			if err := tx.LinkKerberosPrincipal(ctx, p.String(), u.ID); err != nil {
				return err
			}
			return audit(ctx, tx, r, u, "user.kerberos.link", u.Name+" linked the Kerberos principal "+p.String())
		}
	default:
		return err
	}
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	release, err := h.lockout(r, a, settings, "passwords or codes")
	if err != nil {
		return err
	}
	defer release()
	return h.finish(w, r, a, nil, link)
}

func (h *handler) automatic(ctx context.Context, r *http.Request, source store.KerberosSource, app *store.Application) (bool, error) {
	addr, err := netip.ParseAddr(httpx.ClientIP(r))
	if err != nil || app != nil && !app.KerberosSignIn {
		return false, nil
	}
	zones, err := h.st.ListNetworkZones(ctx)
	return slices.ContainsFunc(zones, func(z store.NetworkZone) bool {
		return z.Kind == "trusted" && slices.Contains(source.ZoneIDs, z.ID) && z.Contains(addr.Unmap())
	}), err
}

func (h *handler) acceptTicket(ctx context.Context, source store.KerberosSource, token, ip string) (principal, string, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return principal{}, "The browser's Negotiate header isn't base64.", nil
	}
	if bytes.HasPrefix(raw, []byte("NTLMSSP\x00")) {
		return principal{}, ntlmReason, nil
	}
	var neg spnego.SPNEGOToken
	if err := neg.Unmarshal(raw); err != nil || !neg.Init || len(neg.NegTokenInit.MechTypes) == 0 {
		return principal{}, "The browser's Negotiate token isn't an SPNEGO token.", nil
	}
	switch mech := neg.NegTokenInit.MechTypes[0]; {
	case mech.String() == ntlmMech:
		return principal{}, ntlmReason, nil
	case !mech.Equal(gssapi.OIDKRB5.OID()) && !mech.Equal(gssapi.OIDMSLegacyKRB5.OID()):
		return principal{}, "The browser offered " + mech.String() + " instead of Kerberos.", nil
	}
	var krb spnego.KRB5Token
	if err := krb.Unmarshal(neg.NegTokenInit.MechTokenBytes); err != nil || !krb.IsAPReq() {
		return principal{}, "The browser offered Kerberos but sent no ticket.", nil
	}
	req := krb.APReq
	spn, _ := types.ParseSPNString(source.ServicePrincipal)
	if named := req.Ticket.SName.PrincipalNameString() + "@" + req.Ticket.Realm; !strings.EqualFold(named, source.ServicePrincipal) {
		return principal{}, "The ticket is for " + printable(named) + ", not Halo's service principal " + source.ServicePrincipal + ".", nil
	}
	kt := keytab.New()
	if err := kt.Unmarshal(source.Keytab); err != nil {
		return principal{}, "", err
	}
	var addr types.HostAddress
	if parsed := net.ParseIP(ip); parsed != nil {
		addr = types.HostAddressFromNetIP(parsed)
	}
	if ok, err := req.Verify(kt, kerberosSkew, addr, &spn); err != nil || !ok {
		return principal{}, ticketProblem(err, req.Ticket, source.ServicePrincipal), nil
	}
	enc := req.Ticket.DecryptedEncPart
	if req.Authenticator.CRealm != enc.CRealm || !req.Authenticator.CName.Equal(enc.CName) {
		return principal{}, "The ticket's authenticator names a different client than the ticket.", nil
	}
	ctime := req.Authenticator.CTime.Add(time.Duration(req.Authenticator.Cusec) * time.Microsecond)
	digest := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%s\x00%d", strings.ToUpper(source.ServicePrincipal), enc.CRealm, enc.CName.PrincipalNameString(), ctime.UnixMicro()))
	if err := h.st.UseKerberosAuthenticator(ctx, digest[:], time.Now().Add(2*kerberosSkew)); errors.Is(err, store.ErrConflict) {
		return principal{}, "Halo already accepted this ticket's authenticator, so this request is a replay.", nil
	} else if err != nil {
		return principal{}, "", err
	}
	p := principal{name: strings.ToLower(enc.CName.PrincipalNameString()), realm: strings.ToUpper(enc.CRealm)}
	if len(enc.CName.NameString) != 1 || p.name == "" || strings.ContainsAny(p.name, "@/ \t") {
		return p, "The ticket is for " + p.String() + ", which isn't a person's principal.", nil
	}
	if !slices.ContainsFunc(source.Realms, func(m store.KerberosRealm) bool { return m.Realm == p.realm }) {
		return p, "The realm " + p.realm + " isn't one of the realms Windows sign-in maps to people.", nil
	}
	return p, "", nil
}

func printable(s string) string {
	runes := []rune(strings.Map(func(r rune) rune {
		if r == utf8.RuneError || !unicode.IsPrint(r) {
			return '?'
		}
		return r
	}, s))
	return string(runes[:min(len(runes), 200)])
}

func ticketProblem(err error, t messages.Ticket, spn string) string {
	var ke messages.KRBError
	if errors.As(err, &ke) {
		switch ke.ErrorCode {
		case errorcode.KRB_AP_ERR_TKT_EXPIRED:
			return "The Kerberos ticket expired at " + t.DecryptedEncPart.EndTime.UTC().Format(time.RFC3339) + "."
		case errorcode.KRB_AP_ERR_TKT_NYV:
			return "The Kerberos ticket isn't valid yet."
		case errorcode.KRB_AP_ERR_SKEW:
			return "The computer's clock differs from Halo's by more than 5 minutes."
		case errorcode.KRB_AP_ERR_BADADDR:
			return "The Kerberos ticket was issued for a different client address."
		case errorcode.KRB_AP_ERR_BAD_INTEGRITY, errorcode.KRB_AP_ERR_BADMATCH:
			return "The ticket's authenticator doesn't match the ticket."
		}
	}
	return "Halo couldn't decrypt the ticket with the keytab for " + spn + ". Upload a new keytab if the service account's password or key version changed."
}

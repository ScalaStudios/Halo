package mail

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"halo/internal/store"
)

type phrase map[string]string

var months = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

var riskResponses = map[string]phrase{
	"suspend":         {"en": "Halo suspended your account until an administrator restores it.", "es": "Halo suspendió tu cuenta hasta que un administrador la restablezca."},
	"revoke-sessions": {"en": "Halo ended your sessions, so you need to sign in again.", "es": "Halo cerró tus sesiones, así que tienes que volver a iniciar sesión."},
	"require-passkey": {"en": "Your next sign-in needs a passkey or security key.", "es": "Tu próximo inicio de sesión requerirá una clave de acceso o una llave de seguridad."},
}

func firstName(name string) string {
	if first := strings.Fields(name); len(first) > 0 {
		return first[0]
	}
	return ""
}

func when(t time.Time, locale string) string {
	if t.IsZero() {
		return ""
	}
	t = t.UTC()
	if locale == "es" {
		return fmt.Sprintf("%d de %s de %d, %s", t.Day(), months[t.Month()-1], t.Year(), t.Format("15:04 UTC"))
	}
	return t.Format("2 January 2006, 15:04 UTC")
}

func values(vars map[string]any, locale string) map[string]string {
	out := make(map[string]string, len(vars)+1)
	for name, v := range vars {
		switch v := v.(type) {
		case string:
			out[name] = v
		case time.Time:
			out[name] = when(v, locale)
		case phrase:
			out[name] = v[locale]
		}
	}
	return out
}

func join(names []string, and string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + and + names[len(names)-1]
}

func duration(days *int, extends bool) phrase {
	switch {
	case days == nil:
		return phrase{"en": "until revoked", "es": "hasta que se revoque"}
	case extends:
		return phrase{"en": "for " + plural(*days, "more day", "more days"), "es": "por " + plural(*days, "día más", "días más")}
	}
	return phrase{"en": "for " + plural(*days, "day", "days"), "es": "por " + plural(*days, "día", "días")}
}

func Invite(to, name, inviter, link string, expires time.Time) Message {
	return build("invite", to, map[string]any{"Name": firstName(name), "Administrator": inviter, "Link": link, "Expires": expires})
}

func Reset(to, name, admin, link string, expires time.Time) Message {
	return build("reset", to, map[string]any{"Name": firstName(name), "Administrator": admin, "Link": link, "Expires": expires})
}

func SignUp(to, name, link string, expires time.Time) Message {
	return build("sign_up", to, map[string]any{"Name": firstName(name), "Link": link, "Expires": expires})
}

func AccessPass(to, name, admin string, expires time.Time) Message {
	return build("access_pass", to, map[string]any{"Name": firstName(name), "Administrator": admin, "Expires": expires})
}

func MagicLink(to, name, app, link, code string) Message {
	vars := map[string]any{"Name": firstName(name), "Application": app, "Link": link, "Code": code}
	if code == "" {
		return build("magic_link", to, vars)
	}
	return build("magic_code", to, vars)
}

func AccessRequested(to, requester, requesterEmail, requestedBy, pkg string, days *int, extends bool, reason, answers, link string) Message {
	return build("access_request", to, map[string]any{"Requester": requester, "RequesterEmail": requesterEmail, "RequestedBy": requestedBy, "Package": pkg, "Duration": duration(days, extends), "Reason": reason, "Answers": answers, "Link": link})
}

func AccessRequestedForYou(to, requestedBy, pkg string, days *int, extends bool, link string) Message {
	return build("access_request_for_you", to, map[string]any{"RequestedBy": requestedBy, "Package": pkg, "Duration": duration(days, extends), "Link": link})
}

func AccessExpired(to, pkg, link string) Message {
	return build("access_expired", to, map[string]any{"Package": pkg, "Link": link})
}

func AccessDecided(to string, approved bool, approver, pkg string, expires *time.Time, groups []store.NamedRef, note, link string) Message {
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	vars := map[string]any{"Approver": approver, "Package": pkg, "Expires": time.Time{}, "Groups": phrase{"en": join(names, " and "), "es": join(names, " y ")}, "Note": note, "Link": link}
	if approver == "" {
		vars["Approver"] = phrase{"en": "An approver", "es": "Un aprobador"}
	}
	if expires != nil {
		vars["Expires"] = *expires
	}
	if approved {
		return build("access_approved", to, vars)
	}
	return build("access_denied", to, vars)
}

func GuestExpiring(to, guest, guestEmail, link string, expires time.Time) Message {
	return build("guest_expiring", to, map[string]any{"Guest": guest, "GuestEmail": guestEmail, "Expires": expires, "Link": link})
}

func NewSignIn(to, name, app, device, ip, link string, at time.Time) Message {
	return build("new_sign_in", to, map[string]any{"Name": firstName(name), "Application": app, "Device": device, "IP": ip, "Link": link, "Time": at})
}

func MethodAdded(to, name, method, link string, at time.Time) Message {
	return build("method_added", to, map[string]any{"Name": firstName(name), "Method": method, "Link": link, "Time": at})
}

func PasswordAdded(to, name, link string, at time.Time) Message {
	return build("method_added", to, map[string]any{"Name": firstName(name), "Method": phrase{"en": "A new password", "es": "Una nueva contraseña"}, "Link": link, "Time": at})
}

func MethodRemoved(to, name, method, admin, link string, at time.Time) Message {
	return build("method_removed", to, map[string]any{"Name": firstName(name), "Method": method, "Administrator": admin, "Link": link, "Time": at})
}

func RecoveryCodes(to, name, link string, at time.Time) Message {
	return build("recovery_codes", to, map[string]any{"Name": firstName(name), "Link": link, "Time": at})
}

func EmailChanged(name, oldEmail, newEmail, admin, link string, at time.Time) []Message {
	vars := map[string]any{"Name": firstName(name), "OldEmail": oldEmail, "NewEmail": newEmail, "Administrator": admin, "Link": link, "Time": at}
	if strings.EqualFold(oldEmail, newEmail) {
		return []Message{build("email_changed", newEmail, vars)}
	}
	old := build("email_changed", oldEmail, vars)
	old.lookup = newEmail
	return []Message{old, build("email_changed", newEmail, vars)}
}

func AccountLocked(to, name string, attempts int, until time.Time, link string) Message {
	return build("account_locked", to, map[string]any{"Name": firstName(name), "Attempts": strconv.Itoa(attempts), "Until": until, "Link": link})
}

func AccountLockedAdmin(to, person, personEmail string, attempts int, until time.Time, link string) Message {
	return build("account_locked_admin", to, map[string]any{"Person": person, "PersonEmail": personEmail, "Attempts": strconv.Itoa(attempts), "Until": until, "Link": link})
}

func Impersonation(to, name, admin, purpose, link string, until time.Time) Message {
	return build("impersonation", to, map[string]any{"Name": firstName(name), "Administrator": admin, "Purpose": purpose, "Until": until, "Link": link})
}

func RiskResponse(to, name, risk string, actions []string, link string, at time.Time) Message {
	response := phrase{}
	for _, locale := range store.Locales {
		sentences := make([]string, len(actions))
		for i, action := range actions {
			sentences[i] = riskResponses[action][locale]
		}
		response[locale] = strings.Join(sentences, " ")
	}
	return build("risk_response", to, map[string]any{"Name": firstName(name), "Risk": risk, "Response": response, "Link": link, "Time": at})
}

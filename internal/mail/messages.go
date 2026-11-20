package mail

import (
	"strings"
	"time"
)

func firstName(name string) string {
	if first := strings.Fields(name); len(first) > 0 {
		return first[0]
	}
	return ""
}

func greeting(name string) string {
	if first := firstName(name); first != "" {
		return "Hi " + first + ",\n\n"
	}
	return "Hi,\n\n"
}

func expiry(t time.Time) string {
	return t.UTC().Format("2 January 2006, 15:04 UTC")
}

func Invite(to, name, inviter, link string, expires time.Time) Message {
	return build("invite", to, map[string]string{"Name": firstName(name), "Administrator": inviter, "Link": link, "Expires": expiry(expires)})
}

func Reset(to, name, admin, link string, expires time.Time) Message {
	return build("reset", to, map[string]string{"Name": firstName(name), "Administrator": admin, "Link": link, "Expires": expiry(expires)})
}

func SignUp(to, name, link string, expires time.Time) Message {
	return build("sign_up", to, map[string]string{"Name": firstName(name), "Link": link, "Expires": expiry(expires)})
}

func AccessPass(to, name, admin string, expires time.Time) Message {
	return build("access_pass", to, map[string]string{"Name": firstName(name), "Administrator": admin, "Expires": expiry(expires)})
}

func MagicLink(to, name, app, link, code string) Message {
	vars := map[string]string{"Name": firstName(name), "Application": app, "Link": link, "Code": code}
	if code == "" {
		return build("magic_link", to, vars)
	}
	return build("magic_code", to, vars)
}

func AccessRequested(to, requester, requesterEmail, pkg, duration, reason, link string) Message {
	return build("access_request", to, map[string]string{"Requester": requester, "RequesterEmail": requesterEmail, "Package": pkg, "Duration": duration, "Reason": reason, "Link": link})
}

func AccessDecided(to string, approved bool, approver, pkg, expires, groups, note, link string) Message {
	vars := map[string]string{"Approver": approver, "Package": pkg, "Expires": expires, "Groups": groups, "Note": note, "Link": link}
	if approved {
		return build("access_approved", to, vars)
	}
	return build("access_denied", to, vars)
}

func GuestExpiring(to, guest, guestEmail, link string, expires time.Time) Message {
	return build("guest_expiring", to, map[string]string{"Guest": guest, "GuestEmail": guestEmail, "Expires": expiry(expires), "Link": link})
}

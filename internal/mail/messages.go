package mail

import (
	"strings"
	"time"
)

func greeting(name string) string {
	if first := strings.Fields(name); len(first) > 0 {
		return "Hi " + first[0] + ",\n\n"
	}
	return "Hi,\n\n"
}

func expiry(t time.Time) string {
	return t.UTC().Format("2 January 2006, 15:04 UTC")
}

func Invite(to, name, inviter, link string, expires time.Time) Message {
	return Message{To: to, Subject: "Set up your Halo account", Text: greeting(name) +
		inviter + " invited you to Halo, where you sign in to your work applications with a passkey instead of a password.\n\n" +
		"Set up your passkey with this link:\n\n" + link + "\n\n" +
		"The link works once and expires " + expiry(expires) + ". If you weren't expecting an invitation, you can ignore this email."}
}

func Reset(to, name, admin, link string, expires time.Time) Message {
	return Message{To: to, Subject: "Reset how you sign in to Halo", Text: greeting(name) +
		admin + " reset how you sign in to Halo. Your passkeys, security keys, authenticator apps and recovery codes no longer work, and you were signed out everywhere.\n\n" +
		"Set up a new passkey with this link:\n\n" + link + "\n\n" +
		"The link works once and expires " + expiry(expires) + ". If you didn't expect this, contact your administrator before you use it."}
}

func MagicLink(to, name, app, link string) Message {
	target, browser := "Halo", ""
	if app != "" {
		target, browser = app, " Open it in the browser where you started signing in."
	}
	return Message{To: to, Subject: "Your Halo sign-in link", Text: greeting(name) +
		"Open this link to sign in to " + target + ":\n\n" + link + "\n\n" +
		"It works once and expires in 10 minutes." + browser + " If you didn't ask to sign in, you can ignore this email."}
}

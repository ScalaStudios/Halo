package mail

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"halo/internal/store"
)

const securityWindow = 15 * time.Minute

func SendSecurity(ctx context.Context, mailer Mailer, st *store.Store, userID, subject string, msgs ...Message) {
	settings, err := st.Settings(ctx)
	if err != nil {
		slog.WarnContext(ctx, "security email not sent", "user", userID, "error", err)
		return
	}
	msgs = slices.DeleteFunc(msgs, func(m Message) bool {
		return m.To == "" || slices.Contains(settings.SecurityEmailsOff, m.kind) || m.kind == "account_locked_admin" && !settings.LockoutEmailAdmins
	})
	if len(msgs) == 0 {
		return
	}
	window := securityWindow
	if msgs[0].kind == "account_locked" || msgs[0].kind == "account_locked_admin" {
		window = time.Duration(settings.LockoutMinutes) * time.Minute
	}
	claimed, err := st.ClaimSecurityEmail(ctx, userID, msgs[0].kind, subject, time.Now(), window)
	if err != nil {
		slog.WarnContext(ctx, "security email not sent", "user", userID, "kind", msgs[0].kind, "error", err)
	}
	if !claimed {
		return
	}
	for _, m := range msgs {
		if err := mailer.Send(ctx, m); err != nil {
			slog.WarnContext(ctx, "security email not delivered", "user", userID, "kind", m.kind, "error", err)
		}
	}
}

package policy

import (
	"context"
	"slices"
	"strings"
	"time"

	"halo/internal/mail"
	"halo/internal/store"
)

const riskSettle = 10 * time.Second

var responseOrder = []string{"suspend", "revoke-sessions", "require-passkey", "notify"}

type riskNotice struct {
	user     store.User
	event    store.RiskEvent
	response []string
}

func respond(ctx context.Context, st *store.Store, mailer mail.Mailer, issuer string, now time.Time) error {
	var notices []riskNotice
	err := st.Tx(ctx, func(tx *store.Store) error {
		events, err := tx.ClaimRiskEvents(ctx, now.Add(-riskSettle), 100)
		if err != nil || len(events) == 0 {
			return err
		}
		settings, err := tx.CurrentSettings(ctx)
		if err != nil {
			return err
		}
		for _, e := range events {
			if e.UserID == nil || slices.Contains(settings.EmergencyAccessIDs, *e.UserID) {
				continue
			}
			actions := slices.DeleteFunc(slices.Clone(responseOrder), func(action string) bool {
				return !slices.ContainsFunc(settings.RiskResponses, func(r store.RiskResponse) bool {
					return r.Action == action && (r.Signal == "" || r.Signal == e.Type) && slices.Index(riskLevels, e.Level) >= slices.Index(riskLevels, r.Level)
				})
			})
			if len(actions) == 0 {
				continue
			}
			u, err := tx.GetUser(ctx, *e.UserID)
			if err != nil {
				return err
			}
			steps, response, ended, notify := []string{}, []string{}, false, false
			for _, action := range actions {
				switch action {
				case "suspend":
					last, err := tx.IsLastActiveGlobalAdmin(ctx, u.ID)
					if err != nil {
						return err
					}
					switch {
					case last:
						steps = append(steps, "Did not suspend "+u.Name+", the last active global administrator")
						continue
					case u.Status == "suspended" || u.Status == "deprovisioned":
						steps = append(steps, "Did not suspend "+u.Name+", who is already "+u.Status)
						continue
					}
					if err := tx.SetUserStatus(ctx, u.ID, "suspended"); err != nil {
						return err
					}
					n, err := tx.RevokeUserSessions(ctx, u.ID, "")
					if err != nil {
						return err
					}
					steps, response, ended = append(steps, "Suspended the account and ended "+plural(n, "session", "sessions")), append(response, action), true
				case "revoke-sessions", "require-passkey":
					if !ended {
						n, err := tx.RevokeUserSessions(ctx, u.ID, "")
						if err != nil {
							return err
						}
						steps, response, ended = append(steps, "Ended "+plural(n, "session", "sessions")+" and revoked their tokens"), append(response, "revoke-sessions"), true
					}
					if action == "require-passkey" {
						if err := tx.SetPasskeyRequired(ctx, u.ID, true); err != nil {
							return err
						}
						steps, response = append(steps, "Asked for a passkey or security key at the next sign-in"), append(response, action)
					}
				case "notify":
					if notify = !slices.Contains(settings.SecurityEmailsOff, "risk_response"); notify {
						steps = append(steps, "Emailed "+u.Email)
					}
				}
			}
			if notify {
				notices = append(notices, riskNotice{user: u, event: e, response: response})
			}
			if len(steps) == 0 {
				continue
			}
			err = tx.RecordAudit(ctx, store.AuditEvent{Action: "risk_event.respond", Summary: riskTitles[e.Type] + ": " + strings.Join(steps, "; "), TargetType: "risk_event", TargetID: e.ID, TargetLabel: u.Name})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, n := range notices {
		mail.SendSecurity(ctx, mailer, st, n.user.ID, n.event.Type, mail.RiskResponse(n.user.Email, n.user.Name, n.event.Detail, n.response, issuer+"/account/activity", n.event.Time))
	}
	return nil
}

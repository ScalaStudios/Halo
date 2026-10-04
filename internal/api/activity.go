package api

import (
	"cmp"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

func markCurrent(r *http.Request, sessions []store.Session) []store.Session {
	if current, ok := httpx.CurrentSession(r); ok {
		for i := range sessions {
			sessions[i].Current = sessions[i].ID == current.ID
		}
	}
	return sessions
}

func limit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 1000 {
		return 0, httpx.Invalid("limit must be a whole number between 1 and 1000.")
	}
	return n, nil
}

func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request, _ store.User) error {
	sessions, err := h.st.ListSessions(r.Context(), "")
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, markCurrent(r, sessions))
}

func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request, actor store.User) error {
	sess, err := h.st.GetSession(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This session")
	}
	if current, ok := httpx.CurrentSession(r); ok && current.ID == sess.ID {
		return httpx.Invalid("This is the session you are using right now. Sign out from the account menu instead.")
	}
	owner, err := h.st.GetUser(r.Context(), sess.UserID)
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.RevokeSession(r.Context(), sess.ID, ""); err != nil {
			return found(err, "This session")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "session.revoke", Summary: "Ended the " + sess.Browser + " session on " + sess.Device, TargetType: "user", TargetID: owner.ID, TargetLabel: owner.Name})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (h *handlers) listSignIns(w http.ResponseWriter, r *http.Request, _ store.User) error {
	n, err := limit(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	events, err := h.st.ListSignIns(r.Context(), store.SignInFilter{UserID: q.Get("user"), AppID: q.Get("app"), Limit: n})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, events)
}

func (h *handlers) listAudit(w http.ResponseWriter, r *http.Request, _ store.User) error {
	n, err := limit(r)
	if err != nil {
		return err
	}
	events, err := h.st.ListAudit(r.Context(), n)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, events)
}

type weakAdmin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type expiringCredential struct {
	AppID        string     `json:"appId"`
	AppName      string     `json:"appName"`
	CredentialID string     `json:"credentialId"`
	Label        string     `json:"label"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt"`
	DaysLeft     int        `json:"daysLeft"`
}

type failureRate struct {
	AppID         string  `json:"appId"`
	AppName       string  `json:"appName"`
	FailureRate7d float64 `json:"failureRate7d"`
}

func (h *handlers) overview(w http.ResponseWriter, r *http.Request, _ store.User) error {
	ctx := r.Context()
	now := time.Now()
	users, err := h.st.ListUsers(ctx)
	if err != nil {
		return err
	}
	apps, err := h.st.ListApplications(ctx)
	if err != nil {
		return err
	}
	highRisk, err := h.st.ListHighRiskSignIns(ctx, now.Add(-7*24*time.Hour))
	if err != nil {
		return err
	}
	recent, err := h.st.ListSignIns(ctx, store.SignInFilter{Limit: 8})
	if err != nil {
		return err
	}

	weak := []weakAdmin{}
	activeUsers := 0
	for _, u := range users {
		if u.Status != "active" {
			continue
		}
		activeUsers++
		if len(u.RoleKeys) > 0 && u.Strength != "phishing-resistant" {
			weak = append(weak, weakAdmin{u.ID, u.Name})
		}
	}

	expiring := []expiringCredential{}
	failures := []failureRate{}
	activeApps := 0
	for _, a := range apps {
		if a.FailureRate7d >= 1 {
			failures = append(failures, failureRate{a.ID, a.Name, a.FailureRate7d})
		}
		if a.Status != "active" {
			continue
		}
		activeApps++
		for _, c := range a.Credentials {
			superseded := slices.ContainsFunc(a.Credentials, func(o store.Credential) bool { return o.Kind == c.Kind && o.CreatedAt.After(c.CreatedAt) })
			if superseded || !c.ExpiresAt.After(now) || c.ExpiresAt.After(now.Add(30*24*time.Hour)) {
				continue
			}
			days := int(math.Ceil(c.ExpiresAt.Sub(now).Hours() / 24))
			expiring = append(expiring, expiringCredential{a.ID, a.Name, c.ID, c.Label, c.ExpiresAt, c.LastUsedAt, days})
		}
	}
	slices.SortFunc(expiring, func(a, b expiringCredential) int { return a.ExpiresAt.Compare(b.ExpiresAt) })
	slices.SortFunc(failures, func(a, b failureRate) int { return cmp.Compare(b.FailureRate7d, a.FailureRate7d) })

	return httpx.JSON(w, http.StatusOK, map[string]any{
		"weakAdmins":          weak,
		"expiringCredentials": expiring,
		"highRiskSignIns":     highRisk,
		"recentSignIns":       recent,
		"failureRates":        failures,
		"counts": map[string]int{
			"users":              len(users),
			"activeUsers":        activeUsers,
			"applications":       len(apps),
			"activeApplications": activeApps,
		},
	})
}

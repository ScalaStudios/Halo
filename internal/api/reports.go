package api

import (
	"cmp"
	"encoding/csv"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

type period struct {
	From               time.Time `json:"from"`
	To                 time.Time `json:"to"`
	RetentionDays      int       `json:"retentionDays"`
	LimitedByRetention bool      `json:"limitedByRetention"`
}

func (h *handlers) period(r *http.Request) (period, error) {
	now := time.Now()
	p := period{From: now.AddDate(0, 0, -30), To: now}
	for name, dst := range map[string]*time.Time{"from": &p.From, "to": &p.To} {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t, err = time.Parse(time.DateOnly, raw)
		}
		if err != nil {
			return p, httpx.Invalid(name + " must be a date such as 2026-11-01 or a time such as 2026-11-01T09:00:00Z.")
		}
		*dst = t
	}
	settings, err := h.st.Settings(r.Context())
	if err != nil {
		return p, err
	}
	p.RetentionDays = settings.SignInLogDays
	if cutoff := now.AddDate(0, 0, -p.RetentionDays); p.RetentionDays > 0 && p.From.Before(cutoff) {
		p.From, p.LimitedByRetention = cutoff, true
	}
	if !p.From.Before(p.To) {
		if p.LimitedByRetention {
			return p, httpx.Invalid(fmt.Sprintf("The sign-in log keeps %s, so choose a period that ends after %s.", plural(p.RetentionDays, "day"), p.From.UTC().Format(time.DateOnly)))
		}
		return p, httpx.Invalid("from must be before to.")
	}
	return p, nil
}

func wantsCSV(r *http.Request) bool {
	return r.URL.Query().Get("format") == "csv"
}

func writeCSV(w http.ResponseWriter, name string, head []string, rows [][]string) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="halo-`+name+`-`+time.Now().UTC().Format(time.DateOnly)+`.csv"`)
	out := csv.NewWriter(w)
	for _, row := range append([][]string{head}, rows...) {
		for i, cell := range row {
			if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
				row[i] = "'" + cell
			}
		}
		if err := out.Write(row); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}

func csvTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (h *handlers) reportGroup(r *http.Request) (string, map[string]string, error) {
	groups, err := h.st.ListGroupsRaw(r.Context())
	if err != nil {
		return "", nil, err
	}
	names := map[string]string{}
	for _, g := range groups {
		names[g.ID] = g.Name
	}
	group := r.URL.Query().Get("group")
	if _, ok := names[group]; group != "" && !ok {
		return "", nil, httpx.Invalid(fmt.Sprintf("Group %q does not exist. Choose a group from the list.", group))
	}
	return group, names, nil
}

var registeredKinds = []string{"passkey", "security-key", "totp", "password", "recovery-codes"}

var registrationFilters = []string{"no-passkey", "password-only", "authenticator-only", "no-method"}

func registrationMatches(filter string, has map[string]bool) bool {
	primary := ""
	for _, kind := range []string{"passkey", "security-key", "totp", "password"} {
		if has[kind] {
			primary += kind + " "
		}
	}
	switch filter {
	case "no-passkey":
		return !has["passkey"] && !has["security-key"]
	case "password-only":
		return primary == "password "
	case "authenticator-only":
		return primary == "totp "
	case "no-method":
		return primary == ""
	}
	return true
}

type registrationRow struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Department   string     `json:"department"`
	Methods      []string   `json:"methods"`
	Strength     string     `json:"strength"`
	LastSignInAt *time.Time `json:"lastSignInAt"`
}

type methodCount struct {
	Method string `json:"method"`
	People int    `json:"people"`
}

func (h *handlers) methodRegistrationReport(w http.ResponseWriter, r *http.Request, _ store.User) error {
	filter := r.URL.Query().Get("filter")
	if filter != "" && !slices.Contains(registrationFilters, filter) {
		return httpx.Invalid("filter must be no-passkey, password-only, authenticator-only or no-method.")
	}
	group, _, err := h.reportGroup(r)
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	rows := []registrationRow{}
	people, filters, methods := 0, map[string]int{}, make([]methodCount, len(registeredKinds))
	for i, kind := range registeredKinds {
		methods[i].Method = kind
	}
	for _, u := range users {
		if u.Status != "active" || group != "" && !slices.Contains(u.GroupIDs, group) {
			continue
		}
		people++
		has := map[string]bool{}
		for _, m := range u.Methods {
			has[m.Kind] = true
		}
		kinds := []string{}
		for i, kind := range registeredKinds {
			if has[kind] {
				kinds = append(kinds, kind)
				methods[i].People++
			}
		}
		for _, name := range registrationFilters {
			if registrationMatches(name, has) {
				filters[name]++
			}
		}
		if registrationMatches(filter, has) {
			rows = append(rows, registrationRow{u.ID, u.Name, u.Email, u.Department, kinds, u.Strength, u.LastSignInAt})
		}
	}
	if wantsCSV(r) {
		lines := make([][]string, len(rows))
		for i, p := range rows {
			lines[i] = []string{p.Name, p.Email, p.Department, strings.Join(p.Methods, " "), p.Strength, csvTime(p.LastSignInAt), p.ID}
		}
		return writeCSV(w, "sign-in-methods", []string{"name", "email", "department", "methods", "strength", "last_sign_in", "user_id"}, lines)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"people": rows, "methods": methods, "counts": map[string]any{"people": people, "filters": filters}})
}

type methodUsage struct {
	Method string `json:"method"`
	store.SignInCount
}

func (h *handlers) methodUsageReport(w http.ResponseWriter, r *http.Request, _ store.User) error {
	p, err := h.period(r)
	if err != nil {
		return err
	}
	counts, err := h.st.SignInsByMethod(r.Context(), p.From, p.To)
	if err != nil {
		return err
	}
	rows := []methodUsage{}
	for _, kind := range append(slices.Clone(store.MethodKinds), "access-pass") {
		rows = append(rows, methodUsage{kind, counts[kind]})
	}
	if wantsCSV(r) {
		lines := make([][]string, len(rows))
		for i, m := range rows {
			lines[i] = []string{m.Method, strconv.Itoa(m.SignIns), strconv.Itoa(m.People), csvTime(m.LastSignInAt)}
		}
		return writeCSV(w, "sign-in-method-usage", []string{"method", "sign_ins", "people", "last_sign_in"}, lines)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"period": p, "methods": rows})
}

type inactivePerson struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Email        string           `json:"email"`
	Status       string           `json:"status"`
	Department   string           `json:"department"`
	LastSignInAt *time.Time       `json:"lastSignInAt"`
	Manager      *store.NamedRef  `json:"manager"`
	Groups       []store.NamedRef `json:"groups"`
}

func (h *handlers) inactivePeopleReport(w http.ResponseWriter, r *http.Request, _ store.User) error {
	days := 90
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 3650 {
			return httpx.Invalid("days must be a whole number between 1 and 3650.")
		}
		days = n
	}
	group, groupNames, err := h.reportGroup(r)
	if err != nil {
		return err
	}
	users, err := h.st.ListUsers(r.Context())
	if err != nil {
		return err
	}
	byID := map[string]store.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	rows := []inactivePerson{}
	for _, u := range users {
		if u.Status != "active" && u.Status != "invited" || group != "" && !slices.Contains(u.GroupIDs, group) {
			continue
		}
		if !cmp.Or(u.LastSignInAt, &u.CreatedAt).Before(cutoff) {
			continue
		}
		p := inactivePerson{ID: u.ID, Name: u.Name, Email: u.Email, Status: u.Status, Department: u.Department, LastSignInAt: u.LastSignInAt, Groups: []store.NamedRef{}}
		if m, ok := byID[deref(u.ManagerID)]; ok {
			p.Manager = &store.NamedRef{ID: m.ID, Name: m.Name, Email: m.Email}
		}
		for _, id := range u.GroupIDs {
			p.Groups = append(p.Groups, store.NamedRef{ID: id, Name: groupNames[id]})
		}
		rows = append(rows, p)
	}
	slices.SortStableFunc(rows, func(a, b inactivePerson) int {
		return cmp.Compare(deref(a.LastSignInAt).Unix(), deref(b.LastSignInAt).Unix())
	})
	if wantsCSV(r) {
		lines := make([][]string, len(rows))
		for i, p := range rows {
			manager, managerEmail, groups := "", "", make([]string, len(p.Groups))
			if p.Manager != nil {
				manager, managerEmail = p.Manager.Name, p.Manager.Email
			}
			for j, g := range p.Groups {
				groups[j] = g.Name
			}
			lines[i] = []string{p.Name, p.Email, p.Status, p.Department, csvTime(p.LastSignInAt), manager, managerEmail, strings.Join(groups, "; "), p.ID}
		}
		return writeCSV(w, "inactive-people", []string{"name", "email", "status", "department", "last_sign_in", "manager", "manager_email", "groups", "user_id"}, lines)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"days": days, "people": rows})
}

func deref[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

type applicationUsage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Assigned int    `json:"assigned"`
	store.SignInCount
}

func (h *handlers) applicationUsageReport(w http.ResponseWriter, r *http.Request, _ store.User) error {
	p, err := h.period(r)
	if err != nil {
		return err
	}
	counts, err := h.st.SignInsByApp(r.Context(), p.From, p.To)
	if err != nil {
		return err
	}
	apps, err := h.st.ListApplications(r.Context())
	if err != nil {
		return err
	}
	rows := []applicationUsage{}
	for _, a := range apps {
		if a.Protocol == "bookmark" || a.Type == "service" {
			continue
		}
		rows = append(rows, applicationUsage{a.ID, a.Name, a.Status, a.UserCount, counts[a.ID]})
	}
	slices.SortStableFunc(rows, func(a, b applicationUsage) int { return cmp.Compare(b.SignIns, a.SignIns) })
	if wantsCSV(r) {
		lines := make([][]string, len(rows))
		for i, a := range rows {
			lines[i] = []string{a.Name, a.Status, strconv.Itoa(a.SignIns), strconv.Itoa(a.People), strconv.Itoa(a.Assigned), csvTime(a.LastSignInAt), a.ID}
		}
		return writeCSV(w, "application-usage", []string{"application", "status", "sign_ins", "people", "assigned_people", "last_sign_in", "application_id"}, lines)
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"period": p, "applications": rows})
}

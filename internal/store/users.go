package store

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

var RoleLabels = map[string]string{
	"global_admin":   "Global administrator",
	"security_admin": "Security administrator",
	"user_admin":     "User administrator",
	"helpdesk_admin": "Helpdesk administrator",
	"app_admin":      "Application administrator",
	"auditor":        "Auditor",
}

var ScopedRoles = []string{"user_admin", "helpdesk_admin"}

func (u User) HasRole(keys ...string) bool {
	for _, key := range keys {
		if _, scoped := u.RoleScopes[key]; slices.Contains(u.RoleKeys, key) && !scoped {
			return true
		}
	}
	return false
}

func (u User) Scope(roles ...string) ([]string, bool) {
	if u.HasRole("global_admin") || u.HasRole(roles...) {
		return nil, true
	}
	var groups []string
	for _, role := range roles {
		if slices.Contains(u.RoleKeys, role) {
			groups = append(groups, u.RoleScopes[role]...)
		}
	}
	return groups, len(groups) > 0
}

func (u User) Manages(target User, roles ...string) bool {
	groups, ok := u.Scope(roles...)
	return ok && (groups == nil || slices.ContainsFunc(target.GroupIDs, func(g string) bool { return slices.Contains(groups, g) }))
}

func (u User) Expired() bool {
	return u.Guest && u.ExpiresAt != nil && !u.ExpiresAt.After(time.Now())
}

func (u User) CanSponsor() bool {
	return u.Kind == "person" && !u.Guest && u.Status == "active"
}

func (u User) CanReauthenticate() bool {
	return slices.ContainsFunc(u.Methods, func(m Method) bool { return m.Kind == "passkey" || m.Kind == "security-key" || m.Kind == "totp" })
}

const userColumns = `id, email, name, title, department, location, status, manager_id, source, created_at, last_sign_in_at, kind, email_verified, avatar_updated_at, guest, expires_at, attributes,
	(select json_build_object('id', s.id, 'name', s.name, 'email', s.email) from users s where s.id = users.sponsor_id),
	(select json_build_object('id', d.id, 'name', d.name) from ldap_people p join ldap_directories d on d.id = p.directory_id where p.user_id = users.id)`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var u User
	var avatar *time.Time
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Title, &u.Department, &u.Location, &u.Status, &u.ManagerID, &u.Source, &u.CreatedAt, &u.LastSignInAt, &u.Kind, &u.EmailVerified, &avatar, &u.Guest, &u.ExpiresAt, &u.Attributes, &u.Sponsor, &u.Directory)
	if u.Expired() && (u.Status == "active" || u.Status == "invited") {
		u.Status = "suspended"
	}
	if avatar != nil {
		path := Avatars.Path(u.ID, *avatar)
		u.AvatarURL = &path
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	return s.queryUsers(ctx, `kind = 'person'`)
}

func (s *Store) UsersByID(ctx context.Context, ids []string) ([]User, error) {
	return s.queryUsers(ctx, `id = any($1)`, ids)
}

func (s *Store) ListSponsoredGuests(ctx context.Context, sponsorID string) ([]User, error) {
	return s.queryUsers(ctx, `guest and sponsor_id = $1`, sponsorID)
}

func (s *Store) queryUsers(ctx context.Context, where string, args ...any) ([]User, error) {
	rows, err := s.db.Query(ctx, `select `+userColumns+` from users where `+where+` order by lower(name), id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, s.decorate(ctx, users)
}

func (s *Store) GetUser(ctx context.Context, userID string) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `select `+userColumns+` from users where id = $1`, userID))
	if err != nil {
		return User{}, notFound(err)
	}
	users := []User{u}
	if err := s.decorate(ctx, users); err != nil {
		return User{}, err
	}
	return users[0], nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `select `+userColumns+` from users where lower(email) = lower($1)`, strings.TrimSpace(email)))
	if err != nil {
		return User{}, notFound(err)
	}
	users := []User{u}
	if err := s.decorate(ctx, users); err != nil {
		return User{}, err
	}
	return users[0], nil
}

func (s *Store) UsersByEmail(ctx context.Context, emails []string) ([]User, error) {
	rows, err := s.db.Query(ctx, `select `+userColumns+` from users where lower(email) = any(select lower(trim(e)) from unnest($1::text[]) e)`, emails)
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) { return scanUser(row) })
	if err != nil {
		return nil, err
	}
	return users, s.decorate(ctx, users)
}

type NewUser struct {
	ID         string
	Email      string
	Name       string
	Title      string
	Department string
	Location   string
	Status     string
	ManagerID  *string
	Source     string
	Roles      []string
	GroupIDs   []string
	Guest      bool
	SponsorID  *string
	ExpiresAt  *time.Time
}

func (s *Store) CreateUser(ctx context.Context, in NewUser) (User, error) {
	userID := cmp.Or(in.ID, id.New("usr"))
	if in.Status == "" {
		in.Status = "invited"
	}
	if in.Source == "" {
		in.Source = "Halo"
	}
	err := s.Tx(ctx, func(tx *Store) error {
		_, err := tx.db.Exec(ctx, `insert into users (id, email, name, title, department, location, status, manager_id, source, guest, sponsor_id, expires_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			userID, strings.TrimSpace(in.Email), strings.TrimSpace(in.Name), in.Title, in.Department, in.Location, in.Status, in.ManagerID, in.Source, in.Guest, in.SponsorID, in.ExpiresAt)
		if err != nil {
			return conflict(err)
		}
		for _, role := range in.Roles {
			if _, ok := RoleLabels[role]; !ok {
				return fmt.Errorf("unknown role %q", role)
			}
			if _, err := tx.db.Exec(ctx, `insert into user_roles (user_id, role) values ($1, $2)`, userID, role); err != nil {
				return err
			}
		}
		for _, groupID := range in.GroupIDs {
			if err := tx.AddGroupMember(ctx, groupID, userID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) SetUserStatus(ctx context.Context, userID, status string) error {
	var previous string
	if err := s.db.QueryRow(ctx, `with old as (select status from users where id = $1) update users set status = $2, updated_at = now() where id = $1 returning (select status from old)`, userID, status).Scan(&previous); err != nil {
		return notFound(err)
	}
	disabled := func(status string) bool { return status == "suspended" || status == "deprovisioned" }
	switch {
	case disabled(status) && !disabled(previous):
		return s.signal(ctx, accountDisabled, nil, signalUser, userID)
	case disabled(previous) && !disabled(status):
		return s.signal(ctx, accountEnabled, nil, signalUser, userID)
	}
	return nil
}

func (s *Store) TouchSignIn(ctx context.Context, userID string, at time.Time) error {
	_, err := s.db.Exec(ctx, `update users set last_sign_in_at = $2, status = case when status = 'invited' then 'active' else status end where id = $1`, userID, at)
	return err
}

func (s *Store) SetUserRoles(ctx context.Context, userID string, roles []string, scopes map[string][]string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from user_roles where user_id = $1`, userID); err != nil {
			return err
		}
		for _, role := range roles {
			if _, ok := RoleLabels[role]; !ok {
				return fmt.Errorf("unknown role %q", role)
			}
			if _, err := tx.db.Exec(ctx, `insert into user_roles (user_id, role, scope) values ($1, $2, $3)`, userID, role, scopes[role]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeleteAuthenticationMethods(ctx context.Context, userID string) error {
	return s.Tx(ctx, func(tx *Store) error {
		for _, table := range []string{"webauthn_credentials", "totp_secrets", "recovery_codes", "passwords"} {
			if _, err := tx.deleteCredentials(ctx, table, "revoke", `user_id = $1`, userID); err != nil {
				return err
			}
		}
		return tx.deleteAccessPass(ctx, userID)
	})
}

var credentialColumns = map[string]string{"webauthn_credentials": "kind, label", "totp_secrets": "case when confirmed then 'totp' else '' end, label", "passwords": "'password', ''", "recovery_codes": "'', ''"}

func (s *Store) deleteCredentials(ctx context.Context, table, change, where string, args ...any) (bool, error) {
	rows, err := s.db.Query(ctx, `delete from `+table+` where `+where+` returning user_id, `+credentialColumns[table], args...)
	if err != nil {
		return false, err
	}
	deleted, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ UserID, Kind, Label string }])
	if err != nil {
		return false, err
	}
	for _, d := range deleted {
		if err := s.credentialChanged(ctx, d.UserID, d.Kind, change, d.Label); err != nil {
			return false, err
		}
	}
	return len(deleted) > 0, nil
}

func (s *Store) deleteAccessPass(ctx context.Context, userID string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from access_passes where user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `update enrollment_tokens set used_at = now() where user_id = $1 and purpose = 'access-pass' and used_at is null`, userID)
		return err
	})
}

func (s *Store) DeleteMethod(ctx context.Context, userID, methodID string) error {
	for _, table := range []string{"webauthn_credentials", "totp_secrets"} {
		deleted, err := s.deleteCredentials(ctx, table, "delete", `user_id = $1 and id = $2`, userID, methodID)
		if err != nil || deleted {
			return err
		}
	}
	if methodID == "access-pass" {
		return s.deleteAccessPass(ctx, userID)
	}
	table := map[string]string{"recovery-codes": "recovery_codes", "password": "passwords"}[methodID]
	if table == "" {
		return ErrNotFound
	}
	_, err := s.deleteCredentials(ctx, table, "delete", `user_id = $1`, userID)
	return err
}

func (s *Store) decorate(ctx context.Context, users []User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]string, len(users))
	index := make(map[string]int, len(users))
	for i, u := range users {
		ids[i] = u.ID
		index[u.ID] = i
		users[i].Methods = []Method{}
		users[i].Roles = []string{}
		users[i].RoleKeys = []string{}
		users[i].EligibleRoles = []string{}
		users[i].RoleScopes = map[string][]string{}
		users[i].EligibleScopes = map[string][]string{}
		users[i].Activations = []RoleActivation{}
		users[i].GroupIDs = []string{}
		users[i].AppIDs = []string{}
	}

	rows, err := s.db.Query(ctx, `select user_id, role, kind, id, justification, created_at, expires_at, scope from (
			select user_id, role, 'standing' as kind, '' as id, '' as justification, created_at, null::timestamptz as expires_at, scope from user_roles where user_id = any($1)
			union all
			select user_id, role, 'eligible', '', '', created_at, null, scope from role_eligibilities where user_id = any($1)
			union all
			select a.user_id, a.role, 'active', a.id, a.justification, a.created_at, a.expires_at, e.scope from role_activations a
			left join role_eligibilities e on e.user_id = a.user_id and e.role = a.role
			where a.user_id = any($1) and a.ended_at is null and a.expires_at > now()
				and not exists (select 1 from user_roles r where r.user_id = a.user_id and r.role = a.role)
		) roles order by role`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userID, role, kind string
		var a RoleActivation
		var expires *time.Time
		var scope []string
		if err := rows.Scan(&userID, &role, &kind, &a.ID, &a.Justification, &a.ActivatedAt, &expires, &scope); err != nil {
			rows.Close()
			return err
		}
		u := &users[index[userID]]
		if kind == "eligible" {
			u.EligibleRoles = append(u.EligibleRoles, role)
			if scope != nil {
				u.EligibleScopes[role] = scope
			}
			continue
		}
		if scope != nil {
			u.RoleScopes[role] = scope
		}
		u.RoleKeys = append(u.RoleKeys, role)
		u.Roles = append(u.Roles, RoleLabels[role])
		if expires != nil {
			a.Role, a.ExpiresAt = role, *expires
			u.Activations = append(u.Activations, a)
		}
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select id, user_id, kind, label, false, 0, null::timestamptz, created_at, last_used_at from webauthn_credentials where user_id = any($1)
		union all select 'password', user_id, 'password', 'Password', must_change, 0, null, created_at, last_used_at from passwords where user_id = any($1)
		union all select 'access-pass', user_id, 'access-pass', 'Temporary access pass', false, uses_left, expires_at, created_at, last_used_at from access_passes
			where user_id = any($1) and uses_left > 0 and expires_at > now() order by created_at`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m Method
		var userID string
		var temporary bool
		var left int
		var expires *time.Time
		if err := rows.Scan(&m.ID, &userID, &m.Kind, &m.Label, &temporary, &left, &expires, &m.AddedAt, &m.LastUsedAt); err != nil {
			rows.Close()
			return err
		}
		if temporary {
			m.Detail = "Temporary, must be changed at the next sign-in"
		}
		if expires != nil {
			uses := "uses"
			if left == 1 {
				uses = "use"
			}
			m.Detail = fmt.Sprintf("%d %s left, expires %s", left, uses, expires.UTC().Format("2 Jan 2006, 15:04 UTC"))
		}
		u := &users[index[userID]]
		u.Methods = append(u.Methods, m)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select id, user_id, label, created_at, last_used_at from totp_secrets where confirmed and user_id = any($1) order by created_at`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		m := Method{Kind: "totp"}
		var userID string
		if err := rows.Scan(&m.ID, &userID, &m.Label, &m.AddedAt, &m.LastUsedAt); err != nil {
			rows.Close()
			return err
		}
		u := &users[index[userID]]
		u.Methods = append(u.Methods, m)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select user_id, count(*) filter (where used_at is null), count(*), max(created_at), max(used_at) from recovery_codes where user_id = any($1) group by user_id`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userID string
		var remaining, total int
		var created time.Time
		var used *time.Time
		if err := rows.Scan(&userID, &remaining, &total, &created, &used); err != nil {
			rows.Close()
			return err
		}
		u := &users[index[userID]]
		u.Methods = append(u.Methods, Method{ID: "recovery-codes", Kind: "recovery-codes", Label: "Recovery codes", AddedAt: created, LastUsedAt: used, Detail: fmt.Sprintf("%d of %d remaining", remaining, total)})
	}
	rows.Close()

	dynamic, args, err := s.dynamicGroups(ctx, ids)
	if err != nil {
		return err
	}
	rows, err = s.db.Query(ctx, dynamic+`select user_id, group_id from (
			select m.user_id, m.group_id, 0 as rank, m.created_at from group_members m where m.user_id = any($1)
			union all
			select u.id, d.id, 1, d.created_at from users u join dynamic d on d.user_id = u.id where u.id = any($1) and u.kind <> 'service'
		) memberships order by rank, created_at`, args...)
	if err != nil {
		return err
	}
	var groupIDs []string
	for rows.Next() {
		var userID, groupID string
		if err := rows.Scan(&userID, &groupID); err != nil {
			rows.Close()
			return err
		}
		u := &users[index[userID]]
		u.GroupIDs = append(u.GroupIDs, groupID)
		groupIDs = append(groupIDs, groupID)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select ag.app_id, ag.group_id from app_groups ag join applications a on a.id = ag.app_id where ag.group_id = any($1) order by a.name`, groupIDs)
	if err != nil {
		return err
	}
	appGroups := map[string][]string{}
	var appOrder []string
	for rows.Next() {
		var appID, groupID string
		if err := rows.Scan(&appID, &groupID); err != nil {
			rows.Close()
			return err
		}
		if _, seen := appGroups[appID]; !seen {
			appOrder = append(appOrder, appID)
		}
		appGroups[appID] = append(appGroups[appID], groupID)
	}
	rows.Close()
	for i := range users {
		for _, appID := range appOrder {
			for _, groupID := range appGroups[appID] {
				if slices.Contains(users[i].GroupIDs, groupID) {
					users[i].AppIDs = append(users[i].AppIDs, appID)
					break
				}
			}
		}
		users[i].Strength = strength(users[i].Methods)
	}
	return nil
}

func strength(methods []Method) string {
	level := "single-factor"
	for _, m := range methods {
		switch m.Kind {
		case "passkey", "security-key":
			return "phishing-resistant"
		case "totp":
			level = "multi-factor"
		}
	}
	return level
}

const ruleExpression = `^\s*user\.(status|department|location|title|source)\s*==\s*"([^"]*)"\s*$`

var rulePattern = regexp.MustCompile(ruleExpression)

func ValidRule(rule string) bool {
	return rulePattern.MatchString(rule)
}

func RuleMatches(rule string, u User) bool {
	m := rulePattern.FindStringSubmatch(rule)
	if m == nil {
		return false
	}
	var value string
	switch m[1] {
	case "status":
		value = u.Status
	case "department":
		value = u.Department
	case "location":
		value = u.Location
	case "title":
		value = u.Title
	case "source":
		value = u.Source
	}
	return strings.EqualFold(value, m[2])
}

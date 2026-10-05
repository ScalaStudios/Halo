package store

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

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

func (u User) HasRole(keys ...string) bool {
	for _, key := range keys {
		if slices.Contains(u.RoleKeys, key) {
			return true
		}
	}
	return false
}

const userColumns = `id, email, name, title, department, location, status, manager_id, source, created_at, last_sign_in_at, kind, email_verified, avatar_updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var u User
	var avatar *time.Time
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Title, &u.Department, &u.Location, &u.Status, &u.ManagerID, &u.Source, &u.CreatedAt, &u.LastSignInAt, &u.Kind, &u.EmailVerified, &avatar)
	if avatar != nil {
		path := AvatarPath(u.ID, *avatar)
		u.AvatarURL = &path
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `select `+userColumns+` from users where kind = 'person' order by lower(name), id`)
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

type NewUser struct {
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
}

func (s *Store) CreateUser(ctx context.Context, in NewUser) (User, error) {
	userID := id.New("usr")
	if in.Status == "" {
		in.Status = "invited"
	}
	if in.Source == "" {
		in.Source = "Halo"
	}
	err := s.Tx(ctx, func(tx *Store) error {
		_, err := tx.db.Exec(ctx, `insert into users (id, email, name, title, department, location, status, manager_id, source) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			userID, strings.TrimSpace(in.Email), strings.TrimSpace(in.Name), in.Title, in.Department, in.Location, in.Status, in.ManagerID, in.Source)
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
	tag, err := s.db.Exec(ctx, `update users set status = $2, updated_at = now() where id = $1`, userID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchSignIn(ctx context.Context, userID string, at time.Time) error {
	_, err := s.db.Exec(ctx, `update users set last_sign_in_at = $2, status = case when status = 'invited' then 'active' else status end where id = $1`, userID, at)
	return err
}

func (s *Store) SetUserRoles(ctx context.Context, userID string, roles []string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from user_roles where user_id = $1`, userID); err != nil {
			return err
		}
		for _, role := range roles {
			if _, ok := RoleLabels[role]; !ok {
				return fmt.Errorf("unknown role %q", role)
			}
			if _, err := tx.db.Exec(ctx, `insert into user_roles (user_id, role) values ($1, $2)`, userID, role); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeleteAuthenticationMethods(ctx context.Context, userID string) error {
	return s.Tx(ctx, func(tx *Store) error {
		for _, table := range []string{"webauthn_credentials", "totp_secrets", "recovery_codes"} {
			if _, err := tx.db.Exec(ctx, `delete from `+table+` where user_id = $1`, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeleteMethod(ctx context.Context, userID, methodID string) error {
	for _, table := range []string{"webauthn_credentials", "totp_secrets"} {
		tag, err := s.db.Exec(ctx, `delete from `+table+` where user_id = $1 and id = $2`, userID, methodID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
	}
	if methodID == "recovery-codes" {
		_, err := s.db.Exec(ctx, `delete from recovery_codes where user_id = $1`, userID)
		return err
	}
	return ErrNotFound
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
		users[i].GroupIDs = []string{}
		users[i].AppIDs = []string{}
	}

	rows, err := s.db.Query(ctx, `select user_id, role from user_roles where user_id = any($1) order by role`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userID, role string
		if err := rows.Scan(&userID, &role); err != nil {
			rows.Close()
			return err
		}
		u := &users[index[userID]]
		u.RoleKeys = append(u.RoleKeys, role)
		u.Roles = append(u.Roles, RoleLabels[role])
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select id, user_id, kind, label, created_at, last_used_at from webauthn_credentials where user_id = any($1) order by created_at`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m Method
		var userID string
		if err := rows.Scan(&m.ID, &userID, &m.Kind, &m.Label, &m.AddedAt, &m.LastUsedAt); err != nil {
			rows.Close()
			return err
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

	rows, err = s.db.Query(ctx, `with `+dynamicGroupsSQL+` select user_id, group_id from (
			select m.user_id, m.group_id, 0 as rank, m.created_at from group_members m where m.user_id = any($1)
			union all
			select u.id, d.id, 1, d.created_at from users u join dynamic d on `+ruleMatchSQL+` where u.id = any($1) and u.kind <> 'service'
		) memberships order by rank, created_at`, ids)
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

const dynamicGroupsSQL = `dynamic as materialized (select g.id, g.created_at, r[1] as attribute, lower(r[2]) as value
	from groups g cross join lateral regexp_match(g.rule, '` + ruleExpression + `') r where g.kind = 'dynamic' and r is not null)`

const ruleMatchSQL = `lower(case d.attribute when 'status' then u.status when 'department' then u.department when 'location' then u.location when 'title' then u.title when 'source' then u.source end) = d.value`

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

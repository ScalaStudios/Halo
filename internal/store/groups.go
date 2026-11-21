package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/rule"
)

const posixGroupSQL = `(select json_build_object('name', p.name, 'gidNumber', p.number) from posix_ids p where p.group_id = g.id)`

const assignedCountSQL = `(select count(*) from group_members m join users u on u.id = m.user_id where m.group_id = g.id and u.status <> 'deprovisioned')`

func (s *Store) ListGroupsRaw(ctx context.Context) ([]Group, error) {
	return s.queryGroups(ctx, `select g.id, g.name, g.description, g.kind, g.rule, g.join_policy, g.source, g.created_at, `+posixGroupSQL+`, `+assignedCountSQL+` from groups g order by g.created_at`)
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	dynamic, args, err := s.dynamicGroups(ctx)
	if err != nil {
		return nil, err
	}
	return s.queryGroups(ctx, dynamic+`select g.id, g.name, g.description, g.kind, g.rule, g.join_policy, g.source, g.created_at, `+posixGroupSQL+`,
		case when g.kind = 'dynamic' then (select count(*) from dynamic d join users u on u.id = d.user_id where d.id = g.id and u.status <> 'deprovisioned' and u.kind = 'person')
		else `+assignedCountSQL+` end from groups g order by g.created_at`, args...)
}

func (s *Store) ruleCompiler(ctx context.Context, args ...any) (*rule.Compiler, []rule.Group, error) {
	rows, err := s.db.Query(ctx, `select id, name, rule from groups order by created_at, id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	c := &rule.Compiler{Groups: map[string]rule.Group{}, Args: args}
	var dynamic []rule.Group
	for rows.Next() {
		var g rule.Group
		var name string
		if err := rows.Scan(&g.ID, &name, &g.Rule); err != nil {
			return nil, nil, err
		}
		c.Groups[strings.ToLower(name)] = g
		if g.Rule != nil {
			dynamic = append(dynamic, g)
		}
	}
	return c, dynamic, rows.Err()
}

func (s *Store) dynamicGroups(ctx context.Context, args ...any) (string, []any, error) {
	c, dynamic, err := s.ruleCompiler(ctx, args...)
	if err != nil {
		return "", nil, err
	}
	match := "false"
	if len(dynamic) > 0 {
		match = "case g.id"
		for _, g := range dynamic {
			where, err := c.Rule(g.ID, *g.Rule)
			if err != nil {
				where = "false"
			}
			match += " when " + c.Param(g.ID) + " then " + where
		}
		match += " else false end"
	}
	return `with dynamic as (select g.id, g.created_at, u.id as user_id from groups g cross join users u where not u.guest and (` + match + `)) `, c.Args, nil
}

func (s *Store) CheckRule(ctx context.Context, groupID, src string) error {
	c, _, err := s.ruleCompiler(ctx)
	if err == nil {
		_, err = c.Rule(groupID, src)
	}
	return err
}

func (s *Store) MatchRule(ctx context.Context, groupID, src string) ([]string, error) {
	c, _, err := s.ruleCompiler(ctx)
	if err != nil {
		return nil, err
	}
	where, err := c.Rule(groupID, src)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `select u.id from users u where not u.guest and (`+where+`)`, c.Args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) queryGroups(ctx context.Context, query string, args ...any) ([]Group, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var groups []Group
	index := map[string]int{}
	for rows.Next() {
		g := Group{Owners: []NamedRef{}}
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Kind, &g.Rule, &g.JoinPolicy, &g.Source, &g.CreatedAt, &g.POSIX, &g.MemberCount); err != nil {
			rows.Close()
			return nil, err
		}
		index[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	owners, err := s.db.Query(ctx, `select o.group_id, u.id, u.name, u.email from group_owners o join users u on u.id = o.user_id order by lower(u.name), u.id`)
	if err != nil {
		return nil, err
	}
	defer owners.Close()
	for owners.Next() {
		var groupID string
		var ref NamedRef
		if err := owners.Scan(&groupID, &ref.ID, &ref.Name, &ref.Email); err != nil {
			return nil, err
		}
		if i, ok := index[groupID]; ok {
			groups[i].Owners = append(groups[i].Owners, ref)
		}
	}
	return groups, owners.Err()
}

func (s *Store) GetGroup(ctx context.Context, groupID string) (Group, error) {
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return Group{}, err
	}
	for _, g := range groups {
		if g.ID == groupID {
			return g, nil
		}
	}
	return Group{}, ErrNotFound
}

type NewGroup struct {
	Name        string
	Description string
	Kind        string
	Rule        *string
	Source      string
}

func (s *Store) CreateGroup(ctx context.Context, in NewGroup) (Group, error) {
	groupID := id.New("grp")
	if in.Source == "" {
		in.Source = "Halo"
	}
	_, err := s.db.Exec(ctx, `insert into groups (id, name, description, kind, rule, source) values ($1, $2, $3, $4, $5, $6)`,
		groupID, strings.TrimSpace(in.Name), in.Description, in.Kind, in.Rule, in.Source)
	if err != nil {
		return Group{}, conflict(err)
	}
	return s.GetGroup(ctx, groupID)
}

func (s *Store) AddGroupMember(ctx context.Context, groupID, userID string) error {
	var kind string
	if err := s.db.QueryRow(ctx, `select kind from groups where id = $1`, groupID).Scan(&kind); err != nil {
		return notFound(err)
	}
	if kind != "assigned" {
		return ErrConflict
	}
	if _, err := s.db.Exec(ctx, `delete from group_join_requests where group_id = $1 and user_id = $2`, groupID, userID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into group_members (group_id, user_id) values ($1, $2) on conflict do nothing`, groupID, userID)
	return err
}

func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	tag, err := s.db.Exec(ctx, `delete from group_members where group_id = $1 and user_id = $2`, groupID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetGroupOwners(ctx context.Context, groupID string, userIDs []string) error {
	var kind string
	if err := s.db.QueryRow(ctx, `select kind from groups where id = $1`, groupID).Scan(&kind); err != nil {
		return notFound(err)
	}
	if kind != "assigned" {
		return ErrConflict
	}
	if _, err := s.db.Exec(ctx, `delete from group_owners where group_id = $1`, groupID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into group_owners (group_id, user_id) select $1, unnest($2::text[])`, groupID, userIDs)
	return err
}

type JoinRequest struct {
	User      NamedRef  `json:"user"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) RequestToJoinGroup(ctx context.Context, groupID, userID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `insert into group_join_requests (group_id, user_id) values ($1, $2) on conflict do nothing`, groupID, userID)
	return tag.RowsAffected() > 0, err
}

func (s *Store) GroupJoinRequests(ctx context.Context, groupID string) ([]JoinRequest, error) {
	rows, err := s.db.Query(ctx, `select u.id, u.name, u.email, j.created_at from group_join_requests j join users u on u.id = j.user_id
		where j.group_id = $1 and u.status <> 'deprovisioned' order by j.created_at, u.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []JoinRequest{}
	for rows.Next() {
		var j JoinRequest
		if err := rows.Scan(&j.User.ID, &j.User.Name, &j.User.Email, &j.CreatedAt); err != nil {
			return nil, err
		}
		requests = append(requests, j)
	}
	return requests, rows.Err()
}

func (s *Store) JoinRequestGroupIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select group_id from group_join_requests where user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) DeleteJoinRequest(ctx context.Context, groupID, userID string) error {
	tag, err := s.db.Exec(ctx, `delete from group_join_requests where group_id = $1 and user_id = $2`, groupID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

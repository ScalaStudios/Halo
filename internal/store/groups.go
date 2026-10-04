package store

import (
	"context"
	"strings"

	"halo/internal/id"
)

const assignedCountSQL = `(select count(*) from group_members m join users u on u.id = m.user_id where m.group_id = g.id and u.status <> 'deprovisioned')`

func (s *Store) ListGroupsRaw(ctx context.Context) ([]Group, error) {
	return s.queryGroups(ctx, `select g.id, g.name, g.description, g.kind, g.rule, g.source, g.created_at, `+assignedCountSQL+` from groups g order by g.created_at`)
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	return s.queryGroups(ctx, `with `+dynamicGroupsSQL+` select g.id, g.name, g.description, g.kind, g.rule, g.source, g.created_at,
		case when g.kind = 'dynamic' then (select count(*) from dynamic d join users u on `+ruleMatchSQL+` where d.id = g.id and u.status <> 'deprovisioned' and u.kind = 'person')
		else `+assignedCountSQL+` end from groups g order by g.created_at`)
}

func (s *Store) queryGroups(ctx context.Context, query string) ([]Group, error) {
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Kind, &g.Rule, &g.Source, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
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

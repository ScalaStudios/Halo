package store

import (
	"context"
	"slices"

	"halo/internal/id"
)

type AppRole struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	GroupIDs    []string `json:"groupIds"`
	UserIDs     []string `json:"userIds"`
}

func (s *Store) CreateAppRole(ctx context.Context, appID string, r AppRole) (AppRole, error) {
	r.ID = id.New("arl")
	return r, s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `insert into app_roles (id, app_id, key, name, description) values ($1, $2, $3, $4, $5)`, r.ID, appID, r.Key, r.Name, r.Description); err != nil {
			return conflict(err)
		}
		return tx.assignAppRole(ctx, r)
	})
}

func (s *Store) UpdateAppRole(ctx context.Context, r AppRole) error {
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `update app_roles set name = $2, description = $3 where id = $1`, r.ID, r.Name, r.Description)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return tx.assignAppRole(ctx, r)
	})
}

func (s *Store) assignAppRole(ctx context.Context, r AppRole) error {
	for _, table := range []string{"app_role_groups", "app_role_users"} {
		if _, err := s.db.Exec(ctx, `delete from `+table+` where role_id = $1`, r.ID); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(ctx, `insert into app_role_groups (role_id, group_id) select $1, unnest($2::text[])`, r.ID, r.GroupIDs); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `insert into app_role_users (role_id, user_id) select $1, unnest($2::text[])`, r.ID, r.UserIDs)
	return err
}

func (s *Store) DeleteAppRole(ctx context.Context, roleID string) error {
	tag, err := s.db.Exec(ctx, `delete from app_roles where id = $1`, roleID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (a Application) RolesOf(u User) []AppRole {
	var held []AppRole
	for _, r := range a.Roles {
		if slices.Contains(r.UserIDs, u.ID) || slices.ContainsFunc(r.GroupIDs, func(g string) bool { return slices.Contains(u.GroupIDs, g) }) {
			held = append(held, r)
		}
	}
	return held
}

func (a Application) RoleKeys(u User) []string {
	keys := []string{}
	for _, r := range a.RolesOf(u) {
		keys = append(keys, r.Key)
	}
	return keys
}

func (a Application) Mapped(claims []Claim) []Claim {
	if len(a.Roles) > 0 && !slices.ContainsFunc(claims, func(c Claim) bool { return c.Name == "roles" }) {
		claims = append(claims, Claim{Name: "roles", Source: "user.roles (the application roles the user holds)"})
	}
	return MapClaims(claims, a.ClaimMappings)
}

package store

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Store) UpdateUserProfile(ctx context.Context, u User) error {
	tag, err := s.db.Exec(ctx, `update users set email = $2, email_verified = email_verified and lower(email) = lower($2), name = $3, title = $4, department = $5, location = $6, manager_id = $7, updated_at = now() where id = $1`,
		u.ID, u.Email, u.Name, u.Title, u.Department, u.Location, u.ManagerID)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ManagementChainIncludes(ctx context.Context, fromID, userID string) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, `with recursive chain(id) as (
			select $1::text
			union
			select u.manager_id from users u join chain c on u.id = c.id where u.manager_id is not null
		)
		select exists (select 1 from chain where id = $2)`, fromID, userID).Scan(&found)
	return found, err
}

func (s *Store) UpdateGroup(ctx context.Context, g Group) error {
	tag, err := s.db.Exec(ctx, `update groups set name = $2, description = $3, rule = $4 where id = $1`, g.ID, g.Name, g.Description, g.Rule)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type GroupUse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Store) GroupUses(ctx context.Context, groupID string) ([]GroupUse, error) {
	rows, err := s.db.Query(ctx, `select 'application', a.id, a.name from app_groups ag join applications a on a.id = ag.app_id where ag.group_id = $1
		union all
		select 'access package', p.id, p.name from access_package_groups pg join access_packages p on p.id = pg.package_id where pg.group_id = $1
		union all
		select 'policy', p.id, p.name from access_policies p where p.conditions->'groupIds' ? $1 or p.conditions->'excludeGroupIds' ? $1
		union all
		select 'identity provider', p.id, p.name from identity_providers p where $1 = any(p.jit_group_ids)
		union all
		select 'access review', v.id, v.name from access_reviews v where v.group_id = $1 and v.status <> 'completed'
		union all
		select 'lifecycle rule', l.id, l.name from lifecycle_rules l where l.actions @> jsonb_build_array(jsonb_build_object('groupId', $1::text))
		order by 1, 3`, groupID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[GroupUse])
}

type GroupInUseError struct {
	Group string
	Uses  []GroupUse
}

func (e *GroupInUseError) Error() string {
	names := make([]string, len(e.Uses))
	for i, u := range e.Uses {
		names[i] = "the " + u.Kind + " " + u.Name
	}
	next := "Remove the group from each of them, then delete it."
	if slices.ContainsFunc(e.Uses, func(u GroupUse) bool { return u.Kind == "access review" }) {
		next = "Remove the group from each of them and complete its open access reviews, then delete it."
	}
	return e.Group + " is still used by " + strings.Join(names, ", ") + ". " + next
}

func (s *Store) DeleteGroup(ctx context.Context, groupID string) error {
	var name string
	if err := s.db.QueryRow(ctx, `select name from groups where id = $1`, groupID).Scan(&name); err != nil {
		return notFound(err)
	}
	uses, err := s.GroupUses(ctx, groupID)
	if err != nil {
		return err
	}
	if len(uses) > 0 {
		return &GroupInUseError{Group: name, Uses: uses}
	}
	if _, err := s.db.Exec(ctx, `update access_reviews v set group_name = g.name from groups g where g.id = $1 and v.group_id = $1`, groupID); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `delete from groups where id = $1`, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateApplication(ctx context.Context, a Application) error {
	tag, err := s.db.Exec(ctx, `update applications set name = $2, description = $3, homepage = $4, redirect_uris = $5, post_logout_uris = $6, scopes = $7,
		access_token_ttl = $8, refresh_token_ttl = $9, id_token_ttl = $10, refresh_rotation = $11, owner_id = $12, updated_at = now() where id = $1`,
		a.ID, a.Name, a.Description, a.Homepage, a.RedirectURIs, a.PostLogoutURIs, a.Scopes,
		a.TokenPolicy.AccessTokenTTL, a.TokenPolicy.RefreshTokenTTL, a.TokenPolicy.IDTokenTTL, a.TokenPolicy.Rotation, a.Owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

package store

import (
	"context"
	"strings"
	"time"

	"halo/internal/id"
)

type APIScope struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type APIResourceGrant struct {
	AppID  string   `json:"appId"`
	Scopes []string `json:"scopes"`
}

type APIResource struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Identifier     string             `json:"identifier"`
	Description    string             `json:"description"`
	AccessTokenTTL int                `json:"accessTokenTtl"`
	Scopes         []APIScope         `json:"scopes"`
	Grants         []APIResourceGrant `json:"grants"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

func (s *Store) ListAPIResources(ctx context.Context) ([]APIResource, error) {
	rows, err := s.db.Query(ctx, `select id, name, identifier, description, access_token_ttl, created_at, updated_at from api_resources order by lower(name)`)
	if err != nil {
		return nil, err
	}
	resources := []APIResource{}
	index := map[string]int{}
	for rows.Next() {
		r := APIResource{Scopes: []APIScope{}, Grants: []APIResourceGrant{}}
		if err := rows.Scan(&r.ID, &r.Name, &r.Identifier, &r.Description, &r.AccessTokenTTL, &r.CreatedAt, &r.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		index[r.ID] = len(resources)
		resources = append(resources, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(ctx, `select resource_id, name, description from api_resource_scopes order by name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var resourceID string
		var scope APIScope
		if err := rows.Scan(&resourceID, &scope.Name, &scope.Description); err != nil {
			rows.Close()
			return nil, err
		}
		r := &resources[index[resourceID]]
		r.Scopes = append(r.Scopes, scope)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select g.resource_id, g.app_id, array_agg(g.scope order by g.scope) from api_resource_grants g join applications a on a.id = g.app_id group by g.resource_id, g.app_id, a.name order by lower(a.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var resourceID string
		var grant APIResourceGrant
		if err := rows.Scan(&resourceID, &grant.AppID, &grant.Scopes); err != nil {
			return nil, err
		}
		r := &resources[index[resourceID]]
		r.Grants = append(r.Grants, grant)
	}
	return resources, rows.Err()
}

func (s *Store) GetAPIResource(ctx context.Context, resourceID string) (APIResource, error) {
	resources, err := s.ListAPIResources(ctx)
	if err != nil {
		return APIResource{}, err
	}
	for _, r := range resources {
		if r.ID == resourceID {
			return r, nil
		}
	}
	return APIResource{}, ErrNotFound
}

func (s *Store) SaveAPIResource(ctx context.Context, r APIResource) (APIResource, error) {
	if r.ID == "" {
		r.ID = id.New("api")
	}
	names := make([]string, len(r.Scopes))
	for i, scope := range r.Scopes {
		names[i] = scope.Name
	}
	err := s.Tx(ctx, func(tx *Store) error {
		_, err := tx.db.Exec(ctx, `insert into api_resources (id, name, identifier, description, access_token_ttl) values ($1, $2, $3, $4, $5)
			on conflict (id) do update set name = excluded.name, identifier = excluded.identifier, description = excluded.description, access_token_ttl = excluded.access_token_ttl, updated_at = now()`,
			r.ID, strings.TrimSpace(r.Name), r.Identifier, r.Description, r.AccessTokenTTL)
		if err != nil {
			return conflict(err)
		}
		if _, err := tx.db.Exec(ctx, `delete from api_resource_scopes where resource_id = $1 and not (name = any($2))`, r.ID, names); err != nil {
			return err
		}
		for _, scope := range r.Scopes {
			_, err := tx.db.Exec(ctx, `insert into api_resource_scopes (resource_id, name, description) values ($1, $2, $3)
				on conflict (resource_id, name) do update set description = excluded.description`, r.ID, scope.Name, scope.Description)
			if err != nil {
				return conflict(err)
			}
		}
		return nil
	})
	if err != nil {
		return APIResource{}, err
	}
	return s.GetAPIResource(ctx, r.ID)
}

func (s *Store) DeleteAPIResource(ctx context.Context, resourceID string) error {
	tag, err := s.db.Exec(ctx, `delete from api_resources where id = $1`, resourceID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetAPIResourceGrant(ctx context.Context, resourceID, appID string, scopes []string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from api_resource_grants where resource_id = $1 and app_id = $2`, resourceID, appID); err != nil {
			return err
		}
		for _, scope := range scopes {
			if _, err := tx.db.Exec(ctx, `insert into api_resource_grants (app_id, resource_id, scope) values ($1, $2, $3)`, appID, resourceID, scope); err != nil {
				return err
			}
		}
		return nil
	})
}

type GrantedResource struct {
	Identifier     string
	AccessTokenTTL int
	Scopes         []string
}

func (s *Store) GrantedResources(ctx context.Context, clientID string) ([]GrantedResource, error) {
	rows, err := s.db.Query(ctx, `select r.identifier, r.access_token_ttl, array_agg(g.scope order by g.scope) from api_resource_grants g
		join api_resources r on r.id = g.resource_id join applications a on a.id = g.app_id
		where a.client_id = $1 group by r.identifier, r.access_token_ttl order by r.identifier`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var granted []GrantedResource
	for rows.Next() {
		var g GrantedResource
		if err := rows.Scan(&g.Identifier, &g.AccessTokenTTL, &g.Scopes); err != nil {
			return nil, err
		}
		granted = append(granted, g)
	}
	return granted, rows.Err()
}

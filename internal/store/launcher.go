package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type Collection struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ApplicationIDs []string  `json:"applicationIds"`
	GroupIDs       []string  `json:"groupIds"`
	CreatedAt      time.Time `json:"createdAt"`
}

type AppPreference struct {
	Position  *int
	Favourite bool
}

func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.db.Query(ctx, `select c.id, c.name, c.created_at,
		array(select a.app_id from collection_apps a where a.collection_id = c.id order by a.app_id),
		array(select g.group_id from collection_groups g where g.collection_id = c.id order by g.group_id)
		from collections c order by lower(c.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Collection, error) {
		var c Collection
		err := row.Scan(&c.ID, &c.Name, &c.CreatedAt, &c.ApplicationIDs, &c.GroupIDs)
		return c, err
	})
}

func (s *Store) GetCollection(ctx context.Context, collectionID string) (Collection, error) {
	collections, err := s.ListCollections(ctx)
	if err != nil {
		return Collection{}, err
	}
	for _, c := range collections {
		if c.ID == collectionID {
			return c, nil
		}
	}
	return Collection{}, ErrNotFound
}

func (s *Store) SaveCollection(ctx context.Context, collectionID, name string, appIDs, groupIDs []string) (string, error) {
	err := s.Tx(ctx, func(tx *Store) error {
		if collectionID == "" {
			collectionID = id.New("col")
			if _, err := tx.db.Exec(ctx, `insert into collections (id, name) values ($1, $2)`, collectionID, name); err != nil {
				return conflict(err)
			}
		} else {
			tag, err := tx.db.Exec(ctx, `update collections set name = $2, updated_at = now() where id = $1`, collectionID, name)
			if err != nil {
				return conflict(err)
			}
			if tag.RowsAffected() == 0 {
				return ErrNotFound
			}
		}
		for _, link := range []struct {
			table, column string
			ids           []string
		}{{"collection_apps", "app_id", appIDs}, {"collection_groups", "group_id", groupIDs}} {
			if _, err := tx.db.Exec(ctx, `delete from `+link.table+` where collection_id = $1`, collectionID); err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `insert into `+link.table+` (collection_id, `+link.column+`) select $1, unnest($2::text[])`, collectionID, link.ids); err != nil {
				return err
			}
		}
		return nil
	})
	return collectionID, err
}

func (s *Store) DeleteCollection(ctx context.Context, collectionID string) error {
	tag, err := s.db.Exec(ctx, `delete from collections where id = $1`, collectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AppPreferences(ctx context.Context, userID string) (map[string]AppPreference, error) {
	rows, err := s.db.Query(ctx, `select app_id, position, favourite from app_preferences where user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prefs := map[string]AppPreference{}
	for rows.Next() {
		var appID string
		var p AppPreference
		if err := rows.Scan(&appID, &p.Position, &p.Favourite); err != nil {
			return nil, err
		}
		prefs[appID] = p
	}
	return prefs, rows.Err()
}

func (s *Store) SetAppPreferences(ctx context.Context, userID string, order, favourites []string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from app_preferences where user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into app_preferences (user_id, app_id, position, favourite)
			select $1, ids.app_id, array_position($2::text[], ids.app_id), coalesce(ids.app_id = any($3::text[]), false) from (select distinct unnest($2::text[] || $3::text[]) as app_id) ids`,
			userID, order, favourites)
		return err
	})
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Dismissal struct {
	Reason string    `json:"reason"`
	By     *NamedRef `json:"by"`
	At     time.Time `json:"at"`
}

func (s *Store) Dismissals(ctx context.Context) (map[string]Dismissal, error) {
	rows, err := s.db.Query(ctx, `select d.check_id, d.reason, d.dismissed_at, u.id, u.name from recommendation_dismissals d left join users u on u.id = d.dismissed_by`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Dismissal{}
	for rows.Next() {
		var checkID string
		var d Dismissal
		var byID, byName *string
		if err := rows.Scan(&checkID, &d.Reason, &d.At, &byID, &byName); err != nil {
			return nil, err
		}
		if byID != nil {
			d.By = &NamedRef{ID: *byID, Name: *byName}
		}
		out[checkID] = d
	}
	return out, rows.Err()
}

func (s *Store) DismissRecommendation(ctx context.Context, checkID, reason, userID string) error {
	_, err := s.db.Exec(ctx, `insert into recommendation_dismissals (check_id, reason, dismissed_by) values ($1, $2, $3)
		on conflict (check_id) do update set reason = excluded.reason, dismissed_by = excluded.dismissed_by, dismissed_at = now()`, checkID, reason, userID)
	return err
}

func (s *Store) RestoreRecommendation(ctx context.Context, checkID string) error {
	tag, err := s.db.Exec(ctx, `delete from recommendation_dismissals where check_id = $1`, checkID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SwapDigestRecommendations(ctx context.Context, ids []string) ([]string, error) {
	var previous []string
	err := s.db.QueryRow(ctx, `select value from settings where key = 'weeklyDigestRecommendations'`).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	value, _ := json.Marshal(ids)
	_, err = s.db.Exec(ctx, `insert into settings (key, value) values ('weeklyDigestRecommendations', $1) on conflict (key) do update set value = excluded.value, updated_at = now()`, value)
	return previous, err
}

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type AuthzRelation struct {
	Subjects []string    `json:"subjects,omitempty"`
	Union    []string    `json:"union,omitempty"`
	Roles    []string    `json:"roles,omitempty"`
	From     []AuthzFrom `json:"from,omitempty"`
}

type AuthzFrom struct {
	Via      string `json:"via"`
	Relation string `json:"relation"`
}

type AuthzTypes map[string]map[string]AuthzRelation

type AuthzModel struct {
	Types     AuthzTypes `json:"types"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

type Tuple struct {
	ObjectType      string
	ObjectID        string
	Relation        string
	SubjectType     string
	SubjectID       string
	SubjectRelation string
	CreatedAt       time.Time
}

type PersonTuple struct {
	AppID     string    `json:"applicationId"`
	AppName   string    `json:"application"`
	Object    string    `json:"object"`
	Relation  string    `json:"relation"`
	GroupID   string    `json:"groupId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

const tupleColumns = `object_type, object_id, relation, subject_type, subject_id, subject_relation, created_at`

func (s *Store) GetAuthzModel(ctx context.Context, appID string) (AuthzModel, error) {
	m := AuthzModel{}
	err := s.db.QueryRow(ctx, `select types, updated_at from authz_models where app_id = $1`, appID).Scan(&m.Types, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthzModel{Types: AuthzTypes{}}, ErrNotFound
	}
	return m, err
}

func (s *Store) SaveAuthzModel(ctx context.Context, appID string, types AuthzTypes) error {
	_, err := s.db.Exec(ctx, `insert into authz_models (app_id, types) values ($1, $2) on conflict (app_id) do update set types = excluded.types, updated_at = now()`, appID, types)
	return err
}

func (s *Store) DeleteAuthzModel(ctx context.Context, appID string) (int64, error) {
	var removed int64
	err := s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `delete from authz_tuples where app_id = $1`, appID)
		if err != nil {
			return err
		}
		removed = tag.RowsAffected()
		if tag, err = tx.db.Exec(ctx, `delete from authz_models where app_id = $1`, appID); err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	})
	return removed, err
}

func tupleArrays(tuples []Tuple) []any {
	cols := make([][]string, 6)
	for _, t := range tuples {
		for i, v := range []string{t.ObjectType, t.ObjectID, t.Relation, t.SubjectType, t.SubjectID, t.SubjectRelation} {
			cols[i] = append(cols[i], v)
		}
	}
	return []any{cols[0], cols[1], cols[2], cols[3], cols[4], cols[5]}
}

func (s *Store) WriteTuples(ctx context.Context, appID string, writes, deletes []Tuple) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from authz_tuples t using unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[]) d (ot, oi, r, st, si, sr)
			where t.app_id = $1 and t.object_type = d.ot and t.object_id = d.oi and t.relation = d.r and t.subject_type = d.st and t.subject_id = d.si and t.subject_relation = d.sr`, append([]any{appID}, tupleArrays(deletes)...)...); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into authz_tuples (app_id, object_type, object_id, relation, subject_type, subject_id, subject_relation)
			select $1, * from unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[]) on conflict do nothing`, append([]any{appID}, tupleArrays(writes)...)...)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrNotFound
		}
		return err
	})
}

func scanTuples(rows pgx.Rows, err error) ([]Tuple, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Tuple, error) {
		var t Tuple
		return t, row.Scan(&t.ObjectType, &t.ObjectID, &t.Relation, &t.SubjectType, &t.SubjectID, &t.SubjectRelation, &t.CreatedAt)
	})
}

func (s *Store) ReadTuples(ctx context.Context, appID string, f Tuple, limit int) ([]Tuple, error) {
	return scanTuples(s.db.Query(ctx, `select `+tupleColumns+` from authz_tuples where app_id = $1
		and ($2 = '' or object_type = $2) and ($3 = '' or object_id = $3) and ($4 = '' or relation = $4)
		and ($5 = '' or subject_type = $5) and ($6 = '' or subject_id = $6 and subject_relation = $7)
		order by object_type, object_id, relation, subject_type, subject_id, subject_relation limit $8`,
		appID, f.ObjectType, f.ObjectID, f.Relation, f.SubjectType, f.SubjectID, f.SubjectRelation, limit))
}

func (s *Store) RelatedTuples(ctx context.Context, appID, objectType, objectID, relation, userID string, groupIDs []string, limit int) ([]Tuple, error) {
	return scanTuples(s.db.Query(ctx, `select `+tupleColumns+` from authz_tuples where app_id = $1 and object_type = $2 and object_id = $3 and relation = $4
		and ($5 = '' or subject_type not in ('user', 'group') or subject_type = 'user' and subject_id = $5 or subject_type = 'group' and subject_id = any($6))
		order by subject_type, subject_id, subject_relation limit $7`, appID, objectType, objectID, relation, userID, groupIDs, limit))
}

func (s *Store) AuthzObjects(ctx context.Context, appID, objectType string, limit int) ([]string, error) {
	rows, err := s.db.Query(ctx, `select object_id from authz_tuples where app_id = $1 and object_type = $2
		union select subject_id from authz_tuples where app_id = $1 and subject_type = $2 order by 1 limit $3`, appID, objectType, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) PersonTuples(ctx context.Context, userID string, groupIDs []string, limit int) ([]PersonTuple, error) {
	rows, err := s.db.Query(ctx, `select a.id, a.name, t.object_type || ':' || t.object_id, t.relation, coalesce(t.group_id, ''), t.created_at
		from authz_tuples t join applications a on a.id = t.app_id where t.user_id = $1 or t.group_id = any($2)
		order by lower(a.name), t.object_type, t.object_id, t.relation limit $3`, userID, groupIDs, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PersonTuple, error) {
		var p PersonTuple
		return p, row.Scan(&p.AppID, &p.AppName, &p.Object, &p.Relation, &p.GroupID, &p.CreatedAt)
	})
}

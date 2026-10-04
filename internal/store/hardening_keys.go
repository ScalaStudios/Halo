package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

const SigningKeyOverlap = 7 * 24 * time.Hour

type SigningKeyInfo struct {
	ID        string     `json:"kid"`
	Algorithm string     `json:"algorithm"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"createdAt"`
	RetiresAt *time.Time `json:"retiresAt"`
	RetiredAt *time.Time `json:"retiredAt"`
}

func (s *Store) ListSigningKeyInfo(ctx context.Context) ([]SigningKeyInfo, error) {
	rows, err := s.db.Query(ctx, `select id, algorithm, created_at, retired_at, lag(created_at) over (order by created_at desc) from signing_keys order by created_at desc`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SigningKeyInfo, error) {
		var k SigningKeyInfo
		var superseded *time.Time
		err := row.Scan(&k.ID, &k.Algorithm, &k.CreatedAt, &k.RetiredAt, &superseded)
		k.Active = superseded == nil && k.RetiredAt == nil
		if superseded != nil && k.RetiredAt == nil {
			retires := superseded.Add(SigningKeyOverlap)
			k.RetiresAt = &retires
		}
		return k, err
	})
}

func (s *Store) RetireSigningKeys(ctx context.Context, now time.Time) error {
	_, err := s.db.Exec(ctx, `update signing_keys k set retired_at = $1 where retired_at is null
		and exists (select 1 from signing_keys n where n.created_at > k.created_at and n.created_at < $2)`, now, now.Add(-SigningKeyOverlap))
	return err
}

type SAMLKeyInfo struct {
	ID          string
	Certificate []byte
	CreatedAt   time.Time
	RetiredAt   *time.Time
}

func (s *Store) ListSAMLKeys(ctx context.Context) ([]SAMLKeyInfo, error) {
	rows, err := s.db.Query(ctx, `select id, certificate, created_at, retired_at from saml_keys order by retired_at is not null, created_at desc`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[SAMLKeyInfo])
}

func (s *Store) RotateSAMLKey(ctx context.Context, privateKey, certificate []byte) (string, error) {
	keyID := id.New("key")
	return keyID, s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `update saml_keys set retired_at = now() where retired_at is null`); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into saml_keys (id, private_key_sealed, certificate) values ($1, $2, $3)`, keyID, tx.Sealer.Seal(privateKey), certificate)
		return err
	})
}

func (s *Store) DeleteSAMLKey(ctx context.Context, keyID string) error {
	tag, err := s.db.Exec(ctx, `delete from saml_keys where id = $1 and retired_at is not null`, keyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) WithKey(key []byte) (*Store, error) {
	return New(s.pool, key)
}

var sealedColumns = []struct{ table, key, column string }{
	{"totp_secrets", "id", "secret_sealed"},
	{"signing_keys", "id", "private_key_sealed"},
	{"mail_outbox", "id", "body_sealed"},
	{"saml_keys", "id", "private_key_sealed"},
	{"app_provisioning", "app_id", "token_sealed"},
	{"identity_providers", "id", "client_secret_sealed"},
	{"webhook_endpoints", "id", "secret_sealed"},
	{"ssh_authority", "id", "private_key_sealed"},
}

type ResealedColumn struct {
	Name string
	Rows int
}

type KeyRotation struct {
	Resealed          []ResealedColumn
	RecoveryCodeUsers []string
}

func (s *Store) RotateSecretKey(ctx context.Context, next *secret.Sealer) (KeyRotation, error) {
	var out KeyRotation
	err := s.Tx(ctx, func(tx *Store) error {
		rows, err := tx.db.Query(ctx, `select table_name || '.' || column_name from information_schema.columns where table_schema = current_schema() and column_name like '%\_sealed' order by 1`)
		if err != nil {
			return err
		}
		present, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, name := range present {
			if !slices.ContainsFunc(sealedColumns, func(c struct{ table, key, column string }) bool { return c.table+"."+c.column == name }) {
				return fmt.Errorf("halo does not know how to re-seal %s, so nothing was changed. Upgrade Halo before rotating the key", name)
			}
		}
		for _, c := range sealedColumns {
			n, err := tx.reseal(ctx, c.table, c.key, c.column, next)
			if err != nil {
				return err
			}
			out.Resealed = append(out.Resealed, ResealedColumn{c.table + "." + c.column, n})
		}
		rows, err = tx.db.Query(ctx, `with removed as (delete from recovery_codes returning user_id)
			select u.email from users u where u.id in (select user_id from removed) order by lower(u.email)`)
		if err != nil {
			return err
		}
		out.RecoveryCodeUsers, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return out, err
}

func (s *Store) reseal(ctx context.Context, table, key, column string, next *secret.Sealer) (int, error) {
	rows, err := s.db.Query(ctx, `select `+key+`::text, `+column+` from `+table+` where `+column+` is not null`)
	if err != nil {
		return 0, err
	}
	type sealed struct {
		Key   string
		Value []byte
	}
	values, err := pgx.CollectRows(rows, pgx.RowToStructByPos[sealed])
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		plain, err := s.Sealer.Open(v.Value)
		if err != nil {
			return 0, fmt.Errorf("%s.%s for %s cannot be opened with HALO_SECRET_KEY, so nothing was changed. If you already rotated, HALO_SECRET_KEY must hold the new key", table, column, v.Key)
		}
		if _, err := s.db.Exec(ctx, `update `+table+` set `+column+` = $2 where `+key+`::text = $1`, v.Key, next.Seal(plain)); err != nil {
			return 0, err
		}
	}
	return len(values), nil
}

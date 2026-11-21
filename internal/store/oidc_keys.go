package store

import (
	"context"
	"time"

	"halo/internal/id"
)

var SigningAlgorithms = []string{"RS256", "ES256", "EdDSA"}

type SigningKey struct {
	ID          string
	Algorithm   string
	PrivateKey  []byte
	ExternalKey string
	CreatedAt   time.Time
}

func (s *Store) ListSigningKeys(ctx context.Context) ([]SigningKey, error) {
	rows, err := s.db.Query(ctx, `select id, algorithm, private_key_sealed, coalesce(external_key, ''), created_at from signing_keys where retired_at is null order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []SigningKey
	for rows.Next() {
		var k SigningKey
		var sealed []byte
		if err := rows.Scan(&k.ID, &k.Algorithm, &sealed, &k.ExternalKey, &k.CreatedAt); err != nil {
			return nil, err
		}
		if k.PrivateKey, err = s.openKey(sealed); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) CreateSigningKey(ctx context.Context, algorithm string, privateKey []byte, externalKey string) (SigningKey, error) {
	k := SigningKey{ID: id.New("key"), Algorithm: algorithm, PrivateKey: privateKey, ExternalKey: externalKey, CreatedAt: time.Now()}
	_, err := s.db.Exec(ctx, `insert into signing_keys (id, algorithm, private_key_sealed, external_key, created_at) values ($1, $2, $3, nullif($4, ''), $5)`,
		k.ID, k.Algorithm, s.sealKey(privateKey, externalKey), externalKey, k.CreatedAt)
	return k, err
}

package store

import (
	"context"
	"time"

	"halo/internal/id"
)

type SigningKey struct {
	ID         string
	Algorithm  string
	PrivateKey []byte
	CreatedAt  time.Time
}

func (s *Store) ListSigningKeys(ctx context.Context) ([]SigningKey, error) {
	rows, err := s.db.Query(ctx, `select id, algorithm, private_key_sealed, created_at from signing_keys where retired_at is null order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []SigningKey
	for rows.Next() {
		var k SigningKey
		var sealed []byte
		if err := rows.Scan(&k.ID, &k.Algorithm, &sealed, &k.CreatedAt); err != nil {
			return nil, err
		}
		if k.PrivateKey, err = s.Sealer.Open(sealed); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) CreateSigningKey(ctx context.Context, algorithm string, privateKey []byte) (SigningKey, error) {
	k := SigningKey{ID: id.New("key"), Algorithm: algorithm, PrivateKey: privateKey, CreatedAt: time.Now()}
	_, err := s.db.Exec(ctx, `insert into signing_keys (id, algorithm, private_key_sealed, created_at) values ($1, $2, $3, $4)`,
		k.ID, k.Algorithm, s.Sealer.Seal(privateKey), k.CreatedAt)
	return k, err
}

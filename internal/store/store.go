package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"halo/internal/secret"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrSlowDown = errors.New("polled faster than the interval")
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	pool    *pgxpool.Pool
	db      querier
	Sealer  *secret.Sealer
	codeKey []byte
}

const recoveryCodeLabel = "halo/recovery-codes"

func New(pool *pgxpool.Pool, key []byte) (*Store, error) {
	sealer, err := secret.NewSealer(key)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, db: pool, Sealer: sealer, codeKey: secret.Derive(key, recoveryCodeLabel)}, nil
}

func (s *Store) RecoveryCodeHash(code string) []byte {
	return secret.MAC(s.codeKey, code)
}

func (s *Store) Tx(ctx context.Context, fn func(*Store) error) error {
	if _, inTx := s.db.(pgx.Tx); inTx {
		return fn(s)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, codeKey: s.codeKey})
	})
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func conflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

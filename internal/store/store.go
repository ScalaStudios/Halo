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

type txKey struct{}

type routed struct{ pool *pgxpool.Pool }

func (r routed) on(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.pool
}

func (r routed) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return r.on(ctx).Exec(ctx, sql, args...)
}

func (r routed) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return r.on(ctx).Query(ctx, sql, args...)
}

func (r routed) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return r.on(ctx).QueryRow(ctx, sql, args...)
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
	return &Store{pool: pool, db: routed{pool}, Sealer: sealer, codeKey: secret.Derive(key, recoveryCodeLabel)}, nil
}

func (s *Store) RecoveryCodeHash(code string) []byte {
	return secret.MAC(s.codeKey, code)
}

func (s *Store) Tx(ctx context.Context, fn func(*Store) error) error {
	if _, inTx := s.db.(pgx.Tx); inTx {
		return fn(s)
	}
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, codeKey: s.codeKey})
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, codeKey: s.codeKey})
	})
}

func (s *Store) Bind(ctx context.Context) context.Context {
	if tx, ok := s.db.(pgx.Tx); ok {
		return context.WithValue(ctx, txKey{}, tx)
	}
	return ctx
}

func (s *Store) inTx(ctx context.Context) bool {
	_, inTx := s.db.(pgx.Tx)
	return inTx || ctx.Value(txKey{}) != nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) PoolStat() *pgxpool.Stat {
	return s.pool.Stat()
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

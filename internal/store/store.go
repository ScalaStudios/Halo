package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"halo/internal/kms"
	"halo/internal/secret"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrSlowDown      = errors.New("polled faster than the interval")
	ErrProofRequired = errors.New("identity check required")
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
	Keys    *kms.Keys
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
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, Keys: s.Keys, codeKey: s.codeKey})
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, Keys: s.Keys, codeKey: s.codeKey})
	})
}

func (s *Store) Savepoint(ctx context.Context, fn func(*Store) error) error {
	var begin interface {
		Begin(context.Context) (pgx.Tx, error)
	} = s.pool
	if tx, ok := s.db.(pgx.Tx); ok {
		begin = tx
	} else if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		begin = tx
	}
	return pgx.BeginFunc(ctx, begin, func(tx pgx.Tx) error {
		return fn(&Store{pool: s.pool, db: tx, Sealer: s.Sealer, Keys: s.Keys, codeKey: s.codeKey})
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

const leaseCheck = 2 * time.Second
const schemaLockHash = `(select hashtextextended(case n.nspname when 'public' then $1::text else format('%I.%s', n.nspname, $1::text) end, 0)
	from pg_catalog.pg_class c join pg_catalog.pg_namespace n on n.oid = c.relnamespace where c.oid = 'settings'::regclass)`

func (s *Store) connect(ctx context.Context) (*pgx.Conn, error) {
	conn, err := pgx.ConnectConfig(ctx, s.pool.Config().ConnConfig.Copy())
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, `select set_config('tcp_keepalives_idle', '10', false), set_config('tcp_keepalives_interval', '5', false), set_config('tcp_keepalives_count', '3', false)`); err != nil {
		conn.Close(context.Background())
		return nil, err
	}
	return conn, nil
}

func (s *Store) Lock(ctx context.Context, name string) (func(), error) {
	conn, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, `select pg_advisory_lock(`+schemaLockHash+`)`, name); err != nil {
		conn.Close(context.Background())
		return nil, err
	}
	return func() { conn.Close(context.Background()) }, nil
}

func (s *Store) Lead(ctx context.Context, run func(context.Context)) error {
	conn, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	for held := false; !held; {
		if err := conn.QueryRow(ctx, `select pg_try_advisory_lock(`+schemaLockHash+`)`, "halo:jobs").Scan(&held); err != nil {
			return err
		}
		if !held {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(leaseCheck):
			}
		}
	}
	leading, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(leading)
	}()
	defer func() {
		stop()
		<-done
	}()
	for {
		select {
		case <-done:
			return nil
		case <-time.After(leaseCheck):
			check, cancel := context.WithTimeout(ctx, leaseCheck)
			err := conn.Ping(check)
			cancel()
			if err != nil {
				return err
			}
		}
	}
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

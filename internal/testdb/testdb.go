package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"halo/internal/db"
	"halo/internal/store"
)

type queryCounter struct{ n atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func New(t *testing.T) *store.Store {
	t.Helper()
	st, _ := NewCounted(t)
	return st
}

func NewCounted(t *testing.T) (*store.Store, *atomic.Int64) {
	t.Helper()
	base := os.Getenv("HALO_TEST_DATABASE_URL")
	if base == "" {
		base = "postgres://halo:halo@localhost:5436/halo"
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Skipf("postgres unavailable at %s: %v (start it with: docker compose -f compose.dev.yml up -d)", base, err)
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "halo_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "create database "+name); err != nil {
		admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	st, err := store.New(pool, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "drop database if exists "+name+" with (force)")
		admin.Close(context.Background())
	})
	return st, &counter.n
}

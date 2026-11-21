package store

import (
	"context"
	"slices"
	"strings"
	"time"
)

var MethodUsing = map[string]string{"passkey": "a passkey", "security-key": "a security key", "totp": "an authenticator app", "magic-link": "a magic link", "recovery-codes": "a recovery code", "password": "a password", "federated": "an identity provider", "access-pass": "a temporary access pass", "certificate": "a client certificate"}

type AuthContext struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Methods   []string  `json:"methods"`
	MaxAge    int       `json:"maxAge"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (c AuthContext) Allows(method string) bool {
	return len(c.Methods) == 0 || slices.Contains(c.Methods, method)
}

func (c AuthContext) SatisfiedBy(s Session) bool {
	return c.Allows(s.Method) && (c.MaxAge == 0 || time.Since(s.CreatedAt) <= time.Duration(c.MaxAge)*time.Second)
}

func (c AuthContext) SignInAgain() string {
	ways := []string{}
	for _, m := range c.Methods {
		ways = append(ways, MethodUsing[m])
	}
	switch len(ways) {
	case 0:
		return "Sign in again."
	case 1:
		return "Sign in again with " + ways[0] + "."
	}
	return "Sign in again with " + strings.Join(ways[:len(ways)-1], ", ") + " or " + ways[len(ways)-1] + "."
}

const authContextColumns = `id, name, methods, max_age, created_at, updated_at`

func scanAuthContext(row rowScanner) (AuthContext, error) {
	var c AuthContext
	err := row.Scan(&c.ID, &c.Name, &c.Methods, &c.MaxAge, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) ListAuthContexts(ctx context.Context) ([]AuthContext, error) {
	rows, err := s.db.Query(ctx, `select `+authContextColumns+` from authentication_contexts order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contexts := []AuthContext{}
	for rows.Next() {
		c, err := scanAuthContext(rows)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, c)
	}
	return contexts, rows.Err()
}

func (s *Store) GetAuthContext(ctx context.Context, contextID string) (AuthContext, error) {
	c, err := scanAuthContext(s.db.QueryRow(ctx, `select `+authContextColumns+` from authentication_contexts where id = $1`, contextID))
	return c, notFound(err)
}

func (s *Store) CreateAuthContext(ctx context.Context, c AuthContext) (AuthContext, error) {
	created, err := scanAuthContext(s.db.QueryRow(ctx, `insert into authentication_contexts (id, name, methods, max_age) values ($1, $2, $3, $4) returning `+authContextColumns,
		c.ID, strings.TrimSpace(c.Name), nonNil(c.Methods), c.MaxAge))
	return created, conflict(err)
}

func (s *Store) UpdateAuthContext(ctx context.Context, c AuthContext) (AuthContext, error) {
	updated, err := scanAuthContext(s.db.QueryRow(ctx, `update authentication_contexts set name = $2, methods = $3, max_age = $4, updated_at = now() where id = $1 returning `+authContextColumns,
		c.ID, strings.TrimSpace(c.Name), nonNil(c.Methods), c.MaxAge))
	return updated, notFound(err)
}

func (s *Store) DeleteAuthContext(ctx context.Context, contextID string) error {
	tag, err := s.db.Exec(ctx, `delete from authentication_contexts where id = $1`, contextID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RequestedAuthContexts(ctx context.Context, ids []string) ([]AuthContext, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	all, err := s.ListAuthContexts(ctx)
	requested := []AuthContext{}
	for _, id := range ids {
		if i := slices.IndexFunc(all, func(c AuthContext) bool { return c.ID == id }); i >= 0 && !slices.ContainsFunc(requested, func(c AuthContext) bool { return c.ID == id }) {
			requested = append(requested, all[i])
		}
	}
	return requested, err
}

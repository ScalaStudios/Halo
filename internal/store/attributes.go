package store

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

const OmitClaim = "none"

var AttributeTypes = []string{"string", "number", "boolean", "date", "list"}

var ClaimSources = []string{"user.id", "user.email", "user.emailVerified", "user.name", "user.givenName", "user.familyName", "user.title", "user.department", "user.location", "user.groups", "user.roles"}

type Attribute struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Type        string    `json:"type"`
	SelfService bool      `json:"selfService"`
	CreatedAt   time.Time `json:"createdAt"`
	People      int       `json:"people"`
}

func (s *Store) ListAttributes(ctx context.Context) ([]Attribute, error) {
	rows, err := s.db.Query(ctx, `select id, name, label, type, self_service, created_at from user_attributes order by lower(label), name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attribute, error) {
		var a Attribute
		return a, row.Scan(&a.ID, &a.Name, &a.Label, &a.Type, &a.SelfService, &a.CreatedAt)
	})
}

func (s *Store) AttributeCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `select key, count(*) from users, jsonb_object_keys(attributes) key group by key`)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	var key string
	var count int
	_, err = pgx.ForEachRow(rows, []any{&key, &count}, func() error {
		counts[key] = count
		return nil
	})
	return counts, err
}

func (s *Store) CreateAttribute(ctx context.Context, a Attribute) (Attribute, error) {
	a.ID = id.New("atr")
	err := s.db.QueryRow(ctx, `insert into user_attributes (id, name, label, type, self_service) values ($1, $2, $3, $4, $5) returning created_at`, a.ID, a.Name, a.Label, a.Type, a.SelfService).Scan(&a.CreatedAt)
	return a, conflict(err)
}

func (s *Store) UpdateAttribute(ctx context.Context, a Attribute) error {
	tag, err := s.db.Exec(ctx, `update user_attributes set label = $2, self_service = $3, updated_at = now() where id = $1`, a.ID, a.Label, a.SelfService)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteAttribute(ctx context.Context, a Attribute) (int, error) {
	tag, err := s.db.Exec(ctx, `delete from user_attributes where id = $1`, a.ID)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, ErrNotFound
	}
	tag, err = s.db.Exec(ctx, `update users set attributes = attributes - $1::text where attributes ? $1::text`, a.Name)
	return int(tag.RowsAffected()), err
}

func (s *Store) ApplicationsMapping(ctx context.Context, source string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select name from applications where claim_mappings @> jsonb_build_array(jsonb_build_object('source', $1::text)) order by lower(name)`, source)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (u User) ClaimValue(source string, groups, roles []string) any {
	given, family, _ := strings.Cut(strings.TrimSpace(u.Name), " ")
	var v any
	switch source {
	case "user.id":
		v = u.ID
	case "user.email":
		v = u.Email
	case "user.emailVerified":
		v = u.EmailVerified
	case "user.name":
		v = u.Name
	case "user.givenName":
		v = given
	case "user.familyName":
		v = strings.TrimSpace(family)
	case "user.title":
		v = u.Title
	case "user.department":
		v = u.Department
	case "user.location":
		v = u.Location
	case "user.groups":
		v = groups
	case "user.roles":
		v = roles
	default:
		if name, ok := strings.CutPrefix(source, "attribute."); ok {
			v = u.Attributes[name]
		}
	}
	switch value := v.(type) {
	case string:
		if value == "" {
			return nil
		}
	case []string:
		if len(value) == 0 {
			return nil
		}
	case []any:
		if len(value) == 0 {
			return nil
		}
	}
	return v
}

func MapClaims(claims, mappings []Claim) []Claim {
	out := slices.Clone(claims)
	for _, m := range mappings {
		out = slices.DeleteFunc(out, func(c Claim) bool { return c.Name == m.Name })
		if m.Source != OmitClaim {
			out = append(out, m)
		}
	}
	return out
}

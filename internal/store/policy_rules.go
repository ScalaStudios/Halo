package store

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type PolicyConditions struct {
	AllUsers        bool     `json:"allUsers"`
	GroupIDs        []string `json:"groupIds"`
	UserIDs         []string `json:"userIds"`
	ExcludeGroupIDs []string `json:"excludeGroupIds"`
	ExcludeUserIDs  []string `json:"excludeUserIds"`
	AllApps         bool     `json:"allApps"`
	AppIDs          []string `json:"appIds"`
	Network         string   `json:"network"`
	ZoneIDs         []string `json:"zoneIds"`
	Device          string   `json:"device"`
	Risk            []string `json:"risk"`
	Methods         []string `json:"methods"`
}

func (c PolicyConditions) normalized() PolicyConditions {
	for _, list := range []*[]string{&c.GroupIDs, &c.UserIDs, &c.ExcludeGroupIDs, &c.ExcludeUserIDs, &c.AppIDs, &c.ZoneIDs, &c.Risk, &c.Methods} {
		clean := []string{}
		for _, v := range *list {
			if !slices.Contains(clean, v) {
				clean = append(clean, v)
			}
		}
		*list = clean
	}
	if c.AllUsers {
		c.GroupIDs, c.UserIDs = []string{}, []string{}
	}
	if c.AllApps {
		c.AppIDs = []string{}
	}
	if c.Network == "" {
		c.Network = "any"
	}
	if c.Network == "any" {
		c.ZoneIDs = []string{}
	}
	if c.Device == "" {
		c.Device = "any"
	}
	return c
}

type AccessPolicy struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Priority    int              `json:"priority"`
	Enabled     bool             `json:"enabled"`
	Mode        string           `json:"mode"`
	Effect      string           `json:"effect"`
	Conditions  PolicyConditions `json:"conditions"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

const policyColumns = `id, name, description, priority, enabled, mode, effect, conditions, created_at, updated_at`

func scanPolicy(row rowScanner) (AccessPolicy, error) {
	var p AccessPolicy
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Priority, &p.Enabled, &p.Mode, &p.Effect, &p.Conditions, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) ListAccessPolicies(ctx context.Context) ([]AccessPolicy, error) {
	rows, err := s.db.Query(ctx, `select `+policyColumns+` from access_policies order by priority, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []AccessPolicy{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) GetAccessPolicy(ctx context.Context, policyID string) (AccessPolicy, error) {
	p, err := scanPolicy(s.db.QueryRow(ctx, `select `+policyColumns+` from access_policies where id = $1`, policyID))
	return p, notFound(err)
}

func (s *Store) CreateAccessPolicy(ctx context.Context, p AccessPolicy) (AccessPolicy, error) {
	p.ID = id.New("pol")
	_, err := s.db.Exec(ctx, `insert into access_policies (id, name, description, priority, enabled, mode, effect, conditions)
		values ($1, $2, $3, (select coalesce(max(priority), 0) + 1 from access_policies), $4, $5, $6, $7)`,
		p.ID, strings.TrimSpace(p.Name), p.Description, p.Enabled, p.Mode, p.Effect, p.Conditions.normalized())
	if err != nil {
		return AccessPolicy{}, conflict(err)
	}
	return s.GetAccessPolicy(ctx, p.ID)
}

func (s *Store) UpdateAccessPolicy(ctx context.Context, p AccessPolicy) (AccessPolicy, error) {
	tag, err := s.db.Exec(ctx, `update access_policies set name = $2, description = $3, enabled = $4, mode = $5, effect = $6, conditions = $7, updated_at = now() where id = $1`,
		p.ID, strings.TrimSpace(p.Name), p.Description, p.Enabled, p.Mode, p.Effect, p.Conditions.normalized())
	if err != nil {
		return AccessPolicy{}, conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return AccessPolicy{}, ErrNotFound
	}
	return s.GetAccessPolicy(ctx, p.ID)
}

func (s *Store) DeleteAccessPolicy(ctx context.Context, policyID string) error {
	tag, err := s.db.Exec(ctx, `delete from access_policies where id = $1`, policyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReorderAccessPolicies(ctx context.Context, policyIDs []string) error {
	_, err := s.db.Exec(ctx, `update access_policies p set priority = o.n from unnest($1::text[]) with ordinality o(id, n) where p.id = o.id`, policyIDs)
	return err
}

type NetworkZone struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CIDRs     []string  `json:"cidrs"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Store) ListNetworkZones(ctx context.Context) ([]NetworkZone, error) {
	rows, err := s.db.Query(ctx, `select id, name, kind, cidrs, created_at, updated_at from network_zones order by lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	zones := []NetworkZone{}
	for rows.Next() {
		var z NetworkZone
		if err := rows.Scan(&z.ID, &z.Name, &z.Kind, &z.CIDRs, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

func (s *Store) SaveNetworkZone(ctx context.Context, z NetworkZone) (NetworkZone, error) {
	if z.ID == "" {
		z.ID = id.New("nz")
	}
	err := s.db.QueryRow(ctx, `insert into network_zones (id, name, kind, cidrs) values ($1, $2, $3, $4)
		on conflict (id) do update set name = excluded.name, kind = excluded.kind, cidrs = excluded.cidrs, updated_at = now()
		returning created_at, updated_at`, z.ID, strings.TrimSpace(z.Name), z.Kind, z.CIDRs).Scan(&z.CreatedAt, &z.UpdatedAt)
	return z, conflict(err)
}

func (s *Store) DeleteNetworkZone(ctx context.Context, zoneID string) error {
	tag, err := s.db.Exec(ctx, `delete from network_zones where id = $1`, zoneID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ExistingIDs(ctx context.Context, table string, ids []string) (map[string]bool, error) {
	existing := map[string]bool{}
	if len(ids) == 0 {
		return existing, nil
	}
	rows, err := s.db.Query(ctx, `select id from `+pgx.Identifier{table}.Sanitize()+` where id = any($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var found string
		if err := rows.Scan(&found); err != nil {
			return nil, err
		}
		existing[found] = true
	}
	return existing, rows.Err()
}

var MethodKinds = []string{"passkey", "security-key", "totp", "magic-link", "recovery-codes", "federated"}

type MethodSetting struct {
	Method    string    `json:"method"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Store) ListMethodSettings(ctx context.Context) ([]MethodSetting, error) {
	rows, err := s.db.Query(ctx, `select method, enabled, updated_at from sign_in_method_settings order by array_position($1::text[], method)`, MethodKinds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := []MethodSetting{}
	for rows.Next() {
		var m MethodSetting
		if err := rows.Scan(&m.Method, &m.Enabled, &m.UpdatedAt); err != nil {
			return nil, err
		}
		settings = append(settings, m)
	}
	return settings, rows.Err()
}

func (s *Store) SetMethodEnabled(ctx context.Context, method string, enabled bool) error {
	tag, err := s.db.Exec(ctx, `update sign_in_method_settings set enabled = $2, updated_at = now() where method = $1`, method, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type PolicyMatch struct {
	PolicyID string `json:"policyId"`
	Mode     string `json:"mode"`
	Outcome  string `json:"outcome"`
}

type PolicyEvaluation struct {
	SignInID string
	Risk     string
	Effect   string
	Matches  []PolicyMatch
}

func (s *Store) RecordPolicyEvaluation(ctx context.Context, e PolicyEvaluation) error {
	if e.Matches == nil {
		e.Matches = []PolicyMatch{}
	}
	_, err := s.db.Exec(ctx, `insert into policy_evaluations (sign_in_id, risk, effect, matches) values ($1, $2, $3, $4)`, e.SignInID, e.Risk, e.Effect, e.Matches)
	return err
}

type PolicyActivity struct {
	Matched int `json:"matched"`
	Stopped int `json:"stopped"`
}

func (s *Store) PolicyActivitySince(ctx context.Context, since time.Time) (map[string]PolicyActivity, error) {
	rows, err := s.db.Query(ctx, `select p.id, count(*), count(*) filter (where m->>'outcome' <> 'allow')
		from policy_evaluations e cross join lateral jsonb_array_elements(e.matches) m
		join access_policies p on p.id = m->>'policyId' and p.mode = m->>'mode'
		where e.time > $1 group by p.id`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activity := map[string]PolicyActivity{}
	for rows.Next() {
		var policyID string
		var a PolicyActivity
		if err := rows.Scan(&policyID, &a.Matched, &a.Stopped); err != nil {
			return nil, err
		}
		activity[policyID] = a
	}
	return activity, rows.Err()
}

func (s *Store) PurgePolicyEvaluations(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `delete from policy_evaluations where time < now() - interval '30 days'`)
	return err
}

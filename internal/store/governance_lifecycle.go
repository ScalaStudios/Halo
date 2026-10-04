package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type LifecycleAction struct {
	Type    string `json:"type"`
	GroupID string `json:"groupId,omitempty"`
}

type LifecycleRule struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Trigger   string            `json:"trigger"`
	Condition *string           `json:"condition"`
	Actions   []LifecycleAction `json:"actions"`
	Enabled   bool              `json:"enabled"`
	Runs      int               `json:"runs"`
	LastRunAt *time.Time        `json:"lastRunAt"`
	CreatedAt time.Time         `json:"createdAt"`
}

type LifecycleRuleInput struct {
	Name      string
	Trigger   string
	Condition *string
	Actions   []LifecycleAction
	Enabled   bool
}

type LifecycleRun struct {
	ID        string    `json:"id"`
	Rule      NamedRef  `json:"rule"`
	User      NamedRef  `json:"user"`
	Trigger   string    `json:"trigger"`
	Change    string    `json:"change"`
	Steps     []string  `json:"steps"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"createdAt"`
}

type LifecycleSnapshot struct {
	Department string
	Title      string
	Location   string
	Status     string
}

type LifecycleChange struct {
	User     User
	Previous *LifecycleSnapshot
}

func (s *Store) ListLifecycleRules(ctx context.Context) ([]LifecycleRule, error) {
	rows, err := s.db.Query(ctx, `select l.id, l.name, l.trigger, l.condition, l.actions, l.enabled,
		(select count(*) from lifecycle_runs r where r.rule_id = l.id), (select max(r.created_at) from lifecycle_runs r where r.rule_id = l.id), l.created_at
		from lifecycle_rules l order by l.created_at, l.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LifecycleRule, error) {
		var l LifecycleRule
		return l, row.Scan(&l.ID, &l.Name, &l.Trigger, &l.Condition, &l.Actions, &l.Enabled, &l.Runs, &l.LastRunAt, &l.CreatedAt)
	})
}

func (s *Store) GetLifecycleRule(ctx context.Context, ruleID string) (LifecycleRule, error) {
	rules, err := s.ListLifecycleRules(ctx)
	if err != nil {
		return LifecycleRule{}, err
	}
	for _, l := range rules {
		if l.ID == ruleID {
			return l, nil
		}
	}
	return LifecycleRule{}, ErrNotFound
}

func (s *Store) SaveLifecycleRule(ctx context.Context, ruleID string, in LifecycleRuleInput) (string, error) {
	if ruleID == "" {
		ruleID = id.New("lcr")
		_, err := s.db.Exec(ctx, `insert into lifecycle_rules (id, name, trigger, condition, actions, enabled) values ($1, $2, $3, $4, $5, $6)`,
			ruleID, in.Name, in.Trigger, in.Condition, in.Actions, in.Enabled)
		return ruleID, err
	}
	tag, err := s.db.Exec(ctx, `update lifecycle_rules set name = $2, trigger = $3, condition = $4, actions = $5, enabled = $6, updated_at = now() where id = $1`,
		ruleID, in.Name, in.Trigger, in.Condition, in.Actions, in.Enabled)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return ruleID, err
}

func (s *Store) DeleteLifecycleRule(ctx context.Context, ruleID string) error {
	tag, err := s.db.Exec(ctx, `delete from lifecycle_rules where id = $1`, ruleID)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

func (s *Store) ListLifecycleRuns(ctx context.Context, limit int) ([]LifecycleRun, error) {
	rows, err := s.db.Query(ctx, `select r.id, json_build_object('id', l.id, 'name', l.name), json_build_object('id', u.id, 'name', u.name), r.trigger, r.change, r.steps, r.result, r.created_at
		from lifecycle_runs r join lifecycle_rules l on l.id = r.rule_id join users u on u.id = r.user_id order by r.created_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LifecycleRun, error) {
		var r LifecycleRun
		return r, row.Scan(&r.ID, &r.Rule, &r.User, &r.Trigger, &r.Change, &r.Steps, &r.Result, &r.CreatedAt)
	})
}

func (s *Store) RecordLifecycleRun(ctx context.Context, r LifecycleRun) error {
	if r.ID == "" {
		r.ID = id.New("lcx")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(ctx, `insert into lifecycle_runs (id, rule_id, user_id, trigger, change, steps, result, created_at) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.Rule.ID, r.User.ID, r.Trigger, r.Change, r.Steps, r.Result, r.CreatedAt)
	return err
}

func (s *Store) PendingLifecycleChanges(ctx context.Context) ([]LifecycleChange, error) {
	rows, err := s.db.Query(ctx, `select u.id, u.email, u.name, u.title, u.department, u.location, u.status, u.source, s.user_id is not null, coalesce(s.department, ''), coalesce(s.title, ''), coalesce(s.location, ''), coalesce(s.status, '')
		from users u left join lifecycle_snapshots s on s.user_id = u.id
		where u.kind = 'person' and (s.user_id is null or s.department <> u.department or s.title <> u.title or s.location <> u.location or s.status <> u.status)
		order by u.created_at, u.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LifecycleChange, error) {
		var c LifecycleChange
		var seen bool
		var p LifecycleSnapshot
		if err := row.Scan(&c.User.ID, &c.User.Email, &c.User.Name, &c.User.Title, &c.User.Department, &c.User.Location, &c.User.Status, &c.User.Source, &seen, &p.Department, &p.Title, &p.Location, &p.Status); err != nil {
			return c, err
		}
		if seen {
			c.Previous = &p
		}
		return c, nil
	})
}

func (s *Store) ClaimLifecycleChange(ctx context.Context, c LifecycleChange) (bool, error) {
	u := c.User
	sql := `insert into lifecycle_snapshots (user_id, department, title, location, status) values ($1, $2, $3, $4, $5) on conflict (user_id) do nothing`
	args := []any{u.ID, u.Department, u.Title, u.Location, u.Status}
	if p := c.Previous; p != nil {
		sql = `update lifecycle_snapshots set department = $2, title = $3, location = $4, status = $5
			where user_id = $1 and department = $6 and title = $7 and location = $8 and status = $9`
		args = append(args, p.Department, p.Title, p.Location, p.Status)
	}
	tag, err := s.db.Exec(ctx, sql, args...)
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *Store) IsLastActiveGlobalAdmin(ctx context.Context, userID string) (bool, error) {
	var last bool
	err := s.db.QueryRow(ctx, `select exists (select 1 from user_roles where user_id = $1 and role = 'global_admin')
		and not exists (select 1 from user_roles r join users u on u.id = r.user_id where r.role = 'global_admin' and u.status = 'active' and u.id <> $1)`, userID).Scan(&last)
	return last, err
}

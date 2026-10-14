package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type RoleCount struct {
	Role    string
	Members int
}

type Digest struct {
	From, To       time.Time
	RiskEvents     int
	HighRiskEvents int
	RiskyUsers     int
	OpenRiskEvents int
	FailedSignIns  int
	RolesAssigned  int
	Roles          []RoleCount
	Recipients     []User
}

func (s *Store) ClaimWeeklyDigest(ctx context.Context) (bool, error) {
	tag, err := s.db.Exec(ctx, `insert into settings (key, value) values ('weeklyDigestSentAt', to_jsonb(now()))
		on conflict (key) do update set value = excluded.value, updated_at = now() where settings.updated_at <= now() - interval '7 days'`)
	return tag.RowsAffected() == 1, err
}

func (s *Store) WeeklyDigest(ctx context.Context, now time.Time) (Digest, error) {
	d := Digest{From: now.AddDate(0, 0, -7), To: now}
	err := s.db.QueryRow(ctx, `select
		(select count(*) from risk_events where time >= $1),
		(select count(*) from risk_events where time >= $1 and level = 'high'),
		(select count(distinct user_id) from risk_events where time >= $1),
		(select count(*) from risk_events where status = 'open'),
		(select count(*) from sign_in_events where time >= $1 and result = 'failure'),
		(select count(*) from user_roles where created_at >= $1)`, d.From).
		Scan(&d.RiskEvents, &d.HighRiskEvents, &d.RiskyUsers, &d.OpenRiskEvents, &d.FailedSignIns, &d.RolesAssigned)
	if err != nil {
		return d, err
	}
	rows, err := s.db.Query(ctx, `select r.role, count(*) from user_roles r join users u on u.id = r.user_id where u.status = 'active' group by r.role order by count(*) desc, r.role`)
	if err != nil {
		return d, err
	}
	if d.Roles, err = pgx.CollectRows(rows, pgx.RowToStructByPos[RoleCount]); err != nil {
		return d, err
	}
	rows, err = s.db.Query(ctx, `select distinct u.email, u.name from users u join user_roles r on r.user_id = u.id where r.role in ('global_admin', 'security_admin') and u.status = 'active' and u.kind = 'person' order by u.email`)
	if err != nil {
		return d, err
	}
	d.Recipients, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var u User
		return u, row.Scan(&u.Email, &u.Name)
	})
	return d, err
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type RoleActivation struct {
	ID            string    `json:"id"`
	Role          string    `json:"role"`
	Justification string    `json:"justification"`
	ActivatedAt   time.Time `json:"activatedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type ExpiredActivation struct {
	UserID   string
	UserName string
	Role     string
}

func (s *Store) SetEligibleRoles(ctx context.Context, userID string, roles []string, scopes map[string][]string) ([]string, error) {
	var ended []string
	err := s.Tx(ctx, func(tx *Store) error {
		privileged, err := tx.privileged(ctx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.db.Exec(ctx, `delete from role_eligibilities where user_id = $1 and role <> all($2)`, userID, roles); err != nil {
			return err
		}
		for _, role := range roles {
			if _, err := tx.db.Exec(ctx, `insert into role_eligibilities (user_id, role, scope) values ($1, $2, $3) on conflict (user_id, role) do update set scope = excluded.scope`, userID, role, scopes[role]); err != nil {
				return err
			}
		}
		rows, err := tx.db.Query(ctx, `update role_activations set ended_at = now(), end_reason = 'revoked'
			where user_id = $1 and ended_at is null and expires_at > now() and role <> all($2) returning role`, userID, roles)
		if err != nil {
			return err
		}
		if ended, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil || privileged || len(roles) == 0 {
			return err
		}
		return tx.revokeSetupCredentials(ctx, userID)
	})
	return ended, err
}

func (s *Store) ActivateRole(ctx context.Context, userID, role, justification string, expiresAt time.Time) (RoleActivation, error) {
	a := RoleActivation{ID: id.New("rla"), Role: role, Justification: justification, ExpiresAt: expiresAt}
	err := s.db.QueryRow(ctx, `insert into role_activations (id, user_id, role, justification, expires_at) select $1, $2, $3, $4, $5
		where exists (select 1 from role_eligibilities where user_id = $2 and role = $3) returning created_at`, a.ID, userID, role, justification, expiresAt).Scan(&a.ActivatedAt)
	return a, conflict(notFound(err))
}

func (s *Store) DeactivateRole(ctx context.Context, activationID, userID string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx, `update role_activations set ended_at = now(), end_reason = 'deactivated'
		where id = $1 and user_id = $2 and ended_at is null and expires_at > now() returning role`, activationID, userID).Scan(&role)
	return role, notFound(err)
}

func (s *Store) ExpireRoleActivations(ctx context.Context, now time.Time) ([]ExpiredActivation, error) {
	rows, err := s.db.Query(ctx, `update role_activations a set ended_at = a.expires_at, end_reason = 'expired' from users u
		where u.id = a.user_id and a.ended_at is null and a.expires_at <= $1 returning a.user_id, u.name, a.role`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ExpiredActivation])
}

package store

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

const ImpersonationLifetime = 30 * time.Minute

type Impersonation struct {
	ID        string     `json:"id"`
	Actor     NamedRef   `json:"actor"`
	User      NamedRef   `json:"user"`
	Reason    string     `json:"reason"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	StartedAt *time.Time `json:"startedAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
}

func (s Settings) ImpersonationRefusal(actor, target User) string {
	switch {
	case actor.Status != "active" || actor.Kind != "person" || !actor.HasRole("global_admin", "helpdesk_admin"):
		return "Only a helpdesk or global administrator signed in as themselves can impersonate someone."
	case target.ID == actor.ID:
		return "You can't impersonate yourself."
	case target.Kind != "person":
		return "Only people can be impersonated, not service accounts."
	case target.Status != "active":
		return target.Name + " isn't active, so they can't be impersonated."
	case target.Privileged() || slices.Contains(s.EmergencyAccessIDs, target.ID):
		return target.Name + " holds or is eligible for an administrator role, or is an emergency access account, so nobody can impersonate them."
	case len(s.ImpersonationGroups) == 0:
		return "Impersonation is turned off. A global administrator can choose whose accounts may be impersonated under Security defaults."
	case !slices.ContainsFunc(target.GroupIDs, func(g string) bool { return slices.Contains(s.ImpersonationGroups, g) }):
		return target.Name + " isn't in a group whose members may be impersonated."
	}
	return ""
}

const impersonationColumns = `i.id, i.actor_id, a.name, i.user_id, u.name, i.reason, i.created_at, i.started_at, i.expires_at,
	case when i.started_at is null then case when i.expires_at > now() then 'ready' else 'expired' end
		when exists (select 1 from sessions s where s.impersonation_id = i.id and s.revoked_at is null and s.expires_at > now()) then 'active'
		else 'ended' end
	from impersonations i join users a on a.id = i.actor_id join users u on u.id = i.user_id`

func scanImpersonation(row rowScanner) (Impersonation, error) {
	var i Impersonation
	err := row.Scan(&i.ID, &i.Actor.ID, &i.Actor.Name, &i.User.ID, &i.User.Name, &i.Reason, &i.CreatedAt, &i.StartedAt, &i.ExpiresAt, &i.Status)
	return i, err
}

func (s *Store) CreateImpersonation(ctx context.Context, actorID, userID, reason string, expiresAt time.Time) (Impersonation, error) {
	impersonationID := id.New("imp")
	_, err := s.db.Exec(ctx, `insert into impersonations (id, actor_id, user_id, reason, expires_at) values ($1, $2, $3, $4, $5)`, impersonationID, actorID, userID, reason, expiresAt)
	if err != nil {
		return Impersonation{}, err
	}
	return s.GetImpersonation(ctx, impersonationID)
}

func (s *Store) GetImpersonation(ctx context.Context, impersonationID string) (Impersonation, error) {
	i, err := scanImpersonation(s.db.QueryRow(ctx, `select `+impersonationColumns+` where i.id = $1`, impersonationID))
	return i, notFound(err)
}

func (s *Store) ListImpersonations(ctx context.Context, userID string) ([]Impersonation, error) {
	rows, err := s.db.Query(ctx, `select `+impersonationColumns+` where $1 = '' or i.user_id = $1 order by i.created_at desc limit 100`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Impersonation, error) { return scanImpersonation(row) })
}

func (s *Store) StartImpersonation(ctx context.Context, impersonationID string, in NewSession, until time.Time) (Session, string, error) {
	tag, err := s.db.Exec(ctx, `update impersonations set started_at = now(), expires_at = $2 where id = $1 and started_at is null and expires_at > now()`, impersonationID, until)
	if err != nil {
		return Session{}, "", err
	}
	if tag.RowsAffected() == 0 {
		return Session{}, "", ErrConflict
	}
	sess, token, err := s.CreateSession(ctx, in)
	if err != nil {
		return Session{}, "", err
	}
	if _, err := s.db.Exec(ctx, `update sessions set impersonation_id = $1, expires_at = $2 where id = $3`, impersonationID, until, sess.ID); err != nil {
		return Session{}, "", err
	}
	sess.ImpersonationID, sess.ExpiresAt = impersonationID, until
	return sess, token, nil
}

func (s *Store) EndImpersonation(ctx context.Context, impersonationID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `update impersonations set expires_at = now() where id = $1 and expires_at > now()`, impersonationID)
	if err != nil {
		return false, err
	}
	rows, err := s.db.Query(ctx, `update sessions set revoked_at = now() where impersonation_id = $1 and revoked_at is null returning id`, impersonationID)
	if err != nil {
		return false, err
	}
	ended, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0 || len(ended) > 0, s.revokeTokensForSessions(ctx, ended)
}

func (s *Store) ImpersonationValid(ctx context.Context, sess Session, target User) bool {
	if sess.ImpersonationID == "" {
		return true
	}
	i, err := s.GetImpersonation(ctx, sess.ImpersonationID)
	if err != nil || i.Status != "active" {
		return false
	}
	actor, err := s.GetUser(ctx, i.Actor.ID)
	if err != nil {
		return false
	}
	settings, err := s.Settings(ctx)
	return err == nil && settings.ImpersonationRefusal(actor, target) == ""
}

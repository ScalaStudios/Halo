package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Consent struct {
	AppID     string     `json:"appId"`
	Name      string     `json:"name"`
	Homepage  string     `json:"homepage"`
	LogoURL   *string    `json:"logoUrl"`
	Scopes    []APIScope `json:"scopes"`
	GrantedAt time.Time  `json:"grantedAt"`
}

func (s *Store) ConsentedScopes(ctx context.Context, userID, appID string) ([]string, error) {
	var scopes []string
	err := s.db.QueryRow(ctx, `select scopes from consent_grants where user_id = $1 and app_id = $2`, userID, appID).Scan(&scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return scopes, err
}

func (s *Store) GrantConsent(ctx context.Context, userID, appID string, scopes []string) error {
	_, err := s.db.Exec(ctx, `insert into consent_grants (user_id, app_id, scopes) values ($1, $2, $3)
		on conflict (user_id, app_id) do update set scopes = consent_grants.scopes || array(select s from unnest(excluded.scopes) s where s <> all(consent_grants.scopes)), granted_at = now()`,
		userID, appID, scopes)
	return err
}

func (s *Store) DescribeScopes(ctx context.Context, names []string) ([]APIScope, error) {
	rows, err := s.db.Query(ctx, `select u.name, coalesce((select r.description from api_resource_scopes r where r.name = u.name order by r.resource_id limit 1), '')
		from unnest($1::text[]) with ordinality u(name, n) order by u.n`, names)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[APIScope])
}

func (s *Store) ListConsents(ctx context.Context, userID string) ([]Consent, error) {
	rows, err := s.db.Query(ctx, `select a.id, a.name, a.homepage, a.logo_updated_at, g.scopes, g.granted_at from consent_grants g join applications a on a.id = g.app_id
		where g.user_id = $1 order by lower(a.name), a.id`, userID)
	if err != nil {
		return nil, err
	}
	var scopes [][]string
	consents, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Consent, error) {
		var c Consent
		var logo *time.Time
		var names []string
		err := row.Scan(&c.AppID, &c.Name, &c.Homepage, &logo, &names, &c.GrantedAt)
		if logo != nil {
			path := AppLogos.Path(c.AppID, *logo)
			c.LogoURL = &path
		}
		scopes = append(scopes, names)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	for i := range consents {
		if consents[i].Scopes, err = s.DescribeScopes(ctx, scopes[i]); err != nil {
			return nil, err
		}
	}
	return consents, nil
}

func (s *Store) RevokeConsent(ctx context.Context, userID string, app Application) error {
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `delete from consent_grants where user_id = $1 and app_id = $2`, userID, app.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.db.Exec(ctx, `delete from oidc_auth_requests where user_id = $1 and client_id = $2`, userID, app.ClientID); err != nil {
			return err
		}
		return tx.RevokeClientTokens(ctx, userID, app.ClientID)
	})
}

package store

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

const appColumns = `id, name, description, protocol, app_type, status, client_id, homepage, redirect_uris, post_logout_uris, scopes, access_token_ttl, refresh_token_ttl, id_token_ttl, refresh_rotation, setup_guide, owner_id, created_at`

func scanApp(row rowScanner) (Application, error) {
	var a Application
	err := row.Scan(&a.ID, &a.Name, &a.Description, &a.Protocol, &a.Type, &a.Status, &a.ClientID, &a.Homepage, &a.RedirectURIs, &a.PostLogoutURIs, &a.Scopes,
		&a.TokenPolicy.AccessTokenTTL, &a.TokenPolicy.RefreshTokenTTL, &a.TokenPolicy.IDTokenTTL, &a.TokenPolicy.Rotation, &a.SetupGuide, &a.Owner, &a.CreatedAt)
	return a, err
}

func (s *Store) ListApplications(ctx context.Context) ([]Application, error) {
	rows, err := s.db.Query(ctx, `select `+appColumns+` from applications order by name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []Application
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return apps, s.decorateApps(ctx, apps, true)
}

func (s *Store) GetApplication(ctx context.Context, appID string) (Application, error) {
	a, err := scanApp(s.db.QueryRow(ctx, `select `+appColumns+` from applications where id = $1`, appID))
	if err != nil {
		return Application{}, notFound(err)
	}
	apps := []Application{a}
	if err := s.decorateApps(ctx, apps, true); err != nil {
		return Application{}, err
	}
	return apps[0], nil
}

func (s *Store) GetApplicationByClientID(ctx context.Context, clientID string) (Application, error) {
	a, err := scanApp(s.db.QueryRow(ctx, `select `+appColumns+` from applications where client_id = $1`, clientID))
	if err != nil {
		return Application{}, notFound(err)
	}
	apps := []Application{a}
	if err := s.decorateApps(ctx, apps, false); err != nil {
		return Application{}, err
	}
	return apps[0], nil
}

type NewApplication struct {
	Name           string
	Description    string
	Protocol       string
	Type           string
	Homepage       string
	RedirectURIs   []string
	PostLogoutURIs []string
	Scopes         []string
	GroupIDs       []string
	SetupGuide     string
	OwnerID        *string
	ClientID       string
}

func (s *Store) CreateApplication(ctx context.Context, in NewApplication) (Application, error) {
	appID := id.New("app")
	clientID := in.ClientID
	if clientID == "" {
		clientID = "hl_" + strings.TrimPrefix(id.New("x"), "x_")
	}
	if len(in.Scopes) == 0 {
		in.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	if in.SetupGuide == "" {
		in.SetupGuide = "generic-oidc"
	}
	if in.RedirectURIs == nil {
		in.RedirectURIs = []string{}
	}
	if in.PostLogoutURIs == nil {
		in.PostLogoutURIs = []string{}
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return Application{}, err
	}
	err = s.Tx(ctx, func(tx *Store) error {
		_, err := tx.db.Exec(ctx, `insert into applications (id, name, description, protocol, app_type, client_id, homepage, redirect_uris, post_logout_uris, scopes, setup_guide, owner_id, access_token_ttl, id_token_ttl, refresh_token_ttl) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			appID, strings.TrimSpace(in.Name), in.Description, in.Protocol, in.Type, clientID, in.Homepage, in.RedirectURIs, in.PostLogoutURIs, in.Scopes, in.SetupGuide, in.OwnerID,
			settings.AccessTokenMinutes*60, settings.IDTokenMinutes*60, settings.RefreshTokenHours*3600)
		if err != nil {
			return conflict(err)
		}
		for _, groupID := range in.GroupIDs {
			if _, err := tx.db.Exec(ctx, `insert into app_groups (app_id, group_id) values ($1, $2) on conflict do nothing`, appID, groupID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Application{}, err
	}
	return s.GetApplication(ctx, appID)
}

func (s *Store) SetApplicationStatus(ctx context.Context, appID, status string) error {
	var clientID string
	err := s.db.QueryRow(ctx, `update applications set status = $2, updated_at = now() where id = $1 returning client_id`, appID, status).Scan(&clientID)
	if err != nil {
		return notFound(err)
	}
	if status != "disabled" {
		return nil
	}
	_, err = s.db.Exec(ctx, `update oidc_tokens set revoked_at = now() where client_id = $1 and revoked_at is null`, clientID)
	return err
}

func (s *Store) DeleteApplication(ctx context.Context, appID string) error {
	tag, err := s.db.Exec(ctx, `delete from applications where id = $1`, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetApplicationGroups(ctx context.Context, appID string, groupIDs []string) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `delete from app_groups where app_id = $1`, appID); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			if _, err := tx.db.Exec(ctx, `insert into app_groups (app_id, group_id) values ($1, $2)`, appID, groupID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) AddClientSecret(ctx context.Context, appID, label string, lifetime time.Duration) (Credential, string, error) {
	value := "hls_" + secret.Token(32)
	c := Credential{
		ID:        id.New("crd"),
		Kind:      "secret",
		Label:     label,
		Hint:      "hls_…" + value[len(value)-4:],
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(lifetime),
	}
	_, err := s.db.Exec(ctx, `insert into app_credentials (id, app_id, kind, label, secret_hash, hint, created_at, expires_at) values ($1, $2, 'secret', $3, $4, $5, $6, $7)`,
		c.ID, appID, c.Label, secret.Hash(value), c.Hint, c.CreatedAt, c.ExpiresAt)
	if err != nil {
		return Credential{}, "", err
	}
	return c, value, nil
}

func (s *Store) ExpireCredential(ctx context.Context, appID, credentialID string, at time.Time) error {
	tag, err := s.db.Exec(ctx, `update app_credentials set expires_at = least(expires_at, $3) where app_id = $1 and id = $2 and revoked_at is null`, appID, credentialID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeCredential(ctx context.Context, appID, credentialID string) error {
	tag, err := s.db.Exec(ctx, `update app_credentials set revoked_at = now() where app_id = $1 and id = $2 and revoked_at is null`, appID, credentialID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) VerifyClientSecret(ctx context.Context, clientID, value string) (Application, error) {
	app, err := s.GetApplicationByClientID(ctx, clientID)
	if err != nil {
		return Application{}, err
	}
	hash := secret.Hash(value)
	now := time.Now()
	for _, c := range app.Credentials {
		if c.Kind == "secret" && c.ExpiresAt.After(now) && secret.Equal(c.SecretHash, hash) {
			_, _ = s.db.Exec(ctx, `update app_credentials set last_used_at = now() where id = $1`, c.ID)
			return app, nil
		}
	}
	return Application{}, ErrNotFound
}

func (s *Store) decorateApps(ctx context.Context, apps []Application, withStats bool) error {
	if len(apps) == 0 {
		return nil
	}
	ids := make([]string, len(apps))
	index := map[string]int{}
	for i, a := range apps {
		ids[i] = a.ID
		index[a.ID] = i
		apps[i].GroupIDs = []string{}
		apps[i].Credentials = []Credential{}
		apps[i].Claims = claimsFor(a)
	}

	rows, err := s.db.Query(ctx, `select app_id, group_id from app_groups where app_id = any($1)`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var appID, groupID string
		if err := rows.Scan(&appID, &groupID); err != nil {
			rows.Close()
			return err
		}
		apps[index[appID]].GroupIDs = append(apps[index[appID]].GroupIDs, groupID)
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `select id, app_id, kind, label, hint, created_at, expires_at, last_used_at, secret_hash from app_credentials where revoked_at is null and app_id = any($1) order by created_at`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c Credential
		var appID string
		if err := rows.Scan(&c.ID, &appID, &c.Kind, &c.Label, &c.Hint, &c.CreatedAt, &c.ExpiresAt, &c.LastUsedAt, &c.SecretHash); err != nil {
			rows.Close()
			return err
		}
		apps[index[appID]].Credentials = append(apps[index[appID]].Credentials, c)
	}
	rows.Close()

	if !withStats {
		return nil
	}

	rows, err = s.db.Query(ctx, `select app_id, count(*), count(*) filter (where result <> 'success') from sign_in_events where app_id = any($1) and time > now() - interval '7 days' group by app_id`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var appID string
		var total, failed int
		if err := rows.Scan(&appID, &total, &failed); err != nil {
			rows.Close()
			return err
		}
		a := &apps[index[appID]]
		a.SignIns7d = total
		if total > 0 {
			a.FailureRate7d = math.Round(float64(failed)/float64(total)*1000) / 10
		}
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `with `+dynamicGroupsSQL+` select app_id, count(*) from (select distinct a.app_id, a.user_id from (
			select ag.app_id, m.user_id from app_groups ag join group_members m on m.group_id = ag.group_id where ag.app_id = any($1)
			union all
			select ag.app_id, u.id from app_groups ag join dynamic d on d.id = ag.group_id join users u on `+ruleMatchSQL+` where ag.app_id = any($1)
		) a join users u on u.id = a.user_id and u.kind = 'person' and u.status <> 'deprovisioned') assigned group by app_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var appID string
		var count int
		if err := rows.Scan(&appID, &count); err != nil {
			return err
		}
		apps[index[appID]].UserCount = count
	}
	return rows.Err()
}

func claimsFor(a Application) []Claim {
	if a.Protocol == "saml" {
		return SAMLClaims(DefaultSAMLSettings())
	}
	if a.Type == "service" {
		return []Claim{{Name: "sub", Source: "client.id"}, {Name: "scope", Source: "granted scopes"}}
	}
	claims := []Claim{{Name: "sub", Source: "user.id"}}
	if slices.Contains(a.Scopes, "email") {
		claims = append(claims, Claim{Name: "email", Source: "user.email"}, Claim{Name: "email_verified", Source: "user.emailVerified"})
	}
	if slices.Contains(a.Scopes, "profile") {
		claims = append(claims, Claim{Name: "name", Source: "user.name"}, Claim{Name: "preferred_username", Source: "user.email"})
	}
	if slices.Contains(a.Scopes, "groups") {
		claims = append(claims, Claim{Name: "groups", Source: "user.groups (every group the user belongs to)"})
	}
	return claims
}

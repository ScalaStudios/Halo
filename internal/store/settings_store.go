package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

type Settings struct {
	OrganizationName   string `json:"organizationName"`
	ContactEmail       string `json:"contactEmail"`
	SignInMessage      string `json:"signInMessage"`
	SessionHours       int    `json:"sessionHours"`
	LockoutThreshold   int    `json:"lockoutThreshold"`
	LockoutMinutes     int    `json:"lockoutMinutes"`
	InviteDays         int    `json:"inviteDays"`
	AccessTokenMinutes int    `json:"accessTokenMinutes"`
	IDTokenMinutes     int    `json:"idTokenMinutes"`
	RefreshTokenHours  int    `json:"refreshTokenHours"`
}

var DefaultSettings = Settings{
	SessionHours:       int(SessionLifetime / time.Hour),
	LockoutThreshold:   5,
	LockoutMinutes:     15,
	InviteDays:         int(EnrollmentLifetime / (24 * time.Hour)),
	AccessTokenMinutes: 15,
	IDTokenMinutes:     15,
	RefreshTokenHours:  8,
}

const settingsTTL = 30 * time.Second

type cachedSettings struct {
	loaded   time.Time
	settings Settings
}

var settingsCache sync.Map

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	if c, ok := settingsCache.Load(s.pool); ok && time.Since(c.(cachedSettings).loaded) < settingsTTL {
		return c.(cachedSettings).settings, nil
	}
	rows, err := s.db.Query(ctx, `select key, value from settings`)
	if err != nil {
		return Settings{}, err
	}
	values := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var value json.RawMessage
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return Settings{}, err
		}
		values[key] = value
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Settings{}, err
	}
	merged, _ := json.Marshal(values)
	settings := DefaultSettings
	if err := json.Unmarshal(merged, &settings); err != nil {
		return Settings{}, err
	}
	settingsCache.Store(s.pool, cachedSettings{loaded: time.Now(), settings: settings})
	return settings, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings Settings) error {
	encoded, _ := json.Marshal(settings)
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &values); err != nil {
		return err
	}
	for key, value := range values {
		if _, err := s.db.Exec(ctx, `insert into settings (key, value) values ($1, $2) on conflict (key) do update set value = excluded.value, updated_at = now()`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ForgetSettings() {
	settingsCache.Delete(s.pool)
}

func (s Settings) SessionLifetime() time.Duration {
	return time.Duration(s.SessionHours) * time.Hour
}

type Logo struct {
	ContentType string
	Data        []byte
	UpdatedAt   time.Time
}

func (s *Store) Logo(ctx context.Context) (Logo, error) {
	var l Logo
	err := s.db.QueryRow(ctx, `select content_type, data, updated_at from branding_logo`).Scan(&l.ContentType, &l.Data, &l.UpdatedAt)
	return l, notFound(err)
}

func (s *Store) LogoUpdatedAt(ctx context.Context) (*time.Time, error) {
	var at time.Time
	err := s.db.QueryRow(ctx, `select updated_at from branding_logo`).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &at, err
}

func (s *Store) SetLogo(ctx context.Context, contentType string, data []byte) error {
	_, err := s.db.Exec(ctx, `insert into branding_logo (content_type, data) values ($1, $2) on conflict (singleton) do update set content_type = excluded.content_type, data = excluded.data, updated_at = now()`, contentType, data)
	return err
}

func (s *Store) DeleteLogo(ctx context.Context) error {
	tag, err := s.db.Exec(ctx, `delete from branding_logo`)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type Domain struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Token      string     `json:"token"`
	CreatedAt  time.Time  `json:"createdAt"`
	VerifiedAt *time.Time `json:"verifiedAt"`
}

func (s *Store) ListDomains(ctx context.Context) ([]Domain, error) {
	rows, err := s.db.Query(ctx, `select id, name, token, created_at, verified_at from domains order by name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Domain])
}

func (s *Store) GetDomain(ctx context.Context, domainID string) (Domain, error) {
	rows, err := s.db.Query(ctx, `select id, name, token, created_at, verified_at from domains where id = $1`, domainID)
	if err != nil {
		return Domain{}, err
	}
	d, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Domain])
	return d, notFound(err)
}

func (s *Store) AddDomain(ctx context.Context, name string) (Domain, error) {
	d := Domain{ID: id.New("dom"), Name: name, Token: secret.Token(24)}
	err := s.db.QueryRow(ctx, `insert into domains (id, name, token) values ($1, $2, $3) returning created_at`, d.ID, d.Name, d.Token).Scan(&d.CreatedAt)
	return d, conflict(err)
}

func (s *Store) VerifyDomain(ctx context.Context, domainID string) (Domain, error) {
	if _, err := s.db.Exec(ctx, `update domains set verified_at = coalesce(verified_at, now()) where id = $1`, domainID); err != nil {
		return Domain{}, err
	}
	return s.GetDomain(ctx, domainID)
}

func (s *Store) DeleteDomain(ctx context.Context, domainID string) error {
	tag, err := s.db.Exec(ctx, `delete from domains where id = $1`, domainID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SeedSettings(ctx context.Context) error {
	return s.Tx(ctx, func(tx *Store) error {
		settings, err := tx.Settings(ctx)
		if err != nil {
			return err
		}
		settings.ContactEmail = "it@example.com"
		if err := tx.SaveSettings(ctx, settings); err != nil {
			return err
		}
		if _, err := tx.db.Exec(ctx, `insert into domains (id, name, token, created_at, verified_at) values ($1, 'example.com', $2, now() - interval '400 days', now() - interval '400 days')`, id.New("dom"), secret.Token(24)); err != nil {
			return err
		}
		_, _, err = tx.CreateWebhook(ctx, NewWebhook{URL: "http://localhost:9999/halo", Description: "Local test receiver", Events: []string{"*"}, Enabled: false})
		tx.ForgetSettings()
		return err
	})
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

type Theme struct {
	Accent     string `json:"accent"`
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Border     string `json:"border"`
	Text       string `json:"text"`
	Link       string `json:"link"`
}

type Settings struct {
	OrganizationName     string   `json:"organizationName"`
	ContactEmail         string   `json:"contactEmail"`
	SignInMessage        string   `json:"signInMessage"`
	Theme                Theme    `json:"theme"`
	CustomCSS            string   `json:"customCss"`
	ConsoleBranding      bool     `json:"consoleBranding"`
	SessionHours         int      `json:"sessionHours"`
	LockoutThreshold     int      `json:"lockoutThreshold"`
	LockoutMinutes       int      `json:"lockoutMinutes"`
	InviteDays           int      `json:"inviteDays"`
	AccessTokenMinutes   int      `json:"accessTokenMinutes"`
	IDTokenMinutes       int      `json:"idTokenMinutes"`
	RefreshTokenHours    int      `json:"refreshTokenHours"`
	PasswordSecondFactor bool     `json:"passwordSecondFactor"`
	PasswordGroupIDs     []string `json:"passwordGroupIds"`
	PasswordBreachCheck  bool     `json:"passwordBreachCheck"`
	EmailSignIn          string   `json:"emailSignIn"`

	SessionLimit       int                 `json:"sessionLimit"`
	SessionLimitAction string              `json:"sessionLimitAction"`
	SessionLimitCLI    bool                `json:"sessionLimitCli"`
	SessionLimitGroups []GroupSessionLimit `json:"sessionLimitGroups"`

	ApprovalActions    []string `json:"approvalActions"`
	EmergencyAccessIDs []string `json:"emergencyAccessIds"`
	ReauthMinutes      int      `json:"reauthMinutes"`

	ImpersonationGroups []string `json:"impersonationGroupIds"`

	ClientRegistration string `json:"clientRegistration"`

	BotProtection        string `json:"botProtection"`
	BotProtectionSiteKey string `json:"botProtectionSiteKey"`

	SignInLogDays int `json:"signInLogDays"`
	AuditLogDays  int `json:"auditLogDays"`

	EmailTemplates map[string]EmailTemplate `json:"emailTemplates,omitempty"`
}

type EmailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type GroupSessionLimit struct {
	GroupID string `json:"groupId"`
	Limit   int    `json:"limit"`
}

var DefaultSettings = Settings{
	SessionHours:         int(SessionLifetime / time.Hour),
	LockoutThreshold:     5,
	LockoutMinutes:       15,
	InviteDays:           int(EnrollmentLifetime / (24 * time.Hour)),
	AccessTokenMinutes:   15,
	IDTokenMinutes:       15,
	RefreshTokenHours:    8,
	PasswordSecondFactor: true,
	PasswordGroupIDs:     []string{},
	PasswordBreachCheck:  true,
	EmailSignIn:          "link",
	SessionLimitAction:   "refuse",
	SessionLimitGroups:   []GroupSessionLimit{},
	ApprovalActions:      []string{},
	EmergencyAccessIDs:   []string{},
	ImpersonationGroups:  []string{},
	ReauthMinutes:        10,
	ClientRegistration:   "off",
}

const settingsTTL = 30 * time.Second

type cachedSettings struct {
	loaded   time.Time
	settings Settings
}

var settingsCache sync.Map

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	if s.inTx(ctx) {
		return s.CurrentSettings(ctx)
	}
	if c, ok := settingsCache.Load(s.pool); ok && time.Since(c.(cachedSettings).loaded) < settingsTTL {
		return c.(cachedSettings).settings, nil
	}
	settings, err := s.CurrentSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	settingsCache.Store(s.pool, cachedSettings{loaded: time.Now(), settings: settings})
	return settings, nil
}

func (s *Store) CurrentSettings(ctx context.Context) (Settings, error) {
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
	return settings, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings Settings) error {
	encoded, _ := json.Marshal(settings)
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &values); err != nil {
		return err
	}
	return s.SaveSettingValues(ctx, values)
}

func (s *Store) SaveSettingValues(ctx context.Context, values map[string]json.RawMessage) error {
	for key, value := range values {
		if _, err := s.db.Exec(ctx, `insert into settings (key, value) values ($1, $2) on conflict (key) do update set value = excluded.value, updated_at = now()`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SaveEmailTemplate(ctx context.Context, kind string, t EmailTemplate) error {
	value, _ := json.Marshal(map[string]EmailTemplate{kind: t})
	_, err := s.db.Exec(ctx, `insert into settings (key, value) values ('emailTemplates', $1) on conflict (key) do update set value = settings.value || excluded.value, updated_at = now()`, value)
	return err
}

func (s *Store) ResetEmailTemplate(ctx context.Context, kind string) (bool, error) {
	tag, err := s.db.Exec(ctx, `update settings set value = value - $1::text, updated_at = now() where key = 'emailTemplates' and value ? $1::text`, kind)
	return err == nil && tag.RowsAffected() > 0, err
}

func (s *Store) BotProtectionSecret(ctx context.Context) (string, error) {
	var sealed []byte
	if err := s.db.QueryRow(ctx, `select secret_sealed from bot_protection`).Scan(&sealed); err != nil {
		return "", notFound(err)
	}
	plain, err := s.Sealer.Open(sealed)
	return string(plain), err
}

func (s *Store) SetBotProtectionSecret(ctx context.Context, value string) error {
	_, err := s.db.Exec(ctx, `insert into bot_protection (secret_sealed) values ($1) on conflict (singleton) do update set secret_sealed = excluded.secret_sealed, updated_at = now()`, s.Sealer.Seal([]byte(value)))
	return err
}

func (s *Store) ForgetSettings() {
	settingsCache.Delete(s.pool)
}

func (s Settings) SessionLifetime() time.Duration {
	return time.Duration(s.SessionHours) * time.Hour
}

func (s Settings) ReauthWindow() time.Duration {
	return time.Duration(s.ReauthMinutes) * time.Minute
}

func (s Settings) SessionLimitFor(u User) int {
	limit := 0
	for _, g := range s.SessionLimitGroups {
		if slices.Contains(u.GroupIDs, g.GroupID) && (limit == 0 || g.Limit < limit) {
			limit = g.Limit
		}
	}
	if limit == 0 {
		return s.SessionLimit
	}
	return limit
}

func (s Settings) ClientRegistrationOn() bool {
	return slices.Contains([]string{"open", "token", "approval"}, s.ClientRegistration)
}

func (s Settings) PasswordAllowed(u User) bool {
	return len(s.PasswordGroupIDs) == 0 || slices.ContainsFunc(u.GroupIDs, func(g string) bool { return slices.Contains(s.PasswordGroupIDs, g) })
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

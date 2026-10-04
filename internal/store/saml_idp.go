package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

const SAMLRequestLifetime = 15 * time.Minute

type SAMLAttributes struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
	Groups     string `json:"groups"`
}

type SAMLSettings struct {
	NameIDFormat string         `json:"nameIdFormat"`
	SignResponse bool           `json:"signResponse"`
	Attributes   SAMLAttributes `json:"attributes"`
	MetadataXML  string         `json:"metadataXml"`
}

func DefaultSAMLSettings() SAMLSettings {
	return SAMLSettings{NameIDFormat: "email", Attributes: SAMLAttributes{Email: "email", Name: "name", GivenName: "given_name", FamilyName: "family_name", Groups: "groups"}}
}

func SAMLClaims(s SAMLSettings) []Claim {
	subject := "user.email"
	if s.NameIDFormat == "persistent" {
		subject = "user.id"
	}
	claims := []Claim{{Name: "NameID", Source: subject}}
	for _, c := range []Claim{
		{s.Attributes.Email, "user.email"},
		{s.Attributes.Name, "user.name"},
		{s.Attributes.GivenName, "user.name (first word)"},
		{s.Attributes.FamilyName, "user.name (after the first word)"},
		{s.Attributes.Groups, "user.groups (every group the user belongs to)"},
	} {
		if c.Name != "" {
			claims = append(claims, c)
		}
	}
	return claims
}

func (s *Store) SAMLSettings(ctx context.Context, appID string) (SAMLSettings, error) {
	var settings SAMLSettings
	err := s.db.QueryRow(ctx, `select name_id_format, sign_response, attributes, metadata_xml from saml_service_providers where app_id = $1`, appID).
		Scan(&settings.NameIDFormat, &settings.SignResponse, &settings.Attributes, &settings.MetadataXML)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultSAMLSettings(), nil
	}
	return settings, err
}

func (s *Store) SetSAMLSettings(ctx context.Context, appID, entityID string, acsURLs []string, settings SAMLSettings) error {
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `update applications set client_id = $2, redirect_uris = $3, scopes = '{}', updated_at = now() where id = $1 and protocol = 'saml'`, appID, entityID, acsURLs)
		if err != nil {
			return conflict(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.db.Exec(ctx, `insert into saml_service_providers (app_id, name_id_format, sign_response, attributes, metadata_xml) values ($1, $2, $3, $4, $5)
			on conflict (app_id) do update set name_id_format = excluded.name_id_format, sign_response = excluded.sign_response, attributes = excluded.attributes, metadata_xml = excluded.metadata_xml, updated_at = now()`,
			appID, settings.NameIDFormat, settings.SignResponse, settings.Attributes, settings.MetadataXML)
		return err
	})
}

func (s *Store) SAMLApplication(ctx context.Context, appID string) (Application, error) {
	a, err := scanApp(s.db.QueryRow(ctx, `select `+appColumns+` from applications where id = $1 and protocol = 'saml'`, appID))
	if err != nil {
		return Application{}, notFound(err)
	}
	apps := []Application{a}
	if err := s.decorateApps(ctx, apps, false); err != nil {
		return Application{}, err
	}
	return apps[0], nil
}

type SAMLKey struct {
	ID          string
	PrivateKey  []byte
	Certificate []byte
}

func (s *Store) ActiveSAMLKey(ctx context.Context) (SAMLKey, error) {
	var k SAMLKey
	var sealed []byte
	if err := s.db.QueryRow(ctx, `select id, private_key_sealed, certificate from saml_keys where retired_at is null`).Scan(&k.ID, &sealed, &k.Certificate); err != nil {
		return SAMLKey{}, notFound(err)
	}
	var err error
	k.PrivateKey, err = s.Sealer.Open(sealed)
	return k, err
}

func (s *Store) CreateSAMLKey(ctx context.Context, privateKey, certificate []byte) error {
	_, err := s.db.Exec(ctx, `insert into saml_keys (id, private_key_sealed, certificate) values ($1, $2, $3) on conflict do nothing`, id.New("key"), s.Sealer.Seal(privateKey), certificate)
	return err
}

type SAMLRequest struct {
	ID         string
	Request    []byte
	RelayState string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

func (s *Store) CreateSAMLRequest(ctx context.Context, r SAMLRequest) (string, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = r.CreatedAt.Add(SAMLRequestLifetime)
	}
	r.ID = id.New("srq")
	_, err := s.db.Exec(ctx, `insert into saml_requests (id, request, relay_state, created_at, expires_at) values ($1, $2, $3, $4, $5)`, r.ID, r.Request, r.RelayState, r.CreatedAt, r.ExpiresAt)
	return r.ID, err
}

func (s *Store) TakeSAMLRequest(ctx context.Context, requestID string) (SAMLRequest, error) {
	r := SAMLRequest{ID: requestID}
	err := s.db.QueryRow(ctx, `delete from saml_requests where id = $1 and expires_at > now() returning request, relay_state, created_at, expires_at`, requestID).
		Scan(&r.Request, &r.RelayState, &r.CreatedAt, &r.ExpiresAt)
	return r, notFound(err)
}

func (s *Store) PurgeSAMLRequests(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `delete from saml_requests where expires_at < now()`)
	return err
}

func (s *Store) SeedSAML(ctx context.Context) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `update applications set setup_guide = case client_id when 'urn:amazon:webservices' then 'aws-iam-identity-center' when 'https://slack.com' then 'slack' else setup_guide end where protocol = 'saml'`); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into saml_service_providers (app_id, attributes) select id, $1::jsonb from applications where protocol = 'saml' on conflict do nothing`, DefaultSAMLSettings().Attributes)
		return err
	})
}

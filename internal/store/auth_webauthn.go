package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type WebAuthnCredential struct {
	ID              string
	UserID          string
	Kind            string
	Label           string
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       int64
	Transports      []string
	BackupEligible  bool
	BackupState     bool
}

const webauthnColumns = `id, user_id, kind, label, credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state`

func scanWebAuthn(row rowScanner) (WebAuthnCredential, error) {
	var c WebAuthnCredential
	err := row.Scan(&c.ID, &c.UserID, &c.Kind, &c.Label, &c.CredentialID, &c.PublicKey, &c.AttestationType, &c.AAGUID, &c.SignCount, &c.Transports, &c.BackupEligible, &c.BackupState)
	return c, err
}

func (s *Store) ListWebAuthnCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	rows, err := s.db.Query(ctx, `select `+webauthnColumns+` from webauthn_credentials where user_id = $1 order by created_at`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (WebAuthnCredential, error) { return scanWebAuthn(row) })
}

func (s *Store) WebAuthnCredentialByCredentialID(ctx context.Context, credentialID []byte) (WebAuthnCredential, error) {
	c, err := scanWebAuthn(s.db.QueryRow(ctx, `select `+webauthnColumns+` from webauthn_credentials where credential_id = $1`, credentialID))
	return c, notFound(err)
}

func (s *Store) AddWebAuthnCredential(ctx context.Context, c WebAuthnCredential) (Method, error) {
	m := Method{ID: id.New("mth"), Kind: c.Kind, Label: c.Label}
	err := s.db.QueryRow(ctx, `insert into webauthn_credentials (id, user_id, kind, label, credential_id, public_key, attestation_type, aaguid, sign_count, transports, backup_eligible, backup_state)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) returning created_at`,
		m.ID, c.UserID, c.Kind, c.Label, c.CredentialID, c.PublicKey, c.AttestationType, c.AAGUID, c.SignCount, c.Transports, c.BackupEligible, c.BackupState).Scan(&m.AddedAt)
	return m, conflict(err)
}

func (s *Store) TouchWebAuthnCredential(ctx context.Context, credentialID string, signCount int64, backupState bool) error {
	_, err := s.db.Exec(ctx, `update webauthn_credentials set sign_count = $2, backup_state = $3, last_used_at = now() where id = $1`, credentialID, signCount, backupState)
	return err
}

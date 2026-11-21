package store

import (
	"cmp"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type IdentityProof struct {
	ID          string     `json:"id"`
	Method      string     `json:"method"`
	Provider    string     `json:"provider"`
	Reference   string     `json:"reference"`
	Status      string     `json:"status"`
	Document    string     `json:"document"`
	Note        string     `json:"note"`
	CheckedBy   *NamedRef  `json:"checkedBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt"`
	UserID      *string    `json:"-"`
	SignUpID    *string    `json:"-"`
	Subject     string     `json:"-"`
}

type NewIdentityProof struct {
	UserID    string
	SignUpID  string
	Method    string
	Provider  string
	Reference string
	Status    string
	Document  string
	Note      string
	CheckedBy *string
}

var proofSelect = `select p.id, p.method, p.provider, p.reference, p.status, p.document, p.note, ` + govRef("c") + `,
	p.created_at, p.completed_at, p.user_id, p.sign_up_id, coalesce(u.name, s.name, '')
	from identity_proofs p left join users c on c.id = p.checked_by left join users u on u.id = p.user_id left join sign_ups s on s.id = p.sign_up_id `

func (s *Store) proofs(ctx context.Context, clause string, args ...any) ([]IdentityProof, error) {
	rows, err := s.db.Query(ctx, proofSelect+clause, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (IdentityProof, error) {
		var p IdentityProof
		err := row.Scan(&p.ID, &p.Method, &p.Provider, &p.Reference, &p.Status, &p.Document, &p.Note, &p.CheckedBy, &p.CreatedAt, &p.CompletedAt, &p.UserID, &p.SignUpID, &p.Subject)
		return p, err
	})
}

func (s *Store) proof(ctx context.Context, clause string, args ...any) (IdentityProof, error) {
	list, err := s.proofs(ctx, clause+` limit 1`, args...)
	if err != nil || len(list) == 0 {
		return IdentityProof{}, cmp.Or(err, ErrNotFound)
	}
	return list[0], nil
}

func (s *Store) ListIdentityProofs(ctx context.Context, userID string) ([]IdentityProof, error) {
	return s.proofs(ctx, `where p.user_id = $1 order by p.created_at desc, p.id desc`, userID)
}

func (s *Store) IdentityCheck(ctx context.Context, userID, signUpID string) (IdentityProof, error) {
	return s.proof(ctx, `where p.user_id = $1 or p.sign_up_id = $2 order by p.status = 'verified' desc, p.created_at desc, p.id desc`, userID, signUpID)
}

func (s *Store) PendingIdentityProofs(ctx context.Context) ([]IdentityProof, error) {
	return s.proofs(ctx, `where p.method = 'provider' and p.status = 'pending' and p.created_at > now() - interval '2 days' order by p.created_at limit 100`)
}

func (s *Store) CreateIdentityProof(ctx context.Context, in NewIdentityProof) (IdentityProof, error) {
	proofID := id.New("idp")
	_, err := s.db.Exec(ctx, `insert into identity_proofs (id, user_id, sign_up_id, method, provider, reference, status, document, note, checked_by, completed_at)
		values ($1, nullif($2, ''), nullif($3, ''), $4, $5, $6, $7, $8, $9, $10, case when $7 = 'pending' then null else now() end)`,
		proofID, in.UserID, in.SignUpID, in.Method, in.Provider, in.Reference, in.Status, in.Document, in.Note, in.CheckedBy)
	if err != nil {
		return IdentityProof{}, conflict(err)
	}
	return s.proof(ctx, `where p.id = $1`, proofID)
}

func (s *Store) CompleteIdentityProof(ctx context.Context, provider, reference, status string) (IdentityProof, error) {
	var proofID string
	err := s.db.QueryRow(ctx, `update identity_proofs set status = $3, completed_at = now() where provider = $1 and reference = $2 and status = 'pending' returning id`, provider, reference, status).Scan(&proofID)
	if err != nil {
		return IdentityProof{}, notFound(err)
	}
	return s.proof(ctx, `where p.id = $1`, proofID)
}

func (s *Store) UseProofingEvent(ctx context.Context, eventID string, expiresAt time.Time) error {
	tag, err := s.db.Exec(ctx, `insert into identity_proofing_events (id, expires_at) values ($1, $2) on conflict do nothing`, eventID, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

func (s *Store) ProofingSecrets(ctx context.Context) (string, string, error) {
	var key, hook []byte
	if err := s.db.QueryRow(ctx, `select api_key_sealed, webhook_secret_sealed from identity_proofing_secrets`).Scan(&key, &hook); err != nil {
		return "", "", notFound(err)
	}
	apiKey, err := s.Sealer.Open(key)
	if err != nil {
		return "", "", err
	}
	webhookSecret, err := s.Sealer.Open(hook)
	return string(apiKey), string(webhookSecret), err
}

func (s *Store) SetProofingSecrets(ctx context.Context, apiKey, webhookSecret string) error {
	currentKey, currentHook, err := s.ProofingSecrets(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	_, err = s.db.Exec(ctx, `insert into identity_proofing_secrets (api_key_sealed, webhook_secret_sealed) values ($1, $2)
		on conflict (singleton) do update set api_key_sealed = excluded.api_key_sealed, webhook_secret_sealed = excluded.webhook_secret_sealed, updated_at = now()`,
		s.Sealer.Seal([]byte(cmp.Or(apiKey, currentKey))), s.Sealer.Seal([]byte(cmp.Or(webhookSecret, currentHook))))
	return err
}

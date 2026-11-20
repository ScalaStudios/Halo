package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
	"halo/internal/secret"
)

const SignUpLifetime = 24 * time.Hour

type SignUp struct {
	ID         string     `json:"id"`
	UserID     string     `json:"-"`
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	VerifiedAt *time.Time `json:"verifiedAt"`
}

const signUpColumns = `id, user_id, email, name, created_at, verified_at`

func scanSignUp(row rowScanner) (SignUp, error) {
	var su SignUp
	err := row.Scan(&su.ID, &su.UserID, &su.Email, &su.Name, &su.CreatedAt, &su.VerifiedAt)
	return su, notFound(err)
}

func (s *Store) RequestSignUp(ctx context.Context, email, name string) (string, time.Time, error) {
	token, expires := secret.Token(32), time.Now().Add(SignUpLifetime)
	tag, err := s.db.Exec(ctx, `insert into sign_ups (id, user_id, email, name, token_hash, expires_at) values ($1, $2, $3, $4, $5, $6)
		on conflict ((lower(email))) do update set email = excluded.email, name = excluded.name, token_hash = excluded.token_hash, expires_at = excluded.expires_at, created_at = now()
		where sign_ups.verified_at is null`,
		id.New("sgn"), id.New("usr"), email, name, secret.Hash(token), expires)
	if err != nil || tag.RowsAffected() == 0 {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) PeekSignUp(ctx context.Context, token string) (SignUp, error) {
	return scanSignUp(s.db.QueryRow(ctx, `select `+signUpColumns+` from sign_ups where token_hash = $1 and verified_at is null and expires_at > now()`, secret.Hash(token)))
}

func (s *Store) VerifySignUp(ctx context.Context, token string, cred WebAuthnCredential) (SignUp, error) {
	return scanSignUp(s.db.QueryRow(ctx, `update sign_ups set verified_at = now(), token_hash = null, credential = $2
		where token_hash = $1 and verified_at is null and expires_at > now() returning `+signUpColumns, secret.Hash(token), cred))
}

func (s *Store) ListSignUps(ctx context.Context) ([]SignUp, error) {
	rows, err := s.db.Query(ctx, `select `+signUpColumns+` from sign_ups where verified_at is not null order by verified_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[SignUp])
}

func (s *Store) DeleteSignUp(ctx context.Context, signUpID string) (SignUp, error) {
	return scanSignUp(s.db.QueryRow(ctx, `delete from sign_ups where id = $1 and verified_at is not null returning `+signUpColumns, signUpID))
}

func (s *Store) ActivateSignUp(ctx context.Context, signUpID string, groupIDs []string) (User, error) {
	var u User
	err := s.Tx(ctx, func(tx *Store) error {
		var su SignUp
		var raw []byte
		err := tx.db.QueryRow(ctx, `delete from sign_ups where id = $1 and verified_at is not null returning `+signUpColumns+`, credential`, signUpID).
			Scan(&su.ID, &su.UserID, &su.Email, &su.Name, &su.CreatedAt, &su.VerifiedAt, &raw)
		if err != nil {
			return notFound(err)
		}
		var cred WebAuthnCredential
		if err := json.Unmarshal(raw, &cred); err != nil {
			return err
		}
		rows, err := tx.db.Query(ctx, `select id from groups where kind = 'assigned' and id = any($1) order by id`, groupIDs)
		if err != nil {
			return err
		}
		assigned, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if u, err = tx.CreateUser(ctx, NewUser{ID: su.UserID, Email: su.Email, Name: su.Name, Status: "active", Source: "Sign-up", GroupIDs: assigned}); err != nil {
			return err
		}
		if err := tx.MarkEmailVerified(ctx, u.ID); err != nil {
			return err
		}
		cred.UserID = u.ID
		_, err = tx.AddWebAuthnCredential(ctx, cred)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, u.ID)
}

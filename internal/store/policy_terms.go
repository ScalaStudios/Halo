package store

import (
	"context"
	"strings"
	"time"

	"halo/internal/id"
)

type Terms struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Text      string    `json:"text"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TermsAcceptance struct {
	TermsID    string    `json:"termsId"`
	Name       string    `json:"name"`
	Version    int       `json:"version"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

const termsColumns = `id, name, version, text, url, created_at, updated_at`

func scanTerms(row rowScanner) (Terms, error) {
	var t Terms
	err := row.Scan(&t.ID, &t.Name, &t.Version, &t.Text, &t.URL, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Store) ListTerms(ctx context.Context) ([]Terms, error) {
	rows, err := s.db.Query(ctx, `select `+termsColumns+` from terms order by lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	terms := []Terms{}
	for rows.Next() {
		t, err := scanTerms(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, t)
	}
	return terms, rows.Err()
}

func (s *Store) GetTerms(ctx context.Context, termsID string) (Terms, error) {
	t, err := scanTerms(s.db.QueryRow(ctx, `select `+termsColumns+` from terms where id = $1`, termsID))
	return t, notFound(err)
}

func (s *Store) SaveTerms(ctx context.Context, t Terms) (Terms, error) {
	query := `update terms set name = $2, text = $3, url = $4, updated_at = now(), version = version + (text <> $3 or url <> $4)::int
		where id = $1 returning ` + termsColumns
	if t.ID == "" {
		t.ID = id.New("tou")
		query = `insert into terms (id, name, text, url) values ($1, $2, $3, $4) returning ` + termsColumns
	}
	saved, err := scanTerms(s.db.QueryRow(ctx, query, t.ID, strings.TrimSpace(t.Name), t.Text, t.URL))
	return saved, notFound(conflict(err))
}

func (s *Store) DeleteTerms(ctx context.Context, termsID string) error {
	tag, err := s.db.Exec(ctx, `delete from terms where id = $1`, termsID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AcceptTerms(ctx context.Context, userID string, terms []Terms) error {
	for _, t := range terms {
		if _, err := s.db.Exec(ctx, `insert into terms_acceptances (user_id, terms_id, name, version) values ($1, $2, $3, $4) on conflict do nothing`, userID, t.ID, t.Name, t.Version); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) TermsAcceptances(ctx context.Context, userID string) ([]TermsAcceptance, error) {
	rows, err := s.db.Query(ctx, `select terms_id, name, version, accepted_at from terms_acceptances where user_id = $1 order by accepted_at desc, terms_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accepted := []TermsAcceptance{}
	for rows.Next() {
		var a TermsAcceptance
		if err := rows.Scan(&a.TermsID, &a.Name, &a.Version, &a.AcceptedAt); err != nil {
			return nil, err
		}
		accepted = append(accepted, a)
	}
	return accepted, rows.Err()
}

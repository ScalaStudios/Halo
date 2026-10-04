package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type AccessReview struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Group       NamedRef     `json:"group"`
	Reviewers   []NamedRef   `json:"reviewers"`
	DueAt       time.Time    `json:"dueAt"`
	AutoApply   bool         `json:"autoApply"`
	Status      string       `json:"status"`
	Total       int          `json:"total"`
	Decided     int          `json:"decided"`
	Removals    int          `json:"removals"`
	CreatedBy   *NamedRef    `json:"createdBy"`
	CreatedAt   time.Time    `json:"createdAt"`
	CompletedBy *NamedRef    `json:"completedBy"`
	CompletedAt *time.Time   `json:"completedAt"`
	Items       []ReviewItem `json:"items,omitempty"`
	CanDecide   bool         `json:"canDecide"`
	CanComplete bool         `json:"canComplete"`
}

type ReviewItem struct {
	User      NamedRef   `json:"user"`
	Title     string     `json:"title"`
	Decision  *string    `json:"decision"`
	Note      string     `json:"note"`
	DecidedBy *NamedRef  `json:"decidedBy"`
	DecidedAt *time.Time `json:"decidedAt"`
	Outcome   *string    `json:"outcome"`
}

type AccessReviewFilter struct {
	ID         string
	ReviewerID string
	Open       bool
}

type NewAccessReview struct {
	Name        string
	GroupID     string
	ReviewerIDs []string
	DueAt       time.Time
	AutoApply   bool
	CreatedBy   string
}

type ReviewCompletion struct {
	Kept       int
	Removed    []NamedRef
	NotRemoved int
	NoDecision int
}

var accessReviewSelect = `select v.id, v.name, json_build_object('id', coalesce(g.id, ''), 'name', coalesce(g.name, v.group_name)), ` +
	govRefs(`from access_review_reviewers rr join users x on x.id = rr.user_id where rr.review_id = v.id`) + `,
	v.due_at, v.auto_apply, v.status,
	(select count(*) from access_review_items i where i.review_id = v.id),
	(select count(*) from access_review_items i where i.review_id = v.id and i.decision is not null),
	(select count(*) from access_review_items i where i.review_id = v.id and i.decision = 'remove'),
	` + govRef("c") + `, v.created_at, ` + govRef("f") + `, v.completed_at
	from access_reviews v left join groups g on g.id = v.group_id left join users c on c.id = v.created_by left join users f on f.id = v.completed_by`

func (s *Store) ListAccessReviews(ctx context.Context, f AccessReviewFilter) ([]AccessReview, error) {
	rows, err := s.db.Query(ctx, accessReviewSelect+`
		where ($1 = '' or v.id = $1) and ($2 = '' or exists (select 1 from access_review_reviewers rr where rr.review_id = v.id and rr.user_id = $2))
		and (not $3 or v.status <> 'completed')
		order by v.status = 'completed', v.due_at`, f.ID, f.ReviewerID, f.Open)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AccessReview, error) {
		var v AccessReview
		err := row.Scan(&v.ID, &v.Name, &v.Group, &v.Reviewers, &v.DueAt, &v.AutoApply, &v.Status, &v.Total, &v.Decided, &v.Removals, &v.CreatedBy, &v.CreatedAt, &v.CompletedBy, &v.CompletedAt)
		return v, err
	})
}

func (s *Store) GetAccessReview(ctx context.Context, reviewID string) (AccessReview, error) {
	list, err := s.ListAccessReviews(ctx, AccessReviewFilter{ID: reviewID})
	if err != nil {
		return AccessReview{}, err
	}
	if len(list) == 0 {
		return AccessReview{}, ErrNotFound
	}
	review := list[0]
	rows, err := s.db.Query(ctx, `select json_build_object('id', u.id, 'name', u.name, 'email', u.email), u.title, i.decision, i.note, `+govRef("d")+`, i.decided_at, i.outcome
		from access_review_items i join users u on u.id = i.user_id left join users d on d.id = i.decided_by where i.review_id = $1 order by lower(u.name), u.id`, reviewID)
	if err != nil {
		return AccessReview{}, err
	}
	review.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReviewItem, error) {
		var i ReviewItem
		return i, row.Scan(&i.User, &i.Title, &i.Decision, &i.Note, &i.DecidedBy, &i.DecidedAt, &i.Outcome)
	})
	return review, err
}

func (s *Store) CreateAccessReview(ctx context.Context, in NewAccessReview) (string, error) {
	reviewID := id.New("rev")
	err := s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `insert into access_reviews (id, name, group_id, due_at, auto_apply, created_by) values ($1, $2, $3, $4, $5, $6)`,
			reviewID, in.Name, in.GroupID, in.DueAt, in.AutoApply, in.CreatedBy); err != nil {
			return err
		}
		if _, err := tx.db.Exec(ctx, `insert into access_review_reviewers (review_id, user_id) select $1, unnest($2::text[])`, reviewID, in.ReviewerIDs); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `insert into access_review_items (review_id, user_id) select $1, m.user_id from group_members m join users u on u.id = m.user_id
			where m.group_id = $2 and u.status <> 'deprovisioned'`, reviewID, in.GroupID)
		return err
	})
	return reviewID, err
}

func (s *Store) DecideReviewItem(ctx context.Context, reviewID, userID, decision, note, deciderID string, now time.Time) error {
	tag, err := s.db.Exec(ctx, `update access_review_items i set decision = $3, note = $4, decided_by = $5, decided_at = $6
		where i.review_id = $1 and i.user_id = $2 and exists (select 1 from access_reviews v where v.id = i.review_id and v.status <> 'completed')`,
		reviewID, userID, decision, note, deciderID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CompleteAccessReview(ctx context.Context, reviewID, actorID string, now time.Time) (ReviewCompletion, error) {
	var out ReviewCompletion
	err := s.Tx(ctx, func(tx *Store) error {
		var groupID, status string
		var autoApply bool
		if err := tx.db.QueryRow(ctx, `select coalesce(group_id, ''), auto_apply, status from access_reviews where id = $1 for update`, reviewID).Scan(&groupID, &autoApply, &status); err != nil {
			return notFound(err)
		}
		if status == "completed" {
			return ErrConflict
		}
		if _, err := tx.db.Exec(ctx, `update access_review_items set outcome = case when decision = 'keep' then 'kept' when decision = 'remove' and $2 then 'removed'
			when decision = 'remove' then 'not-removed' else 'no-decision' end where review_id = $1`, reviewID, autoApply); err != nil {
			return err
		}
		if autoApply {
			rows, err := tx.db.Query(ctx, `with removed as (
					delete from group_members m using access_review_items i where i.review_id = $1 and i.decision = 'remove' and m.group_id = $2 and m.user_id = i.user_id returning m.user_id
				) select u.id, u.name, u.email from removed join users u on u.id = removed.user_id order by lower(u.name)`, reviewID, groupID)
			if err != nil {
				return err
			}
			out.Removed, err = pgx.CollectRows(rows, pgx.RowToStructByPos[NamedRef])
			if err != nil {
				return err
			}
		}
		if err := tx.db.QueryRow(ctx, `select count(*) filter (where outcome = 'kept'), count(*) filter (where outcome = 'not-removed'), count(*) filter (where outcome = 'no-decision')
			from access_review_items where review_id = $1`, reviewID).Scan(&out.Kept, &out.NotRemoved, &out.NoDecision); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `update access_reviews set status = 'completed', completed_by = $2, completed_at = $3 where id = $1`, reviewID, actorID, now)
		return err
	})
	return out, err
}

func (s *Store) MarkOverdueReviews(ctx context.Context, now time.Time) ([]NamedRef, error) {
	rows, err := s.db.Query(ctx, `update access_reviews set status = 'overdue' where status = 'in-progress' and due_at < $1 returning id, name, ''`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[NamedRef])
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type NamedRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

func govRefs(from string) string {
	return `(select coalesce(json_agg(json_build_object('id', x.id, 'name', x.name) order by lower(x.name)), '[]') ` + from + `)`
}

func govRef(alias string) string {
	return `case when ` + alias + `.id is null then null else json_build_object('id', ` + alias + `.id, 'name', ` + alias + `.name) end`
}

type AccessPackage struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description"`
	Owner                *NamedRef  `json:"owner"`
	Groups               []NamedRef `json:"groups"`
	Approvers            []NamedRef `json:"approvers"`
	MaxDays              *int       `json:"maxDays"`
	RequireJustification bool       `json:"requireJustification"`
	Archived             bool       `json:"archived"`
	PendingRequests      int        `json:"pendingRequests"`
	ActiveGrants         int        `json:"activeGrants"`
	CreatedAt            time.Time  `json:"createdAt"`
}

type AccessPackageInput struct {
	Name                 string
	Description          string
	OwnerID              *string
	GroupIDs             []string
	ApproverIDs          []string
	MaxDays              *int
	RequireJustification bool
	Archived             bool
}

var accessPackageSelect = `select p.id, p.name, p.description, ` + govRef("o") + `, p.max_days, p.require_justification, p.archived, p.created_at, ` +
	govRefs(`from access_package_groups pg join groups x on x.id = pg.group_id where pg.package_id = p.id`) + `, ` +
	govRefs(`from access_package_approvers pa join users x on x.id = pa.user_id where pa.package_id = p.id`) + `,
	(select count(*) from access_requests r where r.package_id = p.id and r.status = 'pending'),
	(select count(*) from access_requests r where r.package_id = p.id and r.status = 'approved' and r.ended_at is null)
	from access_packages p left join users o on o.id = p.owner_id`

func (s *Store) ListAccessPackages(ctx context.Context) ([]AccessPackage, error) {
	rows, err := s.db.Query(ctx, accessPackageSelect+` order by lower(p.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AccessPackage, error) {
		var p AccessPackage
		err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Owner, &p.MaxDays, &p.RequireJustification, &p.Archived, &p.CreatedAt, &p.Groups, &p.Approvers, &p.PendingRequests, &p.ActiveGrants)
		return p, err
	})
}

func (s *Store) GetAccessPackage(ctx context.Context, packageID string) (AccessPackage, error) {
	packages, err := s.ListAccessPackages(ctx)
	if err != nil {
		return AccessPackage{}, err
	}
	for _, p := range packages {
		if p.ID == packageID {
			return p, nil
		}
	}
	return AccessPackage{}, ErrNotFound
}

func (s *Store) SaveAccessPackage(ctx context.Context, packageID string, in AccessPackageInput) (string, error) {
	err := s.Tx(ctx, func(tx *Store) error {
		if packageID == "" {
			packageID = id.New("pkg")
			if _, err := tx.db.Exec(ctx, `insert into access_packages (id, name, description, owner_id, max_days, require_justification, archived) values ($1, $2, $3, $4, $5, $6, $7)`,
				packageID, in.Name, in.Description, in.OwnerID, in.MaxDays, in.RequireJustification, in.Archived); err != nil {
				return conflict(err)
			}
		} else {
			tag, err := tx.db.Exec(ctx, `update access_packages set name = $2, description = $3, owner_id = $4, max_days = $5, require_justification = $6, archived = $7, updated_at = now() where id = $1`,
				packageID, in.Name, in.Description, in.OwnerID, in.MaxDays, in.RequireJustification, in.Archived)
			if err != nil {
				return conflict(err)
			}
			if tag.RowsAffected() == 0 {
				return ErrNotFound
			}
		}
		for _, link := range []struct {
			table, column string
			ids           []string
		}{{"access_package_groups", "group_id", in.GroupIDs}, {"access_package_approvers", "user_id", in.ApproverIDs}} {
			if _, err := tx.db.Exec(ctx, `delete from `+link.table+` where package_id = $1`, packageID); err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `insert into `+link.table+` (package_id, `+link.column+`) select $1, unnest($2::text[])`, packageID, link.ids); err != nil {
				return err
			}
		}
		return nil
	})
	return packageID, err
}

func (s *Store) DeleteAccessPackage(ctx context.Context, packageID string) error {
	tag, err := s.db.Exec(ctx, `delete from access_packages p where p.id = $1 and not exists (select 1 from access_requests r where r.package_id = p.id)`, packageID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return s.existsOr(ctx, `select exists (select 1 from access_packages where id = $1)`, packageID)
}

func (s *Store) existsOr(ctx context.Context, sql, key string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, sql, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrConflict
	}
	return ErrNotFound
}

type AccessRequest struct {
	ID            string     `json:"id"`
	Package       NamedRef   `json:"package"`
	Requester     NamedRef   `json:"requester"`
	Justification string     `json:"justification"`
	DurationDays  *int       `json:"durationDays"`
	Status        string     `json:"status"`
	DecidedBy     *NamedRef  `json:"decidedBy"`
	DecidedAt     *time.Time `json:"decidedAt"`
	DecisionNote  string     `json:"decisionNote"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	EndedAt       *time.Time `json:"endedAt"`
	EndReason     *string    `json:"endReason"`
	Groups        []NamedRef `json:"groups"`
	Approvers     []NamedRef `json:"approvers"`
	CreatedAt     time.Time  `json:"createdAt"`
	CanDecide     bool       `json:"canDecide"`
}

type AccessRequestFilter struct {
	ID           string
	RequesterID  string
	Statuses     []string
	ActiveGrants bool
}

var accessRequestSelect = `select r.id, json_build_object('id', p.id, 'name', p.name), json_build_object('id', u.id, 'name', u.name, 'email', u.email),
	r.justification, r.duration_days, r.status, ` + govRef("d") + `, r.decided_at, r.decision_note, r.expires_at, r.ended_at, r.end_reason, r.created_at,
	case when r.status = 'approved' then ` + govRefs(`from access_grant_groups gg join groups x on x.id = gg.group_id where gg.request_id = r.id`) + `
	else ` + govRefs(`from access_package_groups pg join groups x on x.id = pg.group_id where pg.package_id = p.id`) + ` end, ` +
	govRefs(`from access_package_approvers pa join users x on x.id = pa.user_id where pa.package_id = p.id`) + `
	from access_requests r join access_packages p on p.id = r.package_id join users u on u.id = r.requester_id left join users d on d.id = r.decided_by`

func (s *Store) ListAccessRequests(ctx context.Context, f AccessRequestFilter) ([]AccessRequest, error) {
	rows, err := s.db.Query(ctx, accessRequestSelect+`
		where ($1 = '' or r.id = $1) and ($2 = '' or r.requester_id = $2) and ($3::text[] is null or r.status = any($3))
		and (not $4 or (r.status = 'approved' and r.ended_at is null))
		order by r.created_at desc limit 500`, f.ID, f.RequesterID, f.Statuses, f.ActiveGrants)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AccessRequest, error) {
		var r AccessRequest
		err := row.Scan(&r.ID, &r.Package, &r.Requester, &r.Justification, &r.DurationDays, &r.Status, &r.DecidedBy, &r.DecidedAt, &r.DecisionNote,
			&r.ExpiresAt, &r.EndedAt, &r.EndReason, &r.CreatedAt, &r.Groups, &r.Approvers)
		return r, err
	})
}

func (s *Store) GetAccessRequest(ctx context.Context, requestID string) (AccessRequest, error) {
	list, err := s.ListAccessRequests(ctx, AccessRequestFilter{ID: requestID})
	if err != nil {
		return AccessRequest{}, err
	}
	if len(list) == 0 {
		return AccessRequest{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Store) CreateAccessRequest(ctx context.Context, packageID, requesterID, justification string, durationDays *int) (string, error) {
	requestID := id.New("req")
	_, err := s.db.Exec(ctx, `insert into access_requests (id, package_id, requester_id, justification, duration_days) values ($1, $2, $3, $4, $5)`,
		requestID, packageID, requesterID, justification, durationDays)
	return requestID, conflict(err)
}

func (s *Store) EnsureGroupMember(ctx context.Context, groupID, userID string) (bool, error) {
	var member bool
	if err := s.db.QueryRow(ctx, `select exists (select 1 from group_members where group_id = $1 and user_id = $2)`, groupID, userID).Scan(&member); err != nil || member {
		return false, err
	}
	return true, s.AddGroupMember(ctx, groupID, userID)
}

func (s *Store) ApproveAccessRequest(ctx context.Context, requestID, deciderID, note string, now time.Time) error {
	return s.Tx(ctx, func(tx *Store) error {
		var status, requesterID, packageID string
		var days *int
		err := tx.db.QueryRow(ctx, `select status, requester_id, package_id, duration_days from access_requests where id = $1 for update`, requestID).Scan(&status, &requesterID, &packageID, &days)
		if err != nil {
			return notFound(err)
		}
		if status != "pending" {
			return ErrConflict
		}
		rows, err := tx.db.Query(ctx, `select group_id from access_package_groups where package_id = $1`, packageID)
		if err != nil {
			return err
		}
		groupIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			added, err := tx.EnsureGroupMember(ctx, groupID, requesterID)
			if err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `insert into access_grant_groups (request_id, group_id, added) values ($1, $2, $3)`, requestID, groupID, added); err != nil {
				return err
			}
		}
		var expires *time.Time
		if days != nil {
			t := now.Add(time.Duration(*days) * 24 * time.Hour)
			expires = &t
		}
		_, err = tx.db.Exec(ctx, `update access_requests set status = 'approved', decided_by = $2, decided_at = $3, decision_note = $4, expires_at = $5 where id = $1`,
			requestID, deciderID, now, note, expires)
		return err
	})
}

func (s *Store) DenyAccessRequest(ctx context.Context, requestID, deciderID, note string, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set status = 'denied', decided_by = $2, decided_at = $3, decision_note = $4 where id = $1 and status = 'pending'`,
		requestID, deciderID, now, note)
}

func (s *Store) CancelAccessRequest(ctx context.Context, requestID, requesterID string, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set status = 'cancelled', decided_at = $3 where id = $1 and requester_id = $2 and status = 'pending'`,
		requestID, requesterID, now)
}

func (s *Store) closePendingRequest(ctx context.Context, sql, requestID string, args ...any) error {
	tag, err := s.db.Exec(ctx, sql, append([]any{requestID}, args...)...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return s.existsOr(ctx, `select exists (select 1 from access_requests where id = $1)`, requestID)
}

func (s *Store) EndAccessGrant(ctx context.Context, requestID, reason string, now time.Time) ([]NamedRef, error) {
	removed := []NamedRef{}
	err := s.Tx(ctx, func(tx *Store) error {
		var userID string
		err := tx.db.QueryRow(ctx, `update access_requests set ended_at = $2, end_reason = $3 where id = $1 and status = 'approved' and ended_at is null returning requester_id`,
			requestID, now, reason).Scan(&userID)
		if err != nil {
			return notFound(err)
		}
		rows, err := tx.db.Query(ctx, `select g.id, g.name, (select o.request_id from access_grant_groups o join access_requests r on r.id = o.request_id
			where o.group_id = gg.group_id and r.requester_id = $2 and r.status = 'approved' and r.ended_at is null order by r.expires_at desc nulls first limit 1)
			from access_grant_groups gg join groups g on g.id = gg.group_id where gg.request_id = $1 and gg.added`, requestID, userID)
		if err != nil {
			return err
		}
		type held struct {
			group NamedRef
			heir  *string
		}
		owned, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (held, error) {
			var h held
			return h, row.Scan(&h.group.ID, &h.group.Name, &h.heir)
		})
		if err != nil {
			return err
		}
		for _, h := range owned {
			if h.heir != nil {
				if _, err := tx.db.Exec(ctx, `update access_grant_groups set added = true where request_id = $1 and group_id = $2`, *h.heir, h.group.ID); err != nil {
					return err
				}
				continue
			}
			tag, err := tx.db.Exec(ctx, `delete from group_members where group_id = $1 and user_id = $2`, h.group.ID, userID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				removed = append(removed, h.group)
			}
		}
		return nil
	})
	return removed, err
}

func (s *Store) ExpiredAccessGrants(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.Query(ctx, `select id from access_requests where status = 'approved' and ended_at is null and expires_at <= $1 order by expires_at`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

type ApproverContact struct {
	Email string
	Admin bool
}

func (s *Store) AccessApproverContacts(ctx context.Context, packageID, requesterID string) ([]ApproverContact, error) {
	rows, err := s.db.Query(ctx, `with approvers as (
			select u.id, u.email from access_package_approvers pa join users u on u.id = pa.user_id where pa.package_id = $1 and u.id <> $2 and u.status = 'active'
		), recipients as (
			select id, email from approvers
			union
			select u.id, u.email from user_roles ur join users u on u.id = ur.user_id
			where ur.role = 'global_admin' and u.id <> $2 and u.status = 'active' and not exists (select 1 from approvers)
		)
		select email, exists (select 1 from user_roles ur where ur.user_id = recipients.id) from recipients order by email`, packageID, requesterID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApproverContact, error) {
		var c ApproverContact
		return c, row.Scan(&c.Email, &c.Admin)
	})
}

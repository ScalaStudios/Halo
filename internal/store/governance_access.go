package store

import (
	"context"
	"errors"
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

type AccessStage struct {
	Approver    string     `json:"approver"`
	Approvers   []NamedRef `json:"approvers"`
	Group       *NamedRef  `json:"group"`
	Fallback    []NamedRef `json:"fallback"`
	TimeoutDays *int       `json:"timeoutDays"`
}

type AccessQuestion struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"`
	Choices  []string `json:"choices"`
	Required bool     `json:"required"`
}

type AccessPackage struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Owner                *NamedRef        `json:"owner"`
	Groups               []NamedRef       `json:"groups"`
	Approvers            []NamedRef       `json:"approvers"`
	Stages               []AccessStage    `json:"stages"`
	Questions            []AccessQuestion `json:"questions"`
	MaxDays              *int             `json:"maxDays"`
	RequireJustification bool             `json:"requireJustification"`
	Archived             bool             `json:"archived"`
	PendingRequests      int              `json:"pendingRequests"`
	ActiveGrants         int              `json:"activeGrants"`
	CreatedAt            time.Time        `json:"createdAt"`
}

type AccessStageInput struct {
	Approver    string
	ApproverIDs []string
	GroupID     *string
	FallbackIDs []string
	TimeoutDays *int
}

type AccessPackageInput struct {
	Name                 string
	Description          string
	OwnerID              *string
	GroupIDs             []string
	ApproverIDs          []string
	Stages               []AccessStageInput
	Questions            []AccessQuestion
	MaxDays              *int
	RequireJustification bool
	Archived             bool
}

func stageRefs(fallback string) string {
	return govRefs(`from access_package_approvers pa join users x on x.id = pa.user_id where pa.package_id = s.package_id and pa.stage = s.stage and ` + fallback + `pa.fallback`)
}

var accessPackageSelect = `select p.id, p.name, p.description, ` + govRef("o") + `, p.max_days, p.require_justification, p.archived, p.created_at, ` +
	govRefs(`from access_package_groups pg join groups x on x.id = pg.group_id where pg.package_id = p.id`) + `, ` +
	govRefs(`from (select distinct x.id, x.name from access_package_approvers pa join users x on x.id = pa.user_id where pa.package_id = p.id and not pa.fallback) x`) + `,
	(select coalesce(json_agg(json_build_object('approver', s.approver, 'approvers', ` + stageRefs("not ") + `, 'group', ` + govRef("sg") + `,
		'fallback', ` + stageRefs("") + `, 'timeoutDays', s.timeout_days) order by s.stage), '[]')
		from access_package_stages s left join groups sg on sg.id = s.group_id where s.package_id = p.id), p.questions,
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
		err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Owner, &p.MaxDays, &p.RequireJustification, &p.Archived, &p.CreatedAt, &p.Groups, &p.Approvers, &p.Stages, &p.Questions, &p.PendingRequests, &p.ActiveGrants)
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
	if len(in.Stages) == 0 {
		in.Stages = []AccessStageInput{{Approver: "people", ApproverIDs: in.ApproverIDs}}
	}
	if in.Questions == nil {
		in.Questions = []AccessQuestion{}
	}
	err := s.Tx(ctx, func(tx *Store) error {
		if packageID == "" {
			packageID = id.New("pkg")
			if _, err := tx.db.Exec(ctx, `insert into access_packages (id, name, description, owner_id, max_days, require_justification, archived, questions) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
				packageID, in.Name, in.Description, in.OwnerID, in.MaxDays, in.RequireJustification, in.Archived, in.Questions); err != nil {
				return conflict(err)
			}
		} else {
			tag, err := tx.db.Exec(ctx, `update access_packages set name = $2, description = $3, owner_id = $4, max_days = $5, require_justification = $6, archived = $7, questions = $8, updated_at = now() where id = $1`,
				packageID, in.Name, in.Description, in.OwnerID, in.MaxDays, in.RequireJustification, in.Archived, in.Questions)
			if err != nil {
				return conflict(err)
			}
			if tag.RowsAffected() == 0 {
				return ErrNotFound
			}
		}
		for _, table := range []string{"access_package_groups", "access_package_approvers", "access_package_stages"} {
			if _, err := tx.db.Exec(ctx, `delete from `+table+` where package_id = $1`, packageID); err != nil {
				return err
			}
		}
		if _, err := tx.db.Exec(ctx, `insert into access_package_groups (package_id, group_id) select $1, unnest($2::text[])`, packageID, in.GroupIDs); err != nil {
			return err
		}
		for i, st := range in.Stages {
			if _, err := tx.db.Exec(ctx, `insert into access_package_stages (package_id, stage, approver, group_id, timeout_days) values ($1, $2, $3, $4, $5)`,
				packageID, i+1, st.Approver, st.GroupID, st.TimeoutDays); err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `insert into access_package_approvers (package_id, stage, fallback, user_id) select $1::text, $2::int, false, unnest($3::text[])
				union select $1::text, $2::int, true, unnest($4::text[])`, packageID, i+1, st.ApproverIDs, st.FallbackIDs); err != nil {
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

type AccessAnswer struct {
	QuestionID string `json:"questionId"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

type StageApproval struct {
	Stage int       `json:"stage"`
	By    *NamedRef `json:"by"`
	Note  string    `json:"note"`
	At    time.Time `json:"at"`
}

type AccessRequest struct {
	ID             string          `json:"id"`
	Package        NamedRef        `json:"package"`
	Requester      NamedRef        `json:"requester"`
	RequestedBy    *NamedRef       `json:"requestedBy"`
	Extends        *string         `json:"extends"`
	Justification  string          `json:"justification"`
	Answers        []AccessAnswer  `json:"answers"`
	DurationDays   *int            `json:"durationDays"`
	Status         string          `json:"status"`
	Stage          int             `json:"stage"`
	Stages         int             `json:"stages"`
	StageDueAt     *time.Time      `json:"stageDueAt"`
	StageApprovals []StageApproval `json:"stageApprovals"`
	DecidedBy      *NamedRef       `json:"decidedBy"`
	DecidedAt      *time.Time      `json:"decidedAt"`
	DecisionNote   string          `json:"decisionNote"`
	ExpiresAt      *time.Time      `json:"expiresAt"`
	EndedAt        *time.Time      `json:"endedAt"`
	EndReason      *string         `json:"endReason"`
	Groups         []NamedRef      `json:"groups"`
	Approvers      []NamedRef      `json:"approvers"`
	CreatedAt      time.Time       `json:"createdAt"`
	CanDecide      bool            `json:"canDecide"`
}

type AccessRequestFilter struct {
	ID           string
	RequesterID  string
	Person       string
	Statuses     []string
	ActiveGrants bool
}

func stagePool(fallback bool) string {
	match := `exists (select 1 from access_package_approvers a where a.package_id = r.package_id and a.stage = r.stage and a.fallback and a.user_id = ap.id)`
	if !fallback {
		match = `(exists (select 1 from access_package_approvers a where a.package_id = r.package_id and a.stage = r.stage and not a.fallback and a.user_id = ap.id)
		or exists (select 1 from access_package_stages s join users b on b.id = r.requester_id where s.package_id = r.package_id and s.stage = r.stage and s.approver = 'manager' and ap.id = coalesce(b.manager_id, b.sponsor_id))
		or exists (select 1 from access_package_stages s join group_owners o on o.group_id = s.group_id where s.package_id = r.package_id and s.stage = r.stage and s.approver = 'group-owners' and o.user_id = ap.id))`
	}
	return `select ap.id, ap.name, ap.email from users ap where ap.status = 'active' and ap.kind = 'person' and not ap.guest and ap.id <> r.requester_id and ap.id is distinct from r.requested_by and ` + match
}

var currentApprovers = stagePool(false) + ` union ` + stagePool(true) + ` and (r.escalated or not exists (` + stagePool(false) + `))`

var accessRequestSelect = `select r.id, json_build_object('id', p.id, 'name', p.name), json_build_object('id', u.id, 'name', u.name, 'email', u.email), ` + govRef("rb") + `, r.extends_id,
	r.justification, r.answers, r.duration_days, r.status, r.stage, greatest(r.stage, (select count(*) from access_package_stages c where c.package_id = r.package_id)),
	case when r.status = 'pending' and st.timeout_days is not null then r.stage_started_at + st.timeout_days * interval '1 day' end,
	(select coalesce(json_agg(json_build_object('stage', a.stage, 'by', ` + govRef("x") + `, 'note', a.note, 'at', a.decided_at) order by a.stage), '[]')
		from access_request_approvals a left join users x on x.id = a.decided_by where a.request_id = r.id),
	` + govRef("d") + `, r.decided_at, r.decision_note, r.expires_at, r.ended_at, r.end_reason, r.created_at,
	case when r.status = 'approved' then ` + govRefs(`from access_grant_groups gg join groups x on x.id = gg.group_id where gg.request_id = r.id`) + `
	else ` + govRefs(`from access_package_groups pg join groups x on x.id = pg.group_id where pg.package_id = p.id`) + ` end,
	case when r.status = 'pending' then ` + govRefs(`from (`+currentApprovers+`) x`) + ` else '[]' end
	from access_requests r join access_packages p on p.id = r.package_id join users u on u.id = r.requester_id left join users d on d.id = r.decided_by
	left join users rb on rb.id = r.requested_by left join access_package_stages st on st.package_id = r.package_id and st.stage = r.stage`

func (s *Store) ListAccessRequests(ctx context.Context, f AccessRequestFilter) ([]AccessRequest, error) {
	rows, err := s.db.Query(ctx, accessRequestSelect+`
		where ($1 = '' or r.id = $1) and ($2 = '' or r.requester_id = $2) and ($3 = '' or r.requester_id = $3 or r.requested_by = $3) and ($4::text[] is null or r.status = any($4))
		and (not $5 or (r.status = 'approved' and r.ended_at is null))
		order by r.created_at desc limit 500`, f.ID, f.RequesterID, f.Person, f.Statuses, f.ActiveGrants)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AccessRequest, error) {
		var r AccessRequest
		err := row.Scan(&r.ID, &r.Package, &r.Requester, &r.RequestedBy, &r.Extends, &r.Justification, &r.Answers, &r.DurationDays, &r.Status, &r.Stage, &r.Stages, &r.StageDueAt,
			&r.StageApprovals, &r.DecidedBy, &r.DecidedAt, &r.DecisionNote, &r.ExpiresAt, &r.EndedAt, &r.EndReason, &r.CreatedAt, &r.Groups, &r.Approvers)
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

type NewAccessRequest struct {
	PackageID     string
	RequesterID   string
	RequestedBy   *string
	ExtendsID     *string
	Justification string
	Answers       []AccessAnswer
	DurationDays  *int
}

func (s *Store) CreateAccessRequest(ctx context.Context, in NewAccessRequest) (string, error) {
	requestID := id.New("req")
	if in.Answers == nil {
		in.Answers = []AccessAnswer{}
	}
	_, err := s.db.Exec(ctx, `insert into access_requests (id, package_id, requester_id, requested_by, extends_id, justification, answers, duration_days) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		requestID, in.PackageID, in.RequesterID, in.RequestedBy, in.ExtendsID, in.Justification, in.Answers, in.DurationDays)
	return requestID, conflict(err)
}

func (s *Store) EnsureGroupMember(ctx context.Context, groupID, userID string) (bool, error) {
	var member bool
	if err := s.db.QueryRow(ctx, `select exists (select 1 from group_members where group_id = $1 and user_id = $2)`, groupID, userID).Scan(&member); err != nil || member {
		return false, err
	}
	return true, s.AddGroupMember(ctx, groupID, userID)
}

func (s *Store) ApproveAccessRequest(ctx context.Context, requestID string, stage int, deciderID, note string, now time.Time) (bool, error) {
	final := false
	err := s.Tx(ctx, func(tx *Store) error {
		var status, requesterID, packageID string
		var days *int
		var current int
		var extends *string
		err := tx.db.QueryRow(ctx, `select status, requester_id, package_id, duration_days, stage, extends_id from access_requests where id = $1 for update`, requestID).Scan(&status, &requesterID, &packageID, &days, &current, &extends)
		if err != nil {
			return notFound(err)
		}
		if status != "pending" || current != stage {
			return ErrConflict
		}
		if _, err := tx.db.Exec(ctx, `insert into access_request_approvals (request_id, stage, decided_by, note, decided_at) values ($1, $2, $3, $4, $5)`, requestID, stage, deciderID, note, now); err != nil {
			return err
		}
		var next *int
		if err := tx.db.QueryRow(ctx, `select min(stage) from access_package_stages where package_id = $1 and stage > $2`, packageID, stage).Scan(&next); err != nil {
			return err
		}
		if next != nil {
			_, err := tx.db.Exec(ctx, `update access_requests set stage = $2, stage_started_at = $3, escalated = false where id = $1`, requestID, *next, now)
			return err
		}
		final = true
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
		start := now
		if extends != nil {
			var until *time.Time
			err := tx.db.QueryRow(ctx, `select expires_at from access_requests where id = $1 and status = 'approved' and ended_at is null`, *extends).Scan(&until)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if until != nil && until.After(now) {
				start = *until
			}
		}
		var expires *time.Time
		if days != nil {
			t := start.Add(time.Duration(*days) * 24 * time.Hour)
			expires = &t
		}
		if _, err := tx.db.Exec(ctx, `update access_requests set status = 'approved', decided_by = $2, decided_at = $3, decision_note = $4, expires_at = $5 where id = $1`,
			requestID, deciderID, now, note, expires); err != nil {
			return err
		}
		if extends == nil {
			return nil
		}
		if _, err := tx.EndAccessGrant(ctx, *extends, "extended", now); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		return nil
	})
	return final, err
}

func (s *Store) DenyAccessRequest(ctx context.Context, requestID string, stage int, deciderID, note string, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set status = 'denied', decided_by = $2, decided_at = $3, decision_note = $4 where id = $1 and status = 'pending' and stage = $5`,
		requestID, deciderID, now, note, stage)
}

func (s *Store) CancelAccessRequest(ctx context.Context, requestID, userID string, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set status = 'cancelled', decided_at = $3 where id = $1 and (requester_id = $2 or requested_by = $2) and status = 'pending'`,
		requestID, userID, now)
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

func (s *Store) AccessApproverContacts(ctx context.Context, requestID string, fallbackOnly bool) ([]ApproverContact, error) {
	pool, admins := currentApprovers, `union
			select u.id, u.email from user_roles ur join users u on u.id = ur.user_id join access_requests r on r.id = $1
			where ur.role = 'global_admin' and u.id <> r.requester_id and u.id is distinct from r.requested_by and u.status = 'active' and not exists (select 1 from approvers)`
	if fallbackOnly {
		pool, admins = stagePool(true), ""
	}
	rows, err := s.db.Query(ctx, `with approvers as (
			select x.id, x.email from access_requests r cross join lateral (`+pool+`) x where r.id = $1
		), recipients as (
			select id, email from approvers `+admins+`
		)
		select email, exists (select 1 from user_roles ur where ur.user_id = recipients.id and ur.scope is null) from recipients order by email`, requestID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApproverContact, error) {
		var c ApproverContact
		return c, row.Scan(&c.Email, &c.Admin)
	})
}

type StageTimeout struct {
	RequestID   string
	Stage       int
	Escalated   bool
	Escalate    bool
	TimeoutDays int
}

func (s *Store) TimedOutAccessRequests(ctx context.Context, now time.Time) ([]StageTimeout, error) {
	rows, err := s.db.Query(ctx, `select r.id, r.stage, r.escalated, not r.escalated and exists (`+stagePool(false)+`) and exists (`+stagePool(true)+`), st.timeout_days
		from access_requests r join access_package_stages st on st.package_id = r.package_id and st.stage = r.stage
		where r.status = 'pending' and st.timeout_days is not null and r.stage_started_at + st.timeout_days * interval '1 day' <= $1 order by r.created_at`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (StageTimeout, error) {
		var t StageTimeout
		return t, row.Scan(&t.RequestID, &t.Stage, &t.Escalated, &t.Escalate, &t.TimeoutDays)
	})
}

func (s *Store) EscalateAccessRequest(ctx context.Context, t StageTimeout, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set escalated = true, stage_started_at = $2 where id = $1 and status = 'pending' and stage = $3 and not escalated`, t.RequestID, now, t.Stage)
}

func (s *Store) ExpireAccessRequest(ctx context.Context, t StageTimeout, now time.Time) error {
	return s.closePendingRequest(ctx, `update access_requests set status = 'expired', decided_at = $2 where id = $1 and status = 'pending' and stage = $3 and escalated = $4`, t.RequestID, now, t.Stage, t.Escalated)
}

func (s *Store) DirectReports(ctx context.Context, managerID string) ([]NamedRef, error) {
	rows, err := s.db.Query(ctx, `select id, name, email from users where kind = 'person' and status in ('active', 'invited') and coalesce(manager_id, sponsor_id) = $1 order by lower(name), id`, managerID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (NamedRef, error) {
		var r NamedRef
		return r, row.Scan(&r.ID, &r.Name, &r.Email)
	})
}

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

var ApprovalActions = map[string]string{
	"global-admin":       "granting the global administrator role",
	"policy-disable":     "stopping an enforced access policy",
	"password-sign-in":   "turning on password sign-in",
	"identity-providers": "changing identity providers",
	"impersonation":      "impersonating someone",
	"user-deletion":      "deleting a person permanently",
}

type ApprovalRequest struct {
	ID          string     `json:"id"`
	Action      string     `json:"action"`
	Summary     string     `json:"summary"`
	TargetType  string     `json:"targetType"`
	TargetID    string     `json:"targetId"`
	TargetLabel string     `json:"targetLabel"`
	Requester   NamedRef   `json:"requester"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	Status      string     `json:"status"`
	DecidedBy   *NamedRef  `json:"decidedBy"`
	DecidedAt   *time.Time `json:"decidedAt"`
	Outcome     string     `json:"outcome"`
	CanDecide   bool       `json:"canDecide"`
	Method      string     `json:"-"`
	Path        string     `json:"-"`
	Body        []byte     `json:"-"`
	State       string     `json:"-"`
}

var approvalTargetLocks = map[string]struct{ tables, row string }{
	"user":              {"user_roles, role_eligibilities, role_activations", "users"},
	"service_account":   {"user_roles", "users"},
	"policy":            {"access_policies", ""},
	"identity_provider": {"identity_providers", ""},
	"method":            {"sign_in_method_settings", ""},
	"settings":          {"settings", ""},
	"configuration":     {"settings, user_attributes, groups, group_owners, group_members, terms, applications, app_groups, app_exchange_sources, access_policies, access_packages, access_package_groups, access_package_approvers", ""},
}

func (s *Store) LockApprovalTarget(ctx context.Context, targetType, targetID string) error {
	lock, ok := approvalTargetLocks[targetType]
	if !ok {
		return fmt.Errorf("no lock for approval target %q", targetType)
	}
	if _, err := s.db.Exec(ctx, `lock table `+lock.tables+` in share row exclusive mode`); err != nil || lock.row == "" {
		return err
	}
	_, err := s.db.Exec(ctx, `select 1 from `+lock.row+` where id = $1 for update`, targetID)
	return err
}

var approvalSelect = `select r.id, r.action, r.summary, r.target_type, r.target_id, r.target_label, json_build_object('id', u.id, 'name', u.name),
	r.created_at, r.expires_at, case when r.status = 'pending' and r.expires_at <= now() then 'expired' else r.status end, ` + govRef("d") + `,
	r.decided_at, r.outcome, r.method, r.path, r.body_sealed, r.state
	from approval_requests r join users u on u.id = r.requested_by left join users d on d.id = r.decided_by `

func (s *Store) approvals(ctx context.Context, clause string, args ...any) ([]ApprovalRequest, error) {
	rows, err := s.db.Query(ctx, approvalSelect+clause, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ApprovalRequest, error) {
		var r ApprovalRequest
		var sealed []byte
		if err := row.Scan(&r.ID, &r.Action, &r.Summary, &r.TargetType, &r.TargetID, &r.TargetLabel, &r.Requester, &r.CreatedAt, &r.ExpiresAt, &r.Status,
			&r.DecidedBy, &r.DecidedAt, &r.Outcome, &r.Method, &r.Path, &sealed, &r.State); err != nil {
			return r, err
		}
		var err error
		r.Body, err = s.Sealer.Open(sealed)
		return r, err
	})
}

func (s *Store) ListApprovalRequests(ctx context.Context) ([]ApprovalRequest, error) {
	return s.approvals(ctx, `order by r.created_at desc limit 500`)
}

func (s *Store) GetApprovalRequest(ctx context.Context, requestID string) (ApprovalRequest, error) {
	list, err := s.approvals(ctx, `where r.id = $1`, requestID)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if len(list) == 0 {
		return ApprovalRequest{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Store) CreateApprovalRequest(ctx context.Context, r ApprovalRequest) (string, error) {
	requestID := id.New("apr")
	_, err := s.db.Exec(ctx, `insert into approval_requests (id, action, summary, target_type, target_id, target_label, method, path, body_sealed, state, requested_by, expires_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		requestID, r.Action, r.Summary, r.TargetType, r.TargetID, r.TargetLabel, r.Method, r.Path, s.Sealer.Seal(r.Body), r.State, r.Requester.ID, r.ExpiresAt)
	return requestID, err
}

func (s *Store) DecideApprovalRequest(ctx context.Context, requestID, status, deciderID, outcome string) error {
	tag, err := s.db.Exec(ctx, `update approval_requests set status = $2, decided_by = $3, decided_at = now(), outcome = $4 where id = $1 and status = 'pending' and expires_at > now()`,
		requestID, status, deciderID, outcome)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return s.existsOr(ctx, `select exists (select 1 from approval_requests where id = $1)`, requestID)
}

func (s *Store) GlobalAdminEmails(ctx context.Context, exceptUserID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select u.email from users u join user_roles r on r.user_id = u.id
		where r.role = 'global_admin' and u.status = 'active' and u.kind = 'person' and u.id <> $1 order by u.email`, exceptUserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

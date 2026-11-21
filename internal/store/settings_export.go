package store

import (
	"context"
	"encoding/json"
	"strings"
)

type ExportSection struct {
	Name  string
	Table string
	Query string
}

var ExportSections = []ExportSection{
	{"users", "users", `select id, email, name, title, department, location, status, kind, manager_id, source, external_id, created_at, updated_at, last_sign_in_at, guest, sponsor_id, expires_at, attributes from users order by created_at, id`},
	{"attributes", "user_attributes", `select id, name, label, type, self_service, created_at, updated_at from user_attributes order by name`},
	{"groups", "groups", `select g.id, g.name, g.description, g.kind, g.rule, g.join_policy, g.source, g.external_id, g.created_at,
		array(select o.user_id from group_owners o where o.group_id = g.id order by o.user_id) as owner_ids from groups g order by g.name`},
	{"memberships", "group_members", `select group_id, user_id, created_at from group_members order by group_id, user_id`},
	{"roles", "user_roles", `select user_id, role, scope, created_at from user_roles order by user_id, role`},
	{"eligibleRoles", "role_eligibilities", `select user_id, role, scope, created_at from role_eligibilities order by user_id, role`},
	{"applications", "applications", `select a.id, a.name, a.description, a.protocol, a.app_type, a.status, a.client_id, a.homepage, a.redirect_uris, a.post_logout_uris, a.scopes,
		a.access_token_ttl, a.refresh_token_ttl, a.id_token_ttl, a.refresh_rotation, a.token_exchange, a.max_auth_age, a.setup_guide, a.owner_id, a.claim_mappings, a.created_at, a.updated_at,
		array(select g.group_id from app_groups g where g.app_id = a.id order by g.group_id) as group_ids,
		array(select x.resource_id from app_exchange_sources x where x.app_id = a.id order by x.resource_id) as token_exchange_from from applications a order by a.name`},
	{"applicationRoles", "app_roles", `select r.id, r.app_id, r.key, r.name, r.description, r.created_at,
		array(select g.group_id from app_role_groups g where g.role_id = r.id order by g.group_id) as group_ids,
		array(select u.user_id from app_role_users u where u.role_id = r.id order by u.user_id) as user_ids from app_roles r order by r.app_id, r.key`},
	{"policies", "access_policies", `select id, name, description, priority, enabled, mode, effect, terms_id, conditions, created_at, updated_at from access_policies order by priority`},
	{"policies", "access_policies", `select id, name, description, priority, enabled, mode, effect, terms_id, context_id, conditions, created_at, updated_at from access_policies order by priority`},
	{"authenticationContexts", "authentication_contexts", `select id, name, methods, max_age, created_at, updated_at from authentication_contexts order by id`},
	{"terms", "terms", `select id, name, version, text, url, created_at, updated_at from terms order by lower(name)`},
	{"termsAcceptances", "terms_acceptances", `select user_id, terms_id, name, version, accepted_at from terms_acceptances order by accepted_at, user_id, terms_id`},
	{"accessPackages", "access_packages", `select p.id, p.name, p.description, p.owner_id, p.max_days, p.require_justification, p.archived, p.created_at, p.updated_at,
		array(select g.group_id from access_package_groups g where g.package_id = p.id order by g.group_id) as group_ids,
		array(select a.user_id from access_package_approvers a where a.package_id = p.id order by a.user_id) as approver_ids from access_packages p order by p.name`},
	{"accessReviews", "access_reviews", `select r.id, r.name, r.group_id, r.due_at, r.auto_apply, r.status, r.created_by, r.created_at, r.completed_by, r.completed_at,
		array(select v.user_id from access_review_reviewers v where v.review_id = r.id order by v.user_id) as reviewer_ids,
		(select coalesce(json_agg(json_build_object('userId', i.user_id, 'decision', i.decision, 'note', i.note, 'decidedBy', i.decided_by, 'decidedAt', i.decided_at, 'outcome', i.outcome) order by i.user_id), '[]')
			from access_review_items i where i.review_id = r.id) as items from access_reviews r order by r.created_at`},
	{"auditEvents", "audit_events", `select id, time, actor_id, action, summary, target_type, target_id, target_label, ip, changes from audit_events order by time, id`},
}

func (s *Store) ExportRows(ctx context.Context, section ExportSection, emit func(json.RawMessage) error, args ...any) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `select to_regclass($1) is not null`, section.Table).Scan(&exists); err != nil || !exists {
		return false, err
	}
	rows, err := s.db.Query(ctx, `select row_to_json(t) from (`+section.Query+`) t`, args...)
	if err != nil {
		return true, err
	}
	defer rows.Close()
	for rows.Next() {
		var row map[string]json.RawMessage
		if err := rows.Scan(&row); err != nil {
			return true, err
		}
		camel := make(map[string]json.RawMessage, len(row))
		for key, value := range row {
			camel[camelCase(key)] = value
		}
		encoded, err := json.Marshal(camel)
		if err != nil {
			return true, err
		}
		if err := emit(encoded); err != nil {
			return true, err
		}
	}
	return true, rows.Err()
}

func camelCase(key string) string {
	parts := strings.Split(key, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

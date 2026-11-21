package store

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

var PersonalDataSections = []ExportSection{
	{"groups", "group_members", `select g.id, g.name, m.created_at as added_at from group_members m join groups g on g.id = m.group_id where m.user_id = $1 order by g.name`},
	{"ownedGroups", "group_owners", `select g.id, g.name, o.created_at as added_at from group_owners o join groups g on g.id = o.group_id where o.user_id = $1 order by g.name`},
	{"groupJoinRequests", "group_join_requests", `select g.id, g.name, j.created_at from group_join_requests j join groups g on g.id = j.group_id where j.user_id = $1 order by j.created_at`},
	{"linkedAccounts", "federated_identities", `select i.id, p.name as provider, i.subject, i.email, i.created_at, i.last_used_at from federated_identities i join identity_providers p on p.id = i.provider_id where i.user_id = $1 order by i.created_at`},
	{"sessions", "sessions", `select id, method, ip, location, user_agent, client_id, second_factor, created_at, last_active_at, reauthenticated_at, expires_at, revoked_at from sessions where user_id = $1 order by created_at`},
	{"devices", "devices", `select id, user_agent, trust, last_ip, sign_in_count, first_seen_at, last_seen_at from devices where user_id = $1 order by first_seen_at`},
	{"signIns", "sign_in_events", `select id, time, email, app_id, result, method, ip, location, device, risk, reason, challenge, impersonator_id, hook from sign_in_events
		where user_id = $1 or lower(email) = (select lower(email) from users where id = $1) order by time`},
	{"auditEvents", "audit_events", `select id, time, actor_id, action, summary, changes from audit_events where target_id = $1 order by time, id`},
	{"applicationRoles", "app_role_users", `select r.app_id, a.name as application, r.key, r.name from app_role_users x join app_roles r on r.id = x.role_id join applications a on a.id = r.app_id where x.user_id = $1 order by a.name, r.key`},
	{"applicationRelationships", "authz_tuples", `select t.app_id, a.name as application, t.object_type, t.object_id, t.relation, t.created_at from authz_tuples t join applications a on a.id = t.app_id where t.user_id = $1 order by a.name, t.object_type, t.object_id, t.relation`},
	{"consents", "consent_grants", `select c.app_id, a.name as application, c.scopes, c.granted_at from consent_grants c join applications a on a.id = c.app_id where c.user_id = $1 order by c.granted_at`},
	{"tokens", "oidc_tokens", `select kind, client_id, scopes, audience, created_at, expires_at, revoked_at from oidc_tokens where user_id = $1 order by created_at`},
	{"accessRequests", "access_requests", `select r.id, r.package_id, p.name as package, r.justification, r.duration_days, r.status, r.decided_at, r.decision_note, r.expires_at, r.ended_at, r.end_reason, r.created_at
		from access_requests r join access_packages p on p.id = r.package_id where r.requester_id = $1 order by r.created_at`},
	{"accessReviews", "access_review_items", `select i.review_id, r.name as review, i.decision, i.note, i.decided_at, i.outcome from access_review_items i join access_reviews r on r.id = i.review_id where i.user_id = $1 order by r.created_at`},
	{"termsAcceptances", "terms_acceptances", `select terms_id, name, version, accepted_at from terms_acceptances where user_id = $1 order by accepted_at`},
	{"impersonations", "impersonations", `select id, actor_id, reason, created_at, expires_at, started_at from impersonations where user_id = $1 order by created_at`},
	{"provisionedAccounts", "app_provisioned_users", `select app_id, remote_id, active, synced_at from app_provisioned_users where user_id = $1 order by app_id`},
	{"lifecycleRuns", "lifecycle_runs", `select id, rule_id, trigger, change, steps, result, created_at from lifecycle_runs where user_id = $1 order by created_at`},
	{"sshCertificates", "ssh_certificates", `select serial, key_id, principals, fingerprint, key_type, ip, valid_after, valid_before, created_at from ssh_certificates where user_id = $1 order by created_at`},
	{"riskEvents", "risk_events", `select id, time, type, level, ip, detail, status, resolved_at from risk_events where user_id = $1 order by time`},
	{"applicationPreferences", "app_preferences", `select app_id, position, favourite from app_preferences where user_id = $1 order by position`},
	{"securityEmails", "security_email_sends", `select kind, subject, sent_at from security_email_sends where user_id = $1 order by sent_at`},
	{"securityEmailOptOuts", "security_email_opt_outs", `select kind from security_email_opt_outs where user_id = $1 order by kind`},
}

func (s *Store) ActiveProvisionedAccounts(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `select count(*) from app_provisioned_users where user_id = $1 and active`, userID).Scan(&n)
	return n, err
}

func (s *Store) ErasePerson(ctx context.Context, u User) (string, error) {
	pseudonym := id.New("deleted")
	terms := []string{regexp.QuoteMeta(u.ID), `(?:(?<=\\u003c)|(?<![[:alnum:]_.%+-]))` + regexp.QuoteMeta(u.Email) + `(?![[:alnum:]_-]|\.[[:alnum:]])`}
	if u.Name != "" && !json.Valid([]byte(u.Name)) {
		terms = append(terms, `(?<![[:alnum:]_])`+regexp.QuoteMeta(u.Name)+`(?![[:alnum:]_])`)
	}
	mention := strings.Join(terms, "|")
	args := pgx.NamedArgs{"id": u.ID, "mention": mention, "pseudonym": pseudonym, "email": u.Email}
	for _, sql := range []string{
		`update audit_events set actor_id = case when actor_id = @id then @pseudonym else actor_id end,
			target_id = case when target_id = @id then @pseudonym else target_id end,
			target_label = case when target_id = @id then @pseudonym else regexp_replace(target_label, @mention, @pseudonym, 'gi') end,
			summary = regexp_replace(summary, @mention, @pseudonym, 'gi'),
			changes = case when target_id = @id then (select coalesce(jsonb_agg(jsonb_build_object('field', c->'field', 'redacted', true)), '[]') from jsonb_array_elements(changes) c)
				else regexp_replace(changes::text, @mention, @pseudonym, 'gi')::jsonb end
			where actor_id = @id or target_id = @id or target_label ~* @mention or summary ~* @mention or changes::text ~* @mention`,
		`update sign_in_events set email = case when user_id = @id or lower(email) = lower(@email) then @pseudonym else email end,
			impersonator_id = case when impersonator_id = @id then @pseudonym else impersonator_id end,
			reason = regexp_replace(reason, @mention, @pseudonym, 'gi')
			where user_id = @id or lower(email) = lower(@email) or impersonator_id = @id or reason ~* @mention`,
		`update approval_requests set target_id = case when target_id = @id then '' else target_id end,
			target_label = case when target_id = @id then @pseudonym else regexp_replace(target_label, @mention, @pseudonym, 'gi') end,
			state = case when target_id = @id then '' else regexp_replace(state, @mention, @pseudonym, 'gi') end,
			path = replace(path, @id, @pseudonym), summary = regexp_replace(summary, @mention, @pseudonym, 'gi')
			where target_id = @id or strpos(path, @id) > 0 or target_label ~* @mention or state ~* @mention or summary ~* @mention`,
		`update webhook_deliveries set body = regexp_replace(body, @mention, @pseudonym, 'gi') where body ~* @mention and (app_id is null or status <> 'pending')`,
		`update ssh_certificates set key_id = replace(key_id, @id, @pseudonym) where user_id = @id`,
		`update access_policies set conditions = conditions || jsonb_build_object('userIds', coalesce(conditions->'userIds', '[]') - @id::text, 'excludeUserIds', coalesce(conditions->'excludeUserIds', '[]') - @id::text)
			where conditions->'userIds' ? @id::text or conditions->'excludeUserIds' ? @id::text`,
		`delete from sign_ups where user_id = @id or lower(email) = lower(@email)`,
		`delete from mail_outbox where lower(recipient) = lower(@email) or subject ~* @mention`,
		`delete from rate_limit_windows where subject = lower(@email)`,
		`delete from oidc_auth_requests where lower(login_hint) = lower(@email)`,
		`delete from risk_events where user_id = @id`,
	} {
		if _, err := s.db.Exec(ctx, sql, args); err != nil {
			return "", err
		}
	}
	if err := s.queueLogouts(ctx, `s.user_id = $1 and s.revoked_at is null and s.expires_at > now()`, u.ID); err != nil {
		return "", err
	}
	_, err := s.db.Exec(ctx, `delete from users where id = $1`, u.ID)
	return pseudonym, err
}

package store

import (
	"context"
	"net/netip"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

func (s *Store) SeedPolicies(ctx context.Context) error {
	tor, err := s.SaveNetworkZone(ctx, NetworkZone{Name: "Tor exit nodes (sample)", Kind: "risky", CIDRs: []string{"185.220.101.0/24"}})
	if err != nil {
		return err
	}
	if _, err := s.SaveNetworkZone(ctx, NetworkZone{Name: "Office", Kind: "trusted", CIDRs: []string{"145.53.112.0/24"}}); err != nil {
		return err
	}
	var admins string
	if err := s.db.QueryRow(ctx, `select id from groups where name = 'Halo administrators'`).Scan(&admins); err != nil {
		return notFound(err)
	}
	rows, err := s.db.Query(ctx, `select id from applications where name = 'AWS IAM Identity Center' or name like 'Kubernetes%' order by name`)
	if err != nil {
		return err
	}
	production := []string{}
	for rows.Next() {
		var appID string
		if err := rows.Scan(&appID); err != nil {
			rows.Close()
			return err
		}
		production = append(production, appID)
	}
	rows.Close()

	ids := map[string]string{}
	for _, p := range []AccessPolicy{
		{Name: "Block high-risk sign-ins", Description: "Stops sign-ins Halo rates high risk, such as attempts from Tor exit nodes or after a burst of failed sign-ins.",
			Enabled: true, Mode: "enforce", Effect: "block", Conditions: PolicyConditions{AllUsers: true, AllApps: true, Risk: []string{"high"}}},
		{Name: "Phishing-resistant MFA for administrators", Description: "Administrators sign in with a passkey or security key. Report-only until every administrator has enrolled one.",
			Enabled: true, Mode: "report", Effect: "require-phishing-resistant", Conditions: PolicyConditions{GroupIDs: []string{admins}, AllApps: true}},
		{Name: "Require trusted device for production", Description: "AWS and the production cluster only accept sign-ins from devices an administrator has marked as trusted.",
			Enabled: true, Mode: "report", Effect: "block", Conditions: PolicyConditions{AllUsers: true, AppIDs: production, Device: "untrusted"}},
	} {
		created, err := s.CreateAccessPolicy(ctx, p)
		if err != nil {
			return err
		}
		ids[p.Name] = created.ID
	}

	type seen struct {
		userID, agent, ip string
		firstSeen         time.Time
		lastSeen          time.Time
		admin             bool
	}
	var devices []seen
	rows, err = s.db.Query(ctx, `select s.user_id, s.user_agent, (array_agg(s.ip order by s.created_at desc))[1], min(s.created_at), max(s.last_active_at),
		exists (select 1 from user_roles r where r.user_id = s.user_id)
		from sessions s where s.revoked_at is null group by s.user_id, s.user_agent`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d seen
		if err := rows.Scan(&d.userID, &d.agent, &d.ip, &d.firstSeen, &d.lastSeen, &d.admin); err != nil {
			rows.Close()
			return err
		}
		devices = append(devices, d)
	}
	rows.Close()
	for _, d := range devices {
		_, os, browser := ParseUserAgent(d.agent)
		var count int
		var first *time.Time
		if err := s.db.QueryRow(ctx, `select count(*), min(time) from sign_in_events where user_id = $1 and result = 'success' and device = $2`, d.userID, browser+" · "+os).Scan(&count, &first); err != nil {
			return err
		}
		if first != nil && first.Before(d.firstSeen) {
			d.firstSeen = *first
		}
		trust := "unknown"
		if d.admin {
			trust = "trusted"
		}
		if _, err := s.db.Exec(ctx, `insert into devices (id, user_id, token_hash, user_agent, trust, first_seen_at, last_seen_at, last_ip, sign_in_count) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id.New("dev"), d.userID, secret.Hash(secret.Token(32)), d.agent, trust, d.firstSeen, d.lastSeen, d.ip, max(count, 1)); err != nil {
			return err
		}
	}

	type risky struct {
		signInID, ip string
		userID       *string
		at           time.Time
	}
	var events []risky
	rows, err = s.db.Query(ctx, `select id, user_id, ip, time from sign_in_events where risk = 'high'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e risky
		if err := rows.Scan(&e.signInID, &e.userID, &e.ip, &e.at); err != nil {
			rows.Close()
			return err
		}
		events = append(events, e)
	}
	rows.Close()
	torRange := netip.MustParsePrefix(tor.CIDRs[0])
	for _, e := range events {
		if addr, err := netip.ParseAddr(e.ip); err != nil || !torRange.Contains(addr) {
			continue
		}
		if err := s.RecordRiskEvent(ctx, RiskEvent{Time: e.at, Type: "risky-network", Level: "high", UserID: e.userID, IP: e.ip, SignInID: &e.signInID, Detail: e.ip + " is in the risky network “" + tor.Name + "”."}); err != nil {
			return err
		}
	}

	_, err = s.db.Exec(ctx, `insert into policy_evaluations (sign_in_id, time, risk, effect, matches)
		select e.id, e.time, e.risk, 'allow', jsonb_build_array(jsonb_build_object('policyId', $1::text, 'mode', 'report',
			'outcome', case when e.method in ('passkey', 'security-key') then 'allow' else 'require' end))
		from sign_in_events e where e.result = 'success' and e.time > now() - interval '24 hours'
		and exists (select 1 from group_members m where m.user_id = e.user_id and m.group_id = $2)`, ids["Phishing-resistant MFA for administrators"], admins)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `insert into policy_evaluations (sign_in_id, time, risk, effect, matches)
		select e.id, e.time, e.risk, 'allow', jsonb_build_array(jsonb_build_object('policyId', $1::text, 'mode', 'report',
			'outcome', case when exists (select 1 from user_roles r where r.user_id = e.user_id) then 'allow' else 'block' end))
		from sign_in_events e where e.result = 'success' and e.time > now() - interval '24 hours' and e.app_id = any($2)
		on conflict (sign_in_id) do update set matches = policy_evaluations.matches || excluded.matches`, ids["Require trusted device for production"], production)
	return err
}

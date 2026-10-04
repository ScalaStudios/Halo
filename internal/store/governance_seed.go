package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

func (s *Store) SeedGovernance(ctx context.Context) error {
	return s.Tx(ctx, func(tx *Store) error { return tx.seedGovernance(ctx, time.Now()) })
}

func (s *Store) seedGovernance(ctx context.Context, now time.Time) error {
	ago := func(minutes int) time.Time { return now.Add(-time.Duration(minutes) * time.Minute) }
	days := func(n int) *int { return &n }
	rows, err := s.db.Query(ctx, `select email, id from users union all select name, id from groups`)
	if err != nil {
		return err
	}
	pairs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Key, ID string }])
	if err != nil {
		return err
	}
	ids := map[string]string{}
	for _, p := range pairs {
		ids[p.Key] = p.ID
	}
	if ids["luna@example.com"] == "" || ids["Production access"] == "" {
		return errors.New("load the demo directory before seeding governance data")
	}
	luna, priya, grace, marcus := ids["luna@example.com"], ids["priya.raman@example.com"], ids["grace.liu@example.com"], ids["marcus.okafor@example.com"]
	prod, grafana, k8s, forgejo := ids["Production access"], ids["Grafana editors"], ids["Kubernetes cluster admins"], ids["Forgejo maintainers"]

	if _, err := s.db.Exec(ctx, `insert into group_members (group_id, user_id, created_at)
		select $1, u.id, $2 from users u where u.email = 'daniel.mensah@example.com' or u.id in (
			select id from users where department in ('Infrastructure', 'Engineering') and status = 'active'
			and email not in ('luna@example.com', 'grace.liu@example.com', 'arjun.mehta@example.com') order by email limit 6)
		on conflict do nothing`, prod, ago(120*demoDay)); err != nil {
		return err
	}

	packages := map[string]string{}
	for _, p := range []struct {
		key string
		in  AccessPackageInput
	}{
		{"prod", AccessPackageInput{Name: "Production access", Description: "Shell and database access to production through the bastion. Granted for on-call work and incidents.",
			OwnerID: &priya, GroupIDs: []string{prod}, ApproverIDs: []string{priya, luna}, MaxDays: days(7), RequireJustification: true}},
		{"grafana", AccessPackageInput{Name: "Grafana editors", Description: "Create and edit dashboards and alert rules in Grafana.",
			OwnerID: &luna, GroupIDs: []string{grafana}, ApproverIDs: []string{luna}, MaxDays: days(90), RequireJustification: true}},
		{"k8s", AccessPackageInput{Name: "Kubernetes cluster admins", Description: "cluster-admin on prod-eu-1 for upgrades and incident response. Expires after 48 hours at most.",
			OwnerID: &luna, GroupIDs: []string{k8s}, ApproverIDs: []string{luna, priya}, MaxDays: days(2), RequireJustification: true}},
		{"forgejo", AccessPackageInput{Name: "Forgejo maintainers", Description: "Administer repositories, branch protection and CI settings on git.example.com.",
			OwnerID: &grace, GroupIDs: []string{forgejo}, ApproverIDs: []string{grace, luna}, RequireJustification: true}},
	} {
		if packages[p.key], err = s.SaveAccessPackage(ctx, "", p.in); err != nil {
			return err
		}
	}

	request := func(pkg, email string, duration *int, justification string, minutes int) (string, error) {
		requestID := id.New("req")
		_, err := s.db.Exec(ctx, `insert into access_requests (id, package_id, requester_id, justification, duration_days, created_at) values ($1, $2, $3, $4, $5, $6)`,
			requestID, packages[pkg], ids[email], justification, duration, ago(minutes))
		return requestID, err
	}
	for _, r := range []struct {
		pkg, email    string
		duration      *int
		justification string
		minutes       int
	}{
		{"prod", "grace.liu@example.com", days(7), "On-call shadowing for the week of 6 October.", 40},
		{"forgejo", "arjun.mehta@example.com", nil, "Need to manage branch protection for the web repo.", 60 * 4},
		{"grafana", "hana.kobayashi@example.com", days(30), "Building the support load dashboard.", 60 * 22},
	} {
		if _, err := request(r.pkg, r.email, r.duration, r.justification, r.minutes); err != nil {
			return err
		}
	}
	for _, d := range []struct {
		pkg, email, justification, decider, note string
		minutes                                  int
	}{
		{"k8s", "tomasz.nowak@example.com", "Need to check node costs for the October budget review.", priya, "Cluster admin is for upgrades and incidents. The cost dashboards in Grafana have what you need.", 60 * 27},
		{"prod", "elena.vasquez@example.com", "Investigating a customer data export ticket.", luna, "Support doesn't need shell access. Ask the on-call engineer to run the export.", 60 * 75},
	} {
		requestID, err := request(d.pkg, d.email, days(2), d.justification, d.minutes+90)
		if err != nil {
			return err
		}
		if err := s.DenyAccessRequest(ctx, requestID, d.decider, d.note, ago(d.minutes)); err != nil {
			return err
		}
	}
	var sre string
	err = s.db.QueryRow(ctx, `select u.email from users u where u.department = 'Infrastructure' and u.status = 'active' and u.email <> 'luna@example.com'
		and not exists (select 1 from group_members m where m.group_id = $1 and m.user_id = u.id) order by u.email limit 1`, k8s).Scan(&sre)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if sre != "" {
		requestID, err := request("k8s", sre, days(2), "Upgrading ingress-nginx on prod-eu-1 during the Thursday maintenance window.", 60*21)
		if err != nil {
			return err
		}
		if err := s.ApproveAccessRequest(ctx, requestID, luna, "", ago(60*20)); err != nil {
			return err
		}
	}

	type decision struct{ user, decider, choice, note string }
	reviews := []struct {
		name, group     string
		reviewers       []string
		due, created    int
		autoApply       bool
		status, creator string
		decisions       func([]string) []decision
	}{
		{"Production access · Q3 2026", prod, []string{priya, luna}, 3 * demoDay, 25 * demoDay, true, "overdue", priya, func(members []string) []decision {
			out := []decision{
				{luna, priya, "keep", "Platform lead and on the on-call rotation."},
				{priya, luna, "keep", "Security on-call."},
				{ids["daniel.mensah@example.com"], priya, "remove", "Suspended since offboarding ticket OPS-2291."},
			}
			for _, member := range members {
				if len(out) == 5 {
					break
				}
				if member != luna && member != priya && member != ids["daniel.mensah@example.com"] {
					out = append(out, decision{member, priya, "keep", "Still on the production on-call rotation."})
				}
			}
			return out
		}},
		{"Contractor access · monthly", ids["Contractors"], []string{marcus, luna}, -4 * demoDay, 3 * demoDay, false, "in-progress", marcus, func(members []string) []decision {
			var out []decision
			for _, member := range members[:min(2, len(members))] {
				out = append(out, decision{member, marcus, "keep", "Contract runs until December."})
			}
			return out
		}},
	}
	for _, v := range reviews {
		reviewID, err := s.CreateAccessReview(ctx, NewAccessReview{Name: v.name, GroupID: v.group, ReviewerIDs: v.reviewers, DueAt: ago(v.due), AutoApply: v.autoApply, CreatedBy: v.creator})
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `update access_reviews set status = $2, created_at = $3 where id = $1`, reviewID, v.status, ago(v.created)); err != nil {
			return err
		}
		rows, err := s.db.Query(ctx, `select i.user_id from access_review_items i join users u on u.id = i.user_id where i.review_id = $1 order by lower(u.name)`, reviewID)
		if err != nil {
			return err
		}
		members, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for i, d := range v.decisions(members) {
			if err := s.DecideReviewItem(ctx, reviewID, d.user, d.choice, d.note, d.decider, ago(v.created-demoDay-i*60)); err != nil {
				return err
			}
		}
	}

	engineering := `user.department == "Engineering"`
	var leaver string
	for i, in := range []LifecycleRuleInput{
		{Name: "Engineering joiners can edit dashboards", Trigger: "joiner", Condition: &engineering, Actions: []LifecycleAction{{Type: "add-to-group", GroupID: grafana}}, Enabled: true},
		{Name: "Leavers lose every session", Trigger: "leaver", Actions: []LifecycleAction{{Type: "revoke-sessions"}}, Enabled: true},
	} {
		ruleID, err := s.SaveLifecycleRule(ctx, "", in)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `update lifecycle_rules set created_at = $2, updated_at = $2 where id = $1`, ruleID, ago(60*demoDay-i)); err != nil {
			return err
		}
		leaver = ruleID
	}
	if err := s.RecordLifecycleRun(ctx, LifecycleRun{Rule: NamedRef{ID: leaver}, User: NamedRef{ID: ids["daniel.mensah@example.com"]}, Trigger: "leaver",
		Change: "Status changed from active to suspended", Steps: []string{"Ended 1 session"}, Result: "succeeded", CreatedAt: ago(60 * 9)}); err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `insert into lifecycle_snapshots (user_id, department, title, location, status) select id, department, title, location, status from users
		on conflict (user_id) do update set department = excluded.department, title = excluded.title, location = excluded.location, status = excluded.status`)
	return err
}

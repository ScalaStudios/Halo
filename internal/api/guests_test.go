package api_test

import (
	"context"
	"testing"
	"time"

	"halo/internal/store"
)

func TestInviteGuestNeedsAnActiveInternalSponsorAndNoRoles(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "gia@example.com", "global_admin")
	sponsor, _ := person(t, st, "sam@example.com")
	gone, _ := person(t, st, "gone@example.com")
	if err := st.SetUserStatus(ctx, gone.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(30 * 24 * time.Hour)
	invite := func(extra map[string]any) map[string]any {
		body := map[string]any{"email": "pat@partner.example", "name": "Pat", "guest": true, "sponsorId": sponsor.ID, "expiresAt": until}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"sponsorId": ""})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"sponsorId": gone.ID})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"expiresAt": nil})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"expiresAt": time.Now().Add(-time.Hour)})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"expiresAt": time.Now().AddDate(0, 0, 400)})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", invite(map[string]any{"roles": []string{"auditor"}})), 422)
	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "kim@example.com", "name": "Kim", "sponsorId": sponsor.ID}), 422)

	rec := call(t, h, admin, "POST", "/api/v1/users", invite(nil))
	expect(t, rec, 201)
	guest := decode[struct{ User store.User }](t, rec).User
	if !guest.Guest || guest.Sponsor == nil || guest.Sponsor.ID != sponsor.ID || guest.ExpiresAt == nil || guest.Status != "invited" {
		t.Fatalf("guest: %+v", guest)
	}

	expect(t, call(t, h, admin, "POST", "/api/v1/users", map[string]any{"email": "lee@partner.example", "name": "Lee", "guest": true, "sponsorId": guest.ID, "expiresAt": until}), 422)
	expect(t, call(t, h, admin, "PUT", "/api/v1/users/"+guest.ID+"/roles", map[string]any{"roles": []string{"auditor"}}), 422)
	expect(t, call(t, h, admin, "PUT", "/api/v1/users/"+guest.ID+"/eligible-roles", map[string]any{"roles": []string{"security_admin"}}), 422)
	if err := st.SetUserRoles(ctx, guest.ID, []string{"auditor"}, nil); err == nil {
		t.Fatal("the database let a guest hold a role")
	}
}

func TestSponsorExtendsGuestUntilItExpires(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	sponsor, sponsorToken := person(t, st, "sam@example.com")
	other, otherToken := person(t, st, "olu@example.com")
	soon := time.Now().Add(time.Hour)
	guest, err := st.CreateUser(ctx, store.NewUser{Email: "pat@partner.example", Name: "Pat", Status: "active", Guest: true, SponsorID: &sponsor.ID, ExpiresAt: &soon})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/users/" + guest.ID + "/guest"
	later := time.Now().Add(60 * 24 * time.Hour).Truncate(time.Second)

	expect(t, call(t, h, otherToken, "PUT", path, map[string]any{"expiresAt": later}), 404)
	expect(t, call(t, h, sponsorToken, "PUT", path, map[string]any{"sponsorId": other.ID}), 403)
	rec := call(t, h, sponsorToken, "PUT", path, map[string]any{"expiresAt": later})
	expect(t, rec, 200)
	if got := decode[store.User](t, rec); got.ExpiresAt == nil || !got.ExpiresAt.Equal(later) {
		t.Fatalf("not extended: %+v", got)
	}
	mine := decode[[]store.User](t, call(t, h, sponsorToken, "GET", "/api/v1/me/guests", nil))
	if len(mine) != 1 || mine[0].ID != guest.ID {
		t.Fatalf("sponsored guests: %+v", mine)
	}
	if len(decode[[]store.User](t, call(t, h, otherToken, "GET", "/api/v1/me/guests", nil))) != 0 {
		t.Fatal("someone else saw the guest")
	}

	if err := st.SetGuestAccess(ctx, guest.ID, sponsor.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if u := decode[store.User](t, call(t, h, admin, "GET", "/api/v1/users/"+guest.ID, nil)); u.Status != "suspended" {
		t.Fatalf("expired guest shows as %s", u.Status)
	}
	if expired, err := st.ClaimExpiredGuests(ctx, time.Now()); err != nil || len(expired) != 1 {
		t.Fatalf("claim: %v %v", expired, err)
	}
	expect(t, call(t, h, sponsorToken, "PUT", path, map[string]any{"expiresAt": later}), 409)
	expect(t, call(t, h, admin, "POST", "/api/v1/users/"+guest.ID+"/restore", nil), 409)
	expect(t, call(t, h, admin, "PUT", path, map[string]any{"expiresAt": later, "sponsorId": other.ID}), 200)
	expect(t, call(t, h, admin, "POST", "/api/v1/users/"+guest.ID+"/restore", nil), 200)

	events, err := st.ListAudit(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	extended, changed := 0, 0
	for _, e := range events {
		switch e.TargetID + " " + e.Action {
		case guest.ID + " guest.extend":
			extended++
		case guest.ID + " guest.sponsor.change":
			changed++
		}
	}
	if extended != 2 || changed != 1 {
		t.Fatalf("audit: %d extensions, %d sponsor changes", extended, changed)
	}
}

func TestGuestsJoinOnlyChosenGroups(t *testing.T) {
	st, h := setup(t)
	ctx := context.Background()
	_, admin := person(t, st, "uma@example.com", "user_admin")
	sponsor, _ := person(t, st, "sam@example.com")
	until := time.Now().Add(24 * time.Hour)
	guest, err := st.CreateUser(ctx, store.NewUser{Email: "pat@partner.example", Name: "Pat", Department: "Engineering", Status: "active", Guest: true, SponsorID: &sponsor.ID, ExpiresAt: &until})
	if err != nil {
		t.Fatal(err)
	}
	guestToken := session(t, st, guest.ID)
	if _, err := st.CreateGroup(ctx, store.NewGroup{Name: "Engineering", Kind: "dynamic", Rule: new(`user.department == "Engineering"`)}); err != nil {
		t.Fatal(err)
	}
	open, err := st.CreateGroup(ctx, store.NewGroup{Name: "Social", Kind: "assigned"})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, h, admin, "PATCH", "/api/v1/groups/"+open.ID, map[string]any{"joinPolicy": "open"}), 200)

	if u, _ := st.GetUser(ctx, guest.ID); len(u.GroupIDs) != 0 {
		t.Fatalf("guest joined a rule-based group: %v", u.GroupIDs)
	}
	expect(t, call(t, h, guestToken, "POST", "/api/v1/me/groups/"+open.ID+"/join", nil), 403)
	if joinable := decode[[]store.Group](t, call(t, h, guestToken, "GET", "/api/v1/me/joinable-groups", nil)); len(joinable) != 0 {
		t.Fatalf("guest can join: %+v", joinable)
	}
	expect(t, call(t, h, admin, "POST", "/api/v1/groups/"+open.ID+"/members", map[string]any{"userId": guest.ID}), 204)
}

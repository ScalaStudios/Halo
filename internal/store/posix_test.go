package store_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"halo/internal/store"
	"halo/internal/testdb"
)

func TestPOSIXAllocation(t *testing.T) {
	st := testdb.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	users := make([]store.User, 24)
	groups := make([]store.Group, 8)
	errs := make(chan error, len(users)+len(groups))
	for i := range users {
		wg.Go(func() {
			u, err := st.CreateUser(ctx, store.NewUser{Email: fmt.Sprintf("sam@team%d.example.com", i), Name: "Sam", Status: "active"})
			users[i] = u
			errs <- err
		})
	}
	for i := range groups {
		wg.Go(func() {
			g, err := st.CreateGroup(ctx, store.NewGroup{Name: fmt.Sprintf("Ops %d", i), Kind: "assigned"})
			groups[i] = g
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		must(err)
	}
	settings, err := st.GetPOSIXSettings(ctx)
	must(err)
	names, numbers := map[string]bool{}, map[int]bool{}
	for _, u := range users {
		p := u.POSIX
		if p == nil || p.UIDNumber < settings.UIDMin || p.UIDNumber > settings.UIDMax || p.GIDNumber != p.UIDNumber || p.HomeDirectory != "/home/"+p.Username || p.LoginShell != "/bin/bash" {
			t.Fatalf("person %s got %+v", u.Email, p)
		}
		if names[p.Username] || numbers[p.UIDNumber] {
			t.Fatalf("%s or %d was handed out twice", p.Username, p.UIDNumber)
		}
		names[p.Username], numbers[p.UIDNumber] = true, true
	}
	if !names["sam"] || !names["sam2"] || !names["sam24"] {
		t.Fatalf("want sam, sam2 … sam24, got %v", names)
	}
	for _, g := range groups {
		got, err := st.GetGroup(ctx, g.ID)
		must(err)
		if got.POSIX == nil || got.POSIX.GIDNumber < settings.GIDMin || got.POSIX.GIDNumber > settings.GIDMax || numbers[got.POSIX.GIDNumber] || names[got.POSIX.Name] {
			t.Fatalf("group %s got %+v", g.Name, got.POSIX)
		}
		names[got.POSIX.Name], numbers[got.POSIX.GIDNumber] = true, true
	}

	root, err := st.CreateUser(ctx, store.NewUser{Email: "root@example.com", Name: "Root", Status: "active"})
	must(err)
	interns, err := st.CreateGroup(ctx, store.NewGroup{Name: "2024 Interns!", Kind: "assigned"})
	must(err)
	interns, err = st.GetGroup(ctx, interns.ID)
	must(err)
	if root.POSIX.Username != "root2" || interns.POSIX.Name != "g2024-interns" {
		t.Fatalf("got %q and %q", root.POSIX.Username, interns.POSIX.Name)
	}

	if err := st.SetPOSIXAccount(ctx, root.ID, store.POSIXAccount{Username: "daemon", LoginShell: "/bin/sh", HomeDirectory: "/home/r"}); !errors.Is(err, store.ErrPOSIXName) {
		t.Fatalf("renaming to a system account: %v", err)
	}
	if err := st.SetPOSIXAccount(ctx, root.ID, store.POSIXAccount{Username: "sam", LoginShell: "/bin/sh", HomeDirectory: "/home/r"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("renaming to a taken name: %v", err)
	}
	if err := st.SetPOSIXAccount(ctx, root.ID, store.POSIXAccount{Username: "g2024-interns", LoginShell: "/bin/sh", HomeDirectory: "/home/r"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("renaming to a group's name: %v", err)
	}
	must(st.SetPOSIXAccount(ctx, root.ID, store.POSIXAccount{Username: "rosa", LoginShell: "/bin/zsh", HomeDirectory: "/srv/rosa"}))
	renamed, err := st.GetUser(ctx, root.ID)
	must(err)
	if *renamed.POSIX != (store.POSIXAccount{Username: "rosa", UIDNumber: root.POSIX.UIDNumber, GIDNumber: root.POSIX.UIDNumber, LoginShell: "/bin/zsh", HomeDirectory: "/srv/rosa"}) {
		t.Fatalf("rename kept the number? %+v", renamed.POSIX)
	}

	if _, err := st.ErasePerson(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	next, err := st.CreateUser(ctx, store.NewUser{Email: "rosa@example.com", Name: "Rosa", Status: "active"})
	must(err)
	if next.POSIX.Username != "rosa" || next.POSIX.UIDNumber <= root.POSIX.UIDNumber {
		t.Fatalf("a deleted person's number came back: %+v after %+v", next.POSIX, root.POSIX)
	}

	must(st.SavePOSIXSettings(ctx, store.POSIXSettings{UIDMin: 70000, UIDMax: 70001, GIDMin: 80000, GIDMax: 89999, LoginShell: "/bin/sh"}))
	for i, want := range []int{70000, 70001} {
		u, err := st.CreateUser(ctx, store.NewUser{Email: fmt.Sprintf("new%d@example.com", i), Name: "New", Status: "active"})
		must(err)
		if u.POSIX.UIDNumber != want || u.POSIX.LoginShell != "/bin/sh" {
			t.Fatalf("want %d with /bin/sh, got %+v", want, u.POSIX)
		}
	}
	full, err := st.CreateUser(ctx, store.NewUser{Email: "full@example.com", Name: "Full", Status: "active"})
	must(err)
	if full.POSIX != nil {
		t.Fatalf("allocated past the end of the range: %+v", full.POSIX)
	}
	if err := st.SavePOSIXSettings(ctx, store.POSIXSettings{UIDMin: 70000, UIDMax: 90000, GIDMin: 80000, GIDMax: 89999, LoginShell: "/bin/sh"}); err == nil {
		t.Fatal("saved overlapping ranges")
	}
	must(st.SavePOSIXSettings(ctx, store.POSIXSettings{UIDMin: 70000, UIDMax: 70009, GIDMin: 80000, GIDMax: 89999, LoginShell: "/bin/sh"}))
	full, err = st.GetUser(ctx, full.ID)
	must(err)
	if full.POSIX == nil || full.POSIX.UIDNumber != 70002 {
		t.Fatalf("widening the range didn't allocate the waiting person: %+v", full.POSIX)
	}

	service, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "Deploy bot"})
	must(err)
	bot, err := st.GetUser(ctx, service.ID)
	must(err)
	if bot.POSIX != nil {
		t.Fatalf("service account got a Linux account: %+v", bot.POSIX)
	}
	all, err := st.ListUsers(ctx)
	must(err)
	if slices.ContainsFunc(all, func(u store.User) bool { return u.POSIX == nil }) {
		t.Fatal("a person has no POSIX account")
	}
}

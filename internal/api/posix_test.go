package api_test

import (
	"reflect"
	"testing"

	"halo/internal/store"
)

func TestSetPOSIXAccount(t *testing.T) {
	st, h := setup(t)
	_, userAdmin := person(t, st, "uma@example.com", "user_admin")
	_, helpdesk := person(t, st, "hal@example.com", "helpdesk_admin")
	sam, _ := person(t, st, "sam@example.com")
	person(t, st, "lee@example.com")
	path := "/api/v1/users/" + sam.ID + "/posix"

	expect(t, call(t, h, helpdesk, "PUT", path, map[string]any{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home/sam"}), 403)
	for _, bad := range []map[string]any{
		{"username": "Sam Lee", "loginShell": "/bin/bash", "homeDirectory": "/home/sam"},
		{"username": "sam", "loginShell": "zsh", "homeDirectory": "/home/sam"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home/sam:/bin/sh"},
		{"username": "sam", "loginShell": "/bin/zsh\nroot", "homeDirectory": "/home/sam"},
		{"username": "root", "loginShell": "/bin/zsh", "homeDirectory": "/home/sam"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/etc/systemd/system/x.service.d"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home/../etc/x"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home/sam/"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home"},
		{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/srv/samuel"},
	} {
		expect(t, call(t, h, userAdmin, "PUT", path, bad), 422)
	}
	expect(t, call(t, h, userAdmin, "PUT", path, map[string]any{"username": "lee", "loginShell": "/bin/zsh", "homeDirectory": "/home/sam"}), 409)
	expect(t, call(t, h, userAdmin, "PUT", path, map[string]any{"username": "sam", "loginShell": "/bin/zsh", "homeDirectory": "/home/lee"}), 409)
	rec := call(t, h, userAdmin, "PUT", path, map[string]any{"username": " samuel ", "loginShell": "/bin/zsh", "homeDirectory": "/home/samuel"})
	expect(t, rec, 200)
	got := decode[store.User](t, rec)
	if got.POSIX.Username != "samuel" || got.POSIX.UIDNumber != sam.POSIX.UIDNumber || got.POSIX.LoginShell != "/bin/zsh" || got.POSIX.HomeDirectory != "/home/samuel" {
		t.Fatalf("posix: %+v", got.POSIX)
	}
	if e := lastAudit(t, st); e.Action != "user.posix" || e.TargetID != sam.ID {
		t.Fatalf("audit: %+v", e)
	} else if got := diff(e); !reflect.DeepEqual(got, map[string]string{"username": `"sam" → "samuel"`, "loginShell": `"/bin/bash" → "/bin/zsh"`, "homeDirectory": `"/home/sam" → "/home/samuel"`}) {
		t.Fatalf("changes: %v", got)
	}
	expect(t, call(t, h, userAdmin, "PUT", "/api/v1/users/usr_missing/posix", map[string]any{"username": "x", "loginShell": "/bin/sh", "homeDirectory": "/home/x"}), 404)
}

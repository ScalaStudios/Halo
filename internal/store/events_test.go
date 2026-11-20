package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"halo/internal/store"
)

func TestDiff(t *testing.T) {
	type tokens struct {
		AccessTokenTTL int    `json:"accessTokenTtl"`
		ClientSecret   string `json:"clientSecret"`
	}
	type app struct {
		Name         string    `json:"name"`
		RedirectURIs []string  `json:"redirectUris"`
		Tokens       tokens    `json:"tokens"`
		PasswordHash string    `json:"passwordHash"`
		Owner        *string   `json:"owner"`
		UpdatedAt    time.Time `json:"updatedAt"`
	}
	before := app{Name: "Grafana", RedirectURIs: []string{"https://a.example/cb"}, Tokens: tokens{900, "old-secret"}, PasswordHash: "old-hash", UpdatedAt: time.Unix(1, 0)}
	after := before
	after.RedirectURIs = []string{"https://a.example/cb", "https://b.example/cb"}
	after.Tokens = tokens{300, "new-secret"}
	after.PasswordHash = "new-hash"
	after.Owner = new("usr_1")
	after.UpdatedAt = time.Unix(2, 0)

	got, _ := json.Marshal(store.Diff(before, after))
	want := `[{"field":"owner","before":null,"after":"usr_1"},{"field":"passwordHash","redacted":true},{"field":"redirectUris","before":["https://a.example/cb"],"after":["https://a.example/cb","https://b.example/cb"]},{"field":"tokens.accessTokenTtl","before":900,"after":300},{"field":"tokens.clientSecret","redacted":true}]`
	if string(got) != want {
		t.Fatalf("diff:\n%s\nwant:\n%s", got, want)
	}
	if same := store.Diff(before, before); len(same) != 0 {
		t.Fatalf("unchanged values produced a diff: %+v", same)
	}
}

package filter

import (
	"errors"
	"strings"
	"testing"
)

var schema = Schema{
	Prefixes: map[string]string{"urn:ietf:params:scim:schemas:core:2.0:user:": ""},
	Multi:    map[string]string{"members": "exists (select 1 where %s)"},
	Attrs: map[string]Attr{
		"username":      {Expr: "u.email"},
		"emails.type":   {Expr: "'work'"},
		"emails.value":  {Expr: "u.email"},
		"active":        {Expr: "u.active", Kind: Boolean},
		"meta.created":  {Expr: "u.created_at", Kind: DateTime},
		"members.value": {Expr: "m.id", Exact: true},
	},
}

func TestCompile(t *testing.T) {
	for raw, want := range map[string]string{
		`userName eq "a" and active eq true or not (active eq false)`: `(((lower((u.email)) = lower($1::text) and lower((u.email)) is not null) and ((u.active) = $2::boolean and (u.active) is not null)) or (not ((u.active) = $3::boolean and (u.active) is not null)))`,
		`urn:ietf:params:scim:schemas:core:2.0:User:userName sw "a"`:  `(coalesce(starts_with(lower((u.email)), lower($1::text)), false))`,
		`emails[type eq "work"].value ew "@x.org"`:                    `((lower(('work')) = lower($1::text) and lower(('work')) is not null) and (coalesce(right(lower((u.email)), char_length(lower($2::text))) = lower($2::text), false)))`,
		`members[value eq "usr_1"]`:                                   `exists (select 1 where ((m.id) = $1::text and (m.id) is not null))`,
		`members.value pr`:                                            `exists (select 1 where (coalesce((m.id), '') <> ''))`,
		`meta.created gt "2026-01-01T00:00:00Z" and userName eq null`: `((coalesce((u.created_at) > $1::timestamptz, false)) and (not (coalesce((u.email), '') <> '')))`,
		`  `: `true`,
		`userName co "x' or '1'='1" or userName eq "\") or (1=1 --"`: `((coalesce(strpos(lower((u.email)), lower($1::text)) > 0, false)) or (lower((u.email)) = lower($2::text) and lower((u.email)) is not null))`,
	} {
		got, args, err := Compile(raw, schema)
		if err != nil || got != want {
			t.Errorf("%s\n got %s %v\nwant %s", raw, got, err, want)
		}
		for _, a := range args {
			if s, ok := a.(string); ok && strings.ContainsAny(s, `'"`) && strings.Contains(got, s) {
				t.Errorf("%s: value %q leaked into the SQL", raw, s)
			}
		}
	}
}

func TestCompileRejects(t *testing.T) {
	for _, raw := range []string{
		`userName eq`,
		`userName eq "a" and`,
		`(userName eq "a"`,
		`userName eq "a")`,
		`userName xx "a"`,
		`userName "eq" "a"`,
		`nickName eq "a"`,
		`active co "a"`,
		`active eq "true"`,
		`meta.created gt "yesterday"`,
		`userName gt 5`,
		`not userName eq "a"`,
		`userName eq "unterminated`,
		`userName eq "nul\u0000"`,
		`emails[type eq "work"`,
		`emails[type eq "work"].nickName eq "a"`,
		`members[value eq "a" and members[value eq "b"]]`,
		`userName) or 1=1 -- eq "a"`,
		`userName eq "a" userName eq "b"`,
		strings.Repeat("(", 20) + `userName eq "a"` + strings.Repeat(")", 20),
		`userName eq "` + strings.Repeat("a", 5000) + `"`,
	} {
		_, _, err := Compile(raw, schema)
		var e Error
		if !errors.As(err, &e) || e == "" {
			t.Errorf("%s: want a filter error, got %v", raw, err)
		}
	}
}

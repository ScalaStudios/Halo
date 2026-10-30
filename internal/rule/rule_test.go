package rule

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseAccepts(t *testing.T) {
	for _, src := range []string{
		`user.status == "active"`,
		`  user.department==""  `,
		`true`,
		`user.email endsWith "@lunarlabs.dev" and user.status == "active"`,
		`user.department in ["Engineering", "Operations"] and not user.title == "Contractor"`,
		`user.memberOf("Staff") or user.memberOf("Contractors")`,
		`not (user.name contains "a" or user.emailDomain startsWith "b") and user.manager != "x@y.z" and user.kind == "person"`,
		`user.location in ["Lisbon"]`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for src, want := range map[string]string{
		`*`:                                   "Position 1: unexpected '*'",
		``:                                    "Position 1: expected user.<attribute>, not, ( or true, found the end of the rule.",
		`user.status = "active"`:              "Position 13: unexpected '='",
		`user.salary == "1"`:                  `Position 6: expected an attribute: status`,
		`user.status == active`:               `Position 16: expected a value in double quotes, like "Engineering", found "active".`,
		`user.status == "active`:              "Position 16: this quote is never closed",
		`user.status is "active"`:             `Position 13: expected ==, !=, contains, startsWith, endsWith or in, found "is".`,
		`(user.status == "a"`:                 "Position 20: expected ) to close the parenthesis at position 1, found the end of the rule.",
		`user.status == "a" user.title`:       `Position 20: expected and, or, or the end of the rule, found "user".`,
		`user.status in ["a" "b"]`:            `Position 21: expected ], found the value "b".`,
		`user.memberOf(Staff)`:                "Position 15: expected a value in double quotes",
		`user.status == "a" and`:              "Position 23: expected user.<attribute>",
		`user."status" == "a"`:                `Position 6: expected an attribute: status, department, location, title, source, email, emailDomain, name, kind, manager or memberOf, found the value "status".`,
		`user.status == "ä" and user.x == ""`: "Position 29: expected an attribute",
	} {
		_, err := Parse(src)
		var e *Error
		if !errors.As(err, &e) || !strings.HasPrefix(e.Error(), want) {
			t.Errorf("%s: got %v, want prefix %q", src, err, want)
		}
	}
}

func TestCompileParameterizes(t *testing.T) {
	c := &Compiler{Args: []any{"caller"}}
	hostile := `x' or 1=1; drop table users; --`
	sql, err := c.Rule("grp_a", `user.title == "`+hostile+`" or user.department in ["a", "$1"] and not user.email endsWith "@x"`)
	if err != nil {
		t.Fatal(err)
	}
	want := `(lower(u.title) = lower($2::text) or (lower(u.department) in (lower($3::text), lower($4::text)) and not (right(lower(u.email), char_length(lower($5::text))) = lower($5::text))))`
	if sql != want {
		t.Fatalf("sql:\n%s\nwant:\n%s", sql, want)
	}
	if !slices.Equal(c.Args, []any{"caller", hostile, "a", "$1", "@x"}) {
		t.Fatalf("args: %v", c.Args)
	}
	if strings.Contains(sql, "drop") {
		t.Fatal("user text leaked into SQL")
	}
}

func TestCompileLegacyRule(t *testing.T) {
	c := &Compiler{}
	sql, err := c.Rule("grp_a", `user.department == "Engineering"`)
	if err != nil || sql != `lower(u.department) = lower($1::text)` || !slices.Equal(c.Args, []any{"Engineering"}) {
		t.Fatalf("%s %v %v", sql, c.Args, err)
	}
}

func TestMemberOf(t *testing.T) {
	ptr := func(s string) *string { return &s }
	c := &Compiler{Groups: map[string]Group{
		"staff":       {ID: "grp_staff", Rule: ptr(`user.memberOf("Contractors") or user.status == "active"`)},
		"contractors": {ID: "grp_contractors"},
		"loop a":      {ID: "grp_a", Rule: ptr(`user.memberOf("Loop B")`)},
		"loop b":      {ID: "grp_b", Rule: ptr(`user.memberOf("Loop A")`)},
		"broken":      {ID: "grp_broken", Rule: ptr(`user.memberOf("Gone")`)},
	}}
	sql, err := c.Rule("grp_new", `user.memberOf("staff")`)
	if err != nil {
		t.Fatal(err)
	}
	if want := `((exists (select 1 from group_members gm where gm.group_id = $1::text and gm.user_id = u.id) or lower(u.status) = lower($2::text)))`; sql != want {
		t.Fatalf("sql:\n%s\nwant:\n%s", sql, want)
	}
	for _, tc := range []struct{ group, src, want string }{
		{"grp_staff", `user.memberOf("Staff")`, `Position 1: user.memberOf("Staff") makes a loop`},
		{"grp_a", `user.status == "a" or user.memberOf("Loop B")`, `Position 23: user.memberOf("Loop B") makes a loop`},
		{"grp_a", `user.memberOf("Loop B")`, `Position 1: user.memberOf("Loop B") makes a loop`},
		{"grp_new", `user.memberOf("Loop B")`, `Position 1: the rule of "Loop B" cannot be used`},
		{"grp_new", `user.memberOf("Broken")`, `Position 1: the rule of "Broken" cannot be used`},
		{"grp_new", `true and user.memberOf("Nobody")`, `Position 10: there is no group named "Nobody".`},
	} {
		n := len(c.Args)
		_, err := c.Rule(tc.group, tc.src)
		if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("%s in %s: got %v, want prefix %q", tc.src, tc.group, err, tc.want)
		}
		if len(c.Args) != n {
			t.Errorf("%s: failed compile left %d parameters behind", tc.src, len(c.Args)-n)
		}
	}
}

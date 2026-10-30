package secret

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	first, second := HashPassword("correct horse battery staple"), HashPassword("correct horse battery staple")
	if !strings.HasPrefix(first, "$argon2id$v=19$m=65536,t=3,p=4$") || first == second {
		t.Fatalf("hashes must be salted Argon2id with their parameters: %s %s", first, second)
	}
	if strings.Contains(first, "correct horse") {
		t.Fatalf("hash leaks the password: %s", first)
	}
	if !PasswordMatches(first, "correct horse battery staple") || !PasswordMatches(second, "correct horse battery staple") {
		t.Fatal("the right password must match")
	}
	for _, wrong := range []string{"", "correct horse battery stapl", "Correct horse battery staple", "correct horse battery staple "} {
		if PasswordMatches(first, wrong) {
			t.Fatalf("%q must not match", wrong)
		}
	}
	if !PasswordMatches(HashPassword("ﬁnancial planning 2026"), "financial planning 2026") || !PasswordMatches(HashPassword("Ångström unit of length"), "Ångström unit of length") {
		t.Fatal("passwords must be compared after NFKC normalization")
	}
}

func TestPasswordParametersTravelWithTheHash(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2id("old enough passphrase", salt, 1, 8*1024, 1, 32)
	weaker := "$argon2id$v=19$m=8192,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
	if !PasswordMatches(weaker, "old enough passphrase") || PasswordMatches(weaker, "another passphrase") {
		t.Fatal("hashes made with other parameters must keep verifying")
	}
	for _, malformed := range []string{"", "plain", "$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=16$m=65536,t=3,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=65536,t=0,p=4$c2FsdA$aGFzaA", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$"} {
		if PasswordMatches(malformed, "") {
			t.Fatalf("malformed hash %q matched", malformed)
		}
	}
}

func TestPasswordProblem(t *testing.T) {
	ctx := context.Background()
	if problem := PasswordProblem(ctx, "fourteen chars", false); !strings.Contains(problem, "at least 15") {
		t.Fatalf("short password: %q", problem)
	}
	if problem := PasswordProblem(ctx, strings.Repeat("é", 257), false); !strings.Contains(problem, "256 characters or fewer") {
		t.Fatalf("long password: %q", problem)
	}
	if problem := PasswordProblem(ctx, strings.Repeat("é", 256), false); problem != "" {
		t.Fatalf("characters must be counted, not bytes: %q", problem)
	}
	if problem := PasswordProblem(ctx, "lowercase only, no digits", false); problem != "" {
		t.Fatalf("no composition rules: %q", problem)
	}

	var asked []string
	pwned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.Header.Get("Add-Padding") != "true" {
			t.Errorf("range request without padding")
		}
		fmt.Fprint(w, "0018A45C4D1DEF81644B54AB7F969B88D65:0\r\n4E01222585930982009E68112AC270DE1EE:3\r\n")
	}))
	defer pwned.Close()
	pwnedRange = pwned.URL + "/range/"
	defer func() { pwnedRange = "https://api.pwnedpasswords.com/range/" }()

	if problem := PasswordProblem(ctx, "password1234567", true); !strings.Contains(problem, "data breach") {
		t.Fatalf("breached password: %q", problem)
	}
	if len(asked) != 1 || asked[0] != "/range/648B5" {
		t.Fatalf("only the 5-character prefix may leave Halo: %v", asked)
	}
	if problem := PasswordProblem(ctx, "a perfectly fine passphrase", true); problem != "" {
		t.Fatalf("unbreached password: %q", problem)
	}
	if problem := PasswordProblem(ctx, "password1234567", false); problem != "" {
		t.Fatalf("breach check turned off: %q", problem)
	}

	pwned.Close()
	if problem := PasswordProblem(ctx, "password1234567", true); problem != "" {
		t.Fatalf("an unreachable breach check must fail open: %q", problem)
	}
}

package secret

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const (
	minPasswordLength  = 15
	maxPasswordLength  = 256
	passwordIterations = 3
	passwordMemory     = 64 * 1024
	passwordThreads    = 4
)

var (
	hashing    = make(chan struct{}, runtime.GOMAXPROCS(0))
	pwnedRange = "https://api.pwnedpasswords.com/range/"
)

func argon2id(password string, salt []byte, iterations, memory uint32, threads uint8, size uint32) []byte {
	hashing <- struct{}{}
	defer func() { <-hashing }()
	return argon2.IDKey([]byte(norm.NFKC.String(password)), salt, iterations, memory, threads, size)
}

func HashPassword(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2id(password, salt, passwordIterations, passwordMemory, passwordThreads, 32)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, passwordMemory, passwordIterations, passwordThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func PasswordMatches(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || iterations == 0 || threads == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	return Equal(argon2id(password, salt, iterations, memory, threads, uint32(len(want))), want)
}

func PasswordProblem(ctx context.Context, password string, breachCheck bool) string {
	length := utf8.RuneCountInString(norm.NFKC.String(password))
	switch {
	case length < minPasswordLength:
		return fmt.Sprintf("Use at least %d characters. A few unrelated words make a password that is long and easy to remember.", minPasswordLength)
	case length > maxPasswordLength:
		return fmt.Sprintf("Use %d characters or fewer.", maxPasswordLength)
	case breachCheck && breached(ctx, password):
		return "This password appeared in a known data breach, so attackers try it early. Choose a different one."
	}
	return ""
}

func breached(ctx context.Context, password string) bool {
	digest := fmt.Sprintf("%X", sha1.Sum([]byte(norm.NFKC.String(password))))
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pwnedRange+digest[:5], nil)
	if err != nil {
		return false
	}
	req.Header.Set("Add-Padding", "true")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "breached password check skipped: Have I Been Pwned is unreachable", "error", errors.Unwrap(err))
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.WarnContext(ctx, "breached password check skipped: Have I Been Pwned answered with an error", "status", res.StatusCode)
		return false
	}
	lines := bufio.NewScanner(res.Body)
	for lines.Scan() {
		suffix, count, _ := strings.Cut(strings.TrimSpace(lines.Text()), ":")
		if suffix == digest[5:] && count != "0" {
			return true
		}
	}
	if err := lines.Err(); err != nil {
		slog.WarnContext(ctx, "breached password check skipped: the answer from Have I Been Pwned was cut off", "error", err)
	}
	return false
}

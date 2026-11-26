package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"halo/internal/store"
)

func TestLockoutHoldsUnderConcurrentGuesses(t *testing.T) {
	e := newEnv(t)
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.withAuthenticator(ada)
	e.org(func(s *store.Settings) { s.LockoutThreshold, s.LockoutMinutes = 5, 15 })

	started := time.Now()
	valid := []string{}
	for step := -2; step <= 2; step++ {
		code, _ := totp.GenerateCode(authenticatorSeed, started.Add(time.Duration(step)*30*time.Second))
		valid = append(valid, code)
	}
	guesses := []string{}
	for i := 0; len(guesses) < 200; i++ {
		if code := fmt.Sprintf("%06d", i); !slices.Contains(valid, code) {
			guesses = append(guesses, code)
		}
	}

	statuses := make([]int, len(guesses))
	var wg sync.WaitGroup
	for i, code := range guesses {
		wg.Go(func() {
			body, _ := json.Marshal(map[string]string{"email": "ada@example.com", "code": code})
			res, err := http.Post(e.srv.URL+"/api/v1/auth/totp", "application/json", bytes.NewReader(body))
			if err != nil {
				statuses[i] = -1
				return
			}
			res.Body.Close()
			statuses[i] = res.StatusCode
		})
	}
	wg.Wait()

	reached := 0
	for _, status := range statuses {
		switch status {
		case http.StatusTooManyRequests:
		case http.StatusUnauthorized:
			reached++
		default:
			t.Fatalf("unexpected status %d in %v", status, statuses)
		}
	}
	if reached != 5 {
		t.Fatalf("%d of %d concurrent guesses were checked, want 5", reached, len(guesses))
	}
	code, _ := totp.GenerateCode(authenticatorSeed, time.Now())
	e.expect(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": code}, http.StatusTooManyRequests, "ERR_TOO_MANY_ATTEMPTS")

	mismatches, err := e.st.RecentMismatches(context.Background(), ada.ID, "127.0.0.1", started.Add(-time.Minute))
	if err != nil || mismatches != 5 {
		t.Fatalf("wrong codes flagged as mismatches: %d %v", mismatches, err)
	}
}

func TestLockoutRefusalsDoNotExtendTheWindow(t *testing.T) {
	e := newEnv(t)
	ada := e.withPassword("ada@example.com", "correct horse battery staple", false)
	e.withAuthenticator(ada)
	e.org(func(s *store.Settings) { s.LockoutThreshold, s.LockoutMinutes = 5, 15 })

	ctx := context.Background()
	record := func(at time.Time, reason string) {
		t.Helper()
		if err := e.st.RecordSignIn(ctx, store.SignInEvent{Time: at, UserID: &ada.ID, Email: ada.Email, Result: "failure", Method: "totp", IP: "127.0.0.1", Reason: reason}); err != nil {
			t.Fatal(err)
		}
	}
	fifth := time.Now().Add(-16 * time.Minute)
	for range 5 {
		record(fifth, "The code did not match.")
	}
	for i := 1; i <= 7; i++ {
		record(fifth.Add(time.Duration(2*i)*time.Minute), store.LockedOutReason)
		record(fifth.Add(time.Duration(2*i)*time.Minute), store.SprayedReason)
	}

	code, _ := totp.GenerateCode(authenticatorSeed, time.Now())
	if status := e.do(client(), "POST", "/api/v1/auth/totp", map[string]any{"email": "ada@example.com", "code": code}, nil); status != http.StatusOK {
		t.Fatalf("refused attempts kept the account locked: %d", status)
	}
}

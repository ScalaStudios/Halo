package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	errChallengeRequired    = httpx.Fail(http.StatusForbidden, "ERR_CHALLENGE_REQUIRED", "Confirm you're a person to continue. Complete the check on the sign-in page, then try again.")
	errChallengeFailed      = httpx.Fail(http.StatusForbidden, "ERR_CHALLENGE_FAILED", "Halo couldn't confirm you're a person. Complete the check again, or sign in with a passkey.")
	errChallengeUnavailable = httpx.Fail(http.StatusServiceUnavailable, "ERR_CHALLENGE_UNAVAILABLE", "Halo couldn't reach the service that checks for automated sign-ins. Try again in a few minutes, or sign in with a passkey.")

	siteverify = map[string]string{
		"turnstile": "https://challenges.cloudflare.com/turnstile/v0/siteverify",
		"hcaptcha":  "https://api.hcaptcha.com/siteverify",
	}
	siteverifyClient = &http.Client{Timeout: 5 * time.Second}
	guessedMethods   = []string{"password", "totp", "recovery-codes", "magic-link", "access-pass"}
)

func (h *handler) challenge(r *http.Request, a *attempt, token string, settings store.Settings, always bool) error {
	if settings.BotProtection == "" {
		return nil
	}
	if !always {
		failures, err := h.st.RecentFailuresFromIP(r.Context(), httpx.ClientIP(r), guessedMethods, lockoutStart(settings))
		if err != nil {
			return err
		}
		if failures < settings.LockoutThreshold {
			return nil
		}
	}
	if token == "" {
		return errChallengeRequired
	}
	passed, err := h.verifyChallenge(r, settings.BotProtection, token)
	if err != nil {
		slog.ErrorContext(r.Context(), "bot check not verified", "provider", settings.BotProtection, "error", err)
		return h.refuse(r, a, "unavailable", "The bot check service could not be reached.", errChallengeUnavailable)
	}
	if !passed {
		return h.refuse(r, a, "failed", "The bot check failed.", errChallengeFailed)
	}
	if a != nil {
		a.challenge = "passed"
	}
	return nil
}

func (h *handler) refuse(r *http.Request, a *attempt, outcome, reason string, e *httpx.Error) error {
	if a == nil {
		return e
	}
	a.challenge = outcome
	return h.reject(r, *a, reason, e)
}

func (h *handler) verifyChallenge(r *http.Request, provider, token string) (bool, error) {
	key, err := h.st.BotProtectionSecret(r.Context())
	if err != nil {
		return false, err
	}
	form := url.Values{"secret": {key}, "response": {token}, "remoteip": {httpx.ClientIP(r)}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, siteverify[provider], strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := siteverifyClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("siteverify answered %d", res.StatusCode)
	}
	var out struct {
		Success bool     `json:"success"`
		Errors  []string `json:"error-codes"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&out); err != nil {
		return false, err
	}
	if !out.Success && slices.ContainsFunc(out.Errors, func(code string) bool { return strings.Contains(code, "secret") || strings.Contains(code, "sitekey") }) {
		return false, fmt.Errorf("siteverify rejected the configuration: %v", out.Errors)
	}
	return out.Success, nil
}

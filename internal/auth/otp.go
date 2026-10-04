package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

const totpPeriod = 30

func (h *handler) totpSignIn(w http.ResponseWriter, r *http.Request) error {
	return h.codeSignIn(w, r, "totp", errCodeMismatch)
}

func (h *handler) recoverySignIn(w http.ResponseWriter, r *http.Request) error {
	return h.codeSignIn(w, r, "recovery-codes", errRecoveryInvalid)
}

func (h *handler) codeSignIn(w http.ResponseWriter, r *http.Request, method string, mismatch *httpx.Error) error {
	var in struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		AuthRequest string `json:"authRequest"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.target(ctx, in.AuthRequest)
	if err != nil {
		return err
	}
	a := attempt{method: method, email: strings.TrimSpace(in.Email), app: app, request: in.AuthRequest}
	u, err := h.st.GetUserByEmail(ctx, a.email)
	if err == nil {
		a.user = &u
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	failures, err := h.st.RecentFailures(ctx, u.ID, a.email, []string{"totp", "recovery-codes"}, time.Now().Add(-time.Duration(settings.LockoutMinutes)*time.Minute))
	if err != nil {
		return err
	}
	if failures >= settings.LockoutThreshold {
		return h.reject(r, a, "Too many failed attempts.", httpx.Fail(http.StatusTooManyRequests, errTooManyAttempts.Code, fmt.Sprintf("Too many incorrect codes. Wait %d minutes, then try again.", settings.LockoutMinutes)))
	}
	if a.user == nil {
		return h.reject(r, a, "No account uses this email address.", mismatch)
	}
	reason := ""
	if method == "totp" {
		reason, err = h.useTOTP(ctx, u.ID, in.Code)
	} else if err = h.st.UseRecoveryCode(ctx, u.ID, h.st.RecoveryCodeHash(normalizeRecoveryCode(in.Code))); errors.Is(err, store.ErrNotFound) {
		reason, err = "The recovery code did not match an unused code.", nil
	}
	if err != nil {
		return err
	}
	if reason != "" {
		return h.reject(r, a, reason, mismatch)
	}
	return h.finish(w, r, a, nil, nil)
}

func (h *handler) useTOTP(ctx context.Context, userID, code string) (string, error) {
	secrets, err := h.st.ListTOTPSecrets(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, s := range secrets {
		if !s.Confirmed {
			continue
		}
		step, ok, err := h.matchTOTP(s.Sealed, code)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		if step <= s.LastStep {
			return "The code was already used.", nil
		}
		advanced, err := h.st.AdvanceTOTP(ctx, s.ID, step)
		if err != nil || !advanced {
			return "The code was already used.", err
		}
		return "", nil
	}
	return "The code did not match.", nil
}

func (h *handler) matchTOTP(sealed []byte, code string) (int64, bool, error) {
	seed, err := h.st.Sealer.Open(sealed)
	if err != nil {
		return 0, false, err
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	now := time.Now().Unix() / totpPeriod
	for step := now - 1; step <= now+1; step++ {
		want, err := hotp.GenerateCodeCustom(string(seed), uint64(step), hotp.ValidateOpts{Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if err != nil {
			return 0, false, err
		}
		if secret.Equal([]byte(want), []byte(code)) {
			return step, true, nil
		}
	}
	return 0, false, nil
}

func normalizeRecoveryCode(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(code))
}

func (h *handler) totpBegin(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Halo", AccountName: u.Email})
	if err != nil {
		return err
	}
	ctx := r.Context()
	secretID, err := h.st.CreateTOTPSecret(ctx, u.ID, "Authenticator app", h.st.Sealer.Seal([]byte(key.Secret())))
	if err != nil {
		return err
	}
	ceremonyID, err := h.st.CreateCeremony(ctx, "totp-enroll", &u.ID, ceremony{TOTPID: secretID})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"ceremony": ceremonyID, "secret": key.Secret(), "uri": key.URL()})
}

func (h *handler) totpConfirm(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ceremony string `json:"ceremony"`
		Code     string `json:"code"`
		Label    string `json:"label"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	if in.Label = strings.TrimSpace(in.Label); in.Label == "" {
		in.Label = "Authenticator app"
	}
	ctx := r.Context()
	var m store.Method
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		var c ceremony
		owner, err := tx.TakeCeremony(ctx, in.Ceremony, "totp-enroll", &c)
		if errors.Is(err, store.ErrNotFound) || err == nil && (owner == nil || *owner != u.ID) {
			return errCeremonyExpired
		}
		if err != nil {
			return err
		}
		secrets, err := tx.ListTOTPSecrets(ctx, u.ID)
		if err != nil {
			return err
		}
		for _, s := range secrets {
			if s.ID != c.TOTPID || s.Confirmed {
				continue
			}
			step, ok, err := h.matchTOTP(s.Sealed, in.Code)
			if err != nil {
				return err
			}
			if !ok {
				return errCodeMismatch
			}
			if m, err = tx.ConfirmTOTPSecret(ctx, s.ID, in.Label, step); err != nil {
				return err
			}
			return audit(ctx, tx, r, u, "user.method.add", u.Name+" added "+in.Label)
		}
		return errCeremonyExpired
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, m)
}

func (h *handler) recoveryCodes(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	codes := make([]string, 10)
	hashes := make([][]byte, len(codes))
	for i := range codes {
		raw := strings.ToLower(rand.Text()[:16])
		codes[i], hashes[i] = raw[:4]+"-"+raw[4:8]+"-"+raw[8:12]+"-"+raw[12:], h.st.RecoveryCodeHash(raw)
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.ReplaceRecoveryCodes(ctx, u.ID, hashes); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.method.add", u.Name+" generated new recovery codes")
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"codes": codes})
}

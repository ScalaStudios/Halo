package auth

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"halo/internal/httpx"
	"halo/internal/secret"
	"halo/internal/store"
)

var (
	errPasswordMismatch  = httpx.Fail(http.StatusUnauthorized, "ERR_PASSWORD_MISMATCH", "That email and password don't match. Check them and try again, or sign in another way.")
	errSecondFactor      = httpx.Fail(http.StatusUnauthorized, "ERR_SECOND_FACTOR_REQUIRED", "Enter the code from your authenticator app to finish signing in.")
	errNoSecondFactor    = httpx.Fail(http.StatusForbidden, "ERR_SECOND_FACTOR_MISSING", "Your organization asks for an authenticator app as well as a password, and this account has none. Sign in with a passkey or security key instead, or ask your administrator for help.")
	errPasswordChange    = httpx.Fail(http.StatusUnauthorized, "ERR_PASSWORD_CHANGE_REQUIRED", "Your administrator gave you a temporary password. Choose a new one to finish signing in.")
	errPasswordForbidden = httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Your organization doesn't let you sign in with a password. Use a passkey or another method instead.")
	dummyPassword        = sync.OnceValue(func() string { return secret.HashPassword(secret.Token(16)) })
)

const addressLockoutFactor = 4

func (h *handler) passwordAllowed(r *http.Request, u store.User) (bool, store.Settings, error) {
	settings, err := h.st.Settings(r.Context())
	if err != nil {
		return false, settings, err
	}
	enabled, err := h.st.MethodEnabled(r.Context(), "password")
	return enabled && settings.PasswordAllowed(u), settings, err
}

func (h *handler) passwordSignIn(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Code        string `json:"code"`
		NewPassword string `json:"newPassword"`
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
	a := attempt{method: "password", email: strings.TrimSpace(in.Email), app: app, request: in.AuthRequest}
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
	if err := h.lockout(r, a, settings, "passwords"); err != nil {
		return err
	}
	sprayed, err := h.st.RecentFailuresFromIP(ctx, httpx.ClientIP(r), "password", lockoutStart(settings))
	if err != nil {
		return err
	}
	if sprayed >= addressLockoutFactor*settings.LockoutThreshold {
		return h.reject(r, a, "Too many failed passwords from this IP address.", tooManyAttempts("passwords", settings))
	}
	if enabled, err := h.st.MethodEnabled(ctx, "password"); err != nil {
		return err
	} else if !enabled {
		return h.reject(r, a, "Sign-in with passwords is turned off for this organization.", errPasswordMismatch)
	}
	stored, err := h.st.GetPassword(ctx, u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	hash := stored.Hash
	if hash == "" {
		hash = dummyPassword()
	}
	matched := secret.PasswordMatches(hash, in.Password)
	switch {
	case a.user == nil:
		return h.reject(r, a, "No account uses this email address.", errPasswordMismatch)
	case stored.Hash == "":
		return h.reject(r, a, "The account has no password.", errPasswordMismatch)
	case !matched:
		return h.reject(r, a, "The password did not match.", errPasswordMismatch)
	case !settings.PasswordAllowed(u):
		return h.reject(r, a, "Sign-in with passwords is limited to groups this account is not in.", errPasswordMismatch)
	}
	totp := slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.Kind == "totp" })
	if settings.PasswordSecondFactor && !totp {
		return h.reject(r, a, "The password was correct, but the account has no authenticator app for the second factor.", errNoSecondFactor)
	}
	replacement := ""
	if stored.MustChange {
		switch {
		case in.NewPassword == "":
			return errPasswordChange
		case in.NewPassword == in.Password:
			return httpx.Invalid("Choose a password that is different from the temporary one.")
		}
		if problem := secret.PasswordProblem(ctx, in.NewPassword, settings.PasswordBreachCheck); problem != "" {
			return httpx.Invalid(problem)
		}
		replacement = secret.HashPassword(in.NewPassword)
	}
	if settings.PasswordSecondFactor {
		if strings.TrimSpace(in.Code) == "" {
			return errSecondFactor
		}
		reason, err := h.useTOTP(ctx, u.ID, in.Code)
		if err != nil {
			return err
		}
		if reason != "" {
			return h.reject(r, a, reason, errCodeMismatch)
		}
	}
	return h.finish(w, r, a, nil, func(tx *store.Store) error {
		if replacement != "" {
			if err := tx.SetPassword(ctx, u.ID, replacement, false); err != nil {
				return err
			}
			if err := audit(ctx, tx, r, u, "user.method.add", u.Name+" replaced their temporary password"); err != nil {
				return err
			}
		}
		return tx.TouchPassword(ctx, u.ID)
	})
}

func (h *handler) myPassword(w http.ResponseWriter, r *http.Request) error {
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	allowed, settings, err := h.passwordAllowed(r, u)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]bool{"allowed": allowed, "secondFactor": settings.PasswordSecondFactor})
}

func (h *handler) setMyPassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	allowed, settings, err := h.passwordAllowed(r, u)
	if err != nil {
		return err
	}
	if !allowed {
		return errPasswordForbidden
	}
	ctx := r.Context()
	if problem := secret.PasswordProblem(ctx, in.Password, settings.PasswordBreachCheck); problem != "" {
		return httpx.Invalid(problem)
	}
	summary := u.Name + " set a password"
	if slices.ContainsFunc(u.Methods, func(m store.Method) bool { return m.Kind == "password" }) {
		summary = u.Name + " changed their password"
	}
	hash := secret.HashPassword(in.Password)
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.SetPassword(ctx, u.ID, hash, false); err != nil {
			return err
		}
		return audit(ctx, tx, r, u, "user.method.add", summary)
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"strconv"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

const (
	signUpLimit      = 10
	signUpEmailLimit = 3
	signUpWindow     = time.Hour
)

var (
	errSignUpOff     = httpx.Fail(http.StatusNotFound, "ERR_SIGN_UP_OFF", "Sign-up is turned off. Ask your administrator for an invitation.")
	errSignUpDomain  = httpx.Fail(http.StatusForbidden, "ERR_SIGN_UP_DOMAIN", "Sign-up isn't open to this email domain. Use your organization's email address, or ask your administrator for an invitation.")
	errSignUpExpired = httpx.Fail(http.StatusNotFound, "ERR_LINK_EXPIRED", "This sign-up link has expired or was already used. Sign up again to get a new one.")
	errSignUpTaken   = httpx.Fail(http.StatusConflict, "ERR_ACCOUNT_EXISTS", "An account with this email address already exists. Sign in instead.")
)

func (h *handler) signUp(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		Challenge string `json:"challenge"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return err
	}
	if settings.SignUp == "off" {
		return errSignUpOff
	}
	email, name := strings.TrimSpace(in.Email), strings.TrimSpace(in.Name)
	if addr, err := netmail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		return httpx.Invalid("Enter one email address, such as sam@example.com.")
	}
	if name == "" || len(name) > 200 {
		return httpx.Invalid("Enter your name, in 200 characters or fewer.")
	}
	if n, reset, err := h.st.CountRequest(ctx, httpx.ClientIP(r), "sign-up", signUpLimit, signUpWindow); err == nil && n > signUpLimit {
		w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(reset).Seconds())+1))
		return httpx.Fail(http.StatusTooManyRequests, "ERR_RATE_LIMITED", "Too many people signed up from this network. Wait an hour, then try again.")
	}
	if err := h.signUpDomain(ctx, settings, email); err != nil {
		return err
	}
	if err := h.challenge(r, nil, in.Challenge, settings, true); err != nil {
		return err
	}
	background := context.WithoutCancel(ctx)
	go func() {
		if err := h.mailSignUp(background, email, name); err != nil {
			slog.ErrorContext(background, "sign-up email not sent", "error", err)
		}
	}()
	return httpx.JSON(w, http.StatusAccepted, map[string]string{})
}

func (h *handler) signUpDomain(ctx context.Context, settings store.Settings, email string) error {
	allowed := settings.SignUpDomains
	if len(allowed) == 0 {
		domains, err := h.st.ListDomains(ctx)
		if err != nil {
			return err
		}
		for _, d := range domains {
			if d.VerifiedAt != nil {
				allowed = append(allowed, d.Name)
			}
		}
	}
	if len(allowed) == 0 || !domainAllowed(allowed, email) {
		return errSignUpDomain
	}
	return nil
}

func (h *handler) mailSignUp(ctx context.Context, email, name string) error {
	_, err := h.st.GetUserByEmail(ctx, email)
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	n, _, err := h.st.CountRequest(ctx, strings.ToLower(email), "sign-up-email", signUpEmailLimit, signUpWindow)
	if err != nil || n > signUpEmailLimit {
		return err
	}
	token, expires, err := h.st.RequestSignUp(ctx, email, name)
	if err != nil || token == "" {
		return err
	}
	return h.mailer.Send(ctx, mail.SignUp(email, name, h.cfg.Issuer()+"/sign-up?token="+token, expires))
}

func (h *handler) pendingSignUp(r *http.Request, token string) (store.SignUp, store.Settings, error) {
	ctx := r.Context()
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return store.SignUp{}, settings, err
	}
	if settings.SignUp == "off" {
		return store.SignUp{}, settings, errSignUpOff
	}
	su, err := h.st.PeekSignUp(ctx, token)
	if errors.Is(err, store.ErrNotFound) {
		return su, settings, errSignUpExpired
	}
	if err != nil {
		return su, settings, err
	}
	return su, settings, h.signUpDomain(ctx, settings, su.Email)
}

func (h *handler) signUpInfo(w http.ResponseWriter, r *http.Request) error {
	su, _, err := h.pendingSignUp(r, r.URL.Query().Get("token"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"user": map[string]string{"name": su.Name, "email": su.Email}, "purpose": "sign-up"})
}

func (h *handler) signUpBegin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	su, _, err := h.pendingSignUp(r, in.Token)
	if err != nil {
		return err
	}
	return h.registerBegin(w, r, store.User{ID: su.UserID, Email: su.Email, Name: su.Name}, "", nil)
}

func (h *handler) signUpFinish(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token      string          `json:"token"`
		Ceremony   string          `json:"ceremony"`
		Credential json.RawMessage `json:"credential"`
		Label      string          `json:"label"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	su, settings, err := h.pendingSignUp(r, in.Token)
	if err != nil {
		return err
	}
	cred, err := h.registerFinish(r, store.User{ID: su.UserID, Email: su.Email, Name: su.Name}, in.Ceremony, in.Credential, in.Label)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var u store.User
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		verified, err := tx.VerifySignUp(ctx, in.Token, cred)
		if errors.Is(err, store.ErrNotFound) {
			return errSignUpExpired
		}
		if err != nil {
			return err
		}
		event := store.AuditEvent{Action: "user.sign_up", Summary: "Signed up as " + verified.Email + " and is waiting for approval", TargetType: "sign_up", TargetID: verified.ID, TargetLabel: verified.Name, IP: httpx.ClientIP(r)}
		if settings.SignUp == "open" {
			if u, err = tx.ActivateSignUp(ctx, verified.ID, settings.SignUpGroupIDs); errors.Is(err, store.ErrConflict) {
				return errSignUpTaken
			} else if err != nil {
				return err
			}
			event.ActorID, event.Summary, event.TargetType, event.TargetID = &u.ID, "Signed up as "+u.Email+" with "+cred.Label, "user", u.ID
		}
		return tx.RecordAudit(ctx, event)
	})
	if err != nil {
		return err
	}
	if settings.SignUp != "open" {
		return httpx.JSON(w, http.StatusOK, map[string]bool{"pending": true})
	}
	return h.finish(w, r, attempt{method: cred.Kind, user: &u}, nil, nil)
}

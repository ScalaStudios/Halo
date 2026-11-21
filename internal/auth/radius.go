package auth

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/mail"
	"halo/internal/store"
)

type RADIUS struct {
	h *handler
}

type RADIUSRequest struct {
	AppID    string
	Email    string
	Password string
	State    string
	IP       string
	Device   string
}

type RADIUSReply struct {
	Accept         bool
	Challenge      string
	SessionTimeout time.Duration
}

type radiusChallenge struct {
	AppID string `json:"appId"`
}

func NewRADIUS(d httpx.Deps) *RADIUS {
	return &RADIUS{h: &handler{st: d.Store, cfg: d.Config, policy: cmp.Or[access.Engine](d.Policy, access.AllowAll{}), mailer: cmp.Or[mail.Mailer](d.Mailer, mail.Log{})}}
}

func (x *RADIUS) SignIn(ctx context.Context, in RADIUSRequest) (RADIUSReply, error) {
	h := x.h
	app, err := h.st.GetApplication(ctx, in.AppID)
	if err != nil {
		return RADIUSReply{}, err
	}
	a := attempt{method: "password", email: strings.TrimSpace(in.Email), app: &app}
	u, err := h.st.GetUserByEmail(ctx, a.email)
	if err == nil {
		a.user = &u
	} else if !errors.Is(err, store.ErrNotFound) {
		return RADIUSReply{}, err
	}
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return RADIUSReply{}, err
	}
	if locked, err := h.lockedOut(ctx, a, settings); err != nil || locked {
		return RADIUSReply{}, x.reject(ctx, a, in, lockedOutReason, err)
	}
	if sprayed, err := h.sprayed(ctx, in.IP, settings); err != nil || sprayed {
		return RADIUSReply{}, x.reject(ctx, a, in, sprayedReason, err)
	}
	if in.State != "" {
		return x.code(ctx, a, in, settings)
	}
	stored, reason, err := h.checkPassword(ctx, a, in.Password, settings)
	if err != nil || reason != "" {
		return RADIUSReply{}, x.reject(ctx, a, in, reason, err)
	}
	if stored.MustChange {
		return RADIUSReply{}, nil
	}
	if settings.PasswordSecondFactor {
		if !hasTOTP(u) {
			return RADIUSReply{}, x.reject(ctx, a, in, noSecondFactorReason, nil)
		}
		state, err := h.st.CreateCeremony(ctx, "radius", &u.ID, radiusChallenge{AppID: app.ID})
		return RADIUSReply{Challenge: state}, err
	}
	return x.finish(ctx, a, in, settings)
}

func (x *RADIUS) code(ctx context.Context, a attempt, in RADIUSRequest, settings store.Settings) (RADIUSReply, error) {
	var c radiusChallenge
	userID, err := x.h.st.TakeCeremony(ctx, in.State, "radius", &c)
	if errors.Is(err, store.ErrNotFound) || err == nil && (a.user == nil || userID == nil || *userID != a.user.ID || c.AppID != a.app.ID) {
		return RADIUSReply{}, x.reject(ctx, a, in, "The code arrived after the sign-in expired, or for another sign-in.", nil)
	}
	if err != nil {
		return RADIUSReply{}, err
	}
	reason, err := x.h.useTOTP(ctx, a.user.ID, in.Password)
	if err != nil || reason != "" {
		return RADIUSReply{}, x.reject(ctx, a, in, reason, err)
	}
	a.secondFactor = true
	return x.finish(ctx, a, in, settings)
}

func (x *RADIUS) finish(ctx context.Context, a attempt, in RADIUSRequest, settings store.Settings) (RADIUSReply, error) {
	h, u, app := x.h, *a.user, *a.app
	switch {
	case u.Status == "suspended" || u.Status == "deprovisioned":
		return RADIUSReply{}, x.reject(ctx, a, in, "The account is suspended.", nil)
	case app.Status != "active":
		return RADIUSReply{}, x.reject(ctx, a, in, "The application is disabled.", nil)
	case !assigned(u, app):
		return RADIUSReply{}, x.reject(ctx, a, in, notAssignedReason, nil)
	case slices.Contains(settings.EmergencyAccessIDs, u.ID):
		return RADIUSReply{}, x.reject(ctx, a, in, "Emergency access accounts sign in to Halo only, not over RADIUS.", nil)
	}
	if missing, err := h.unproven(ctx, u); err != nil || missing {
		return RADIUSReply{}, x.reject(ctx, a, in, "The account is waiting for an identity check.", err)
	}
	decision, err := h.policy.Evaluate(ctx, access.Input{User: u, App: &app, Method: a.method, IP: in.IP})
	if err != nil {
		return RADIUSReply{}, err
	}
	a.eventID, a.risk = decision.EventID, decision.Risk
	_, unmet := decision.StepUp(nil, store.Session{Method: a.method, SecondFactor: a.secondFactor, CreatedAt: time.Now()})
	switch {
	case decision.Effect == access.Block || decision.Effect == access.RequirePhishingResistant:
		return RADIUSReply{}, x.reject(ctx, a, in, policyReason(decision), nil)
	case len(decision.Terms) > 0:
		return RADIUSReply{}, x.reject(ctx, a, in, "Terms of use are waiting to be accepted in Halo.", nil)
	case unmet != nil:
		return RADIUSReply{}, x.reject(ctx, a, in, "The sign-in does not meet the authentication context “"+unmet.Name+"”.", nil)
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.TouchPassword(ctx, u.ID); err != nil {
			return err
		}
		if err := tx.TouchSignIn(ctx, u.ID, time.Now()); err != nil {
			return err
		}
		return tx.RecordSignIn(ctx, a.eventFrom(in.IP, in.Device, "success", ""))
	})
	timeout := settings.SessionLifetime()
	if app.MaxAuthAge > 0 {
		timeout = time.Duration(app.MaxAuthAge) * time.Second
	}
	return RADIUSReply{Accept: err == nil, SessionTimeout: timeout}, err
}

func (x *RADIUS) reject(ctx context.Context, a attempt, in RADIUSRequest, reason string, err error) error {
	if err != nil {
		return err
	}
	return x.h.fail(ctx, a, a.eventFrom(in.IP, in.Device, "failure", reason))
}

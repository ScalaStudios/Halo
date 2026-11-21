package auth

import (
	"cmp"
	"context"
	"errors"
	"time"

	"halo/internal/access"
	"halo/internal/httpx"
	"halo/internal/metrics"
	"halo/internal/secret"
	"halo/internal/store"
)

func PasswordBind(ctx context.Context, d httpx.Deps, app store.Application, email, password, ip string) (string, error) {
	h := &handler{st: d.Store, policy: d.Policy, mailer: d.Mailer, cfg: d.Config}
	event := store.SignInEvent{Email: email, AppID: &app.ID, Result: "failure", Method: "password", IP: ip, Device: "LDAP"}
	var user *store.User
	refuse := func(reason string) (string, error) {
		event.Reason = reason
		if err := h.st.RecordSignIn(ctx, event); err != nil {
			return "", err
		}
		if user != nil {
			go h.notifyLocked(context.WithoutCancel(ctx), *user)
		}
		return "", nil
	}
	u, err := h.st.GetUserByEmail(ctx, email)
	if err == nil {
		user, event.UserID, event.Email = &u, &u.ID, u.Email
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return "", err
	}
	failures, err := h.st.RecentFailures(ctx, u.ID, email, guessedMethods, lockoutStart(settings))
	if err != nil {
		return "", err
	}
	if failures >= settings.LockoutThreshold {
		metrics.Lockouts.Inc("account")
		return refuse("Too many failed attempts.")
	}
	enabled, err := h.st.MethodEnabled(ctx, "password")
	if err != nil {
		return "", err
	}
	stored, err := h.st.GetPassword(ctx, u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	matched := secret.PasswordMatches(cmp.Or(stored.Hash, dummyPassword()), password)
	switch {
	case !enabled:
		return refuse("Sign-in with passwords is turned off for this organization.")
	case user == nil:
		return refuse("No account uses this email address.")
	case stored.Hash == "":
		return refuse("The account has no password.")
	case !matched:
		return refuse("The password did not match.")
	case !settings.PasswordAllowed(u):
		return refuse("Sign-in with passwords is limited to groups this account is not in.")
	case stored.MustChange:
		return refuse("The account has a temporary password, which an LDAP bind can't replace.")
	case settings.PasswordSecondFactor:
		return refuse("Passwords need an authenticator app code as well, which an LDAP bind can't carry.")
	case u.Status != "active":
		return refuse("The account is " + u.Status + ".")
	case app.Status != "active":
		return refuse("The application is disabled.")
	case !assigned(u, app):
		return refuse(notAssignedReason)
	}
	decision, err := h.policy.Evaluate(ctx, access.Input{User: u, App: &app, Method: "password", IP: ip})
	if err != nil {
		return "", err
	}
	event.ID, event.Risk = decision.EventID, decision.Risk
	_, unmet := decision.StepUp(nil, store.Session{Method: "password", CreatedAt: time.Now()})
	switch {
	case decision.Effect == access.Block || decision.Effect == access.RequirePhishingResistant || len(decision.Terms) > 0:
		return refuse(policyReason(decision))
	case unmet != nil:
		return refuse("The sign-in does not meet the authentication context “" + unmet.Name + "”.")
	}
	event.Result = "success"
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		if err := tx.RecordSignIn(ctx, event); err != nil {
			return err
		}
		if err := tx.TouchSignIn(ctx, u.ID, time.Now()); err != nil {
			return err
		}
		return tx.TouchPassword(ctx, u.ID)
	})
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"halo/internal/approval"
	"halo/internal/httpx"
	halomail "halo/internal/mail"
	"halo/internal/secret"
	"halo/internal/store"
)

var impersonationEnders = []string{"global_admin", "user_admin", "helpdesk_admin", "security_admin"}

func (h *handlers) impersonationRefusal(r *http.Request, actor, target store.User) error {
	if _, viaKey := httpx.KeyScopes(r); viaKey {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Impersonate people from the console, signed in as yourself. API keys can't.")
	}
	settings, err := h.st.Settings(r.Context())
	if err != nil {
		return err
	}
	if refusal := settings.ImpersonationRefusal(actor, target); refusal != "" {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", refusal)
	}
	return nil
}

func (h *handlers) requestImpersonation(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 500 {
		return httpx.Invalid("Say why you need to see Halo as " + target.Name + ", in at most 500 characters. They and other administrators can read it.")
	}
	if err := h.impersonationRefusal(r, actor, target); err != nil {
		return err
	}
	change := approval.Change{Action: "impersonation", Summary: "Impersonate " + target.Name + ": " + in.Reason, TargetType: "user", TargetID: target.ID, TargetLabel: target.Name,
		State: []any{target.Status, target.RoleKeys, target.EligibleRoles, target.GroupIDs}}
	if held, err := h.gate.Hold(w, r, actor, change); held || err != nil {
		return err
	}
	created, err := h.st.CreateImpersonation(r.Context(), actor.ID, target.ID, in.Reason, time.Now().Add(store.ImpersonationLifetime))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, created)
}

func (h *handlers) listImpersonations(w http.ResponseWriter, r *http.Request, _ store.User) error {
	list, err := h.st.ListImpersonations(r.Context(), r.URL.Query().Get("user"))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, list)
}

func (h *handlers) startImpersonation(w http.ResponseWriter, r *http.Request, actor store.User) error {
	ctx := r.Context()
	current, signedIn := httpx.CurrentSession(r)
	grant, err := h.st.GetImpersonation(ctx, r.PathValue("id"))
	if err != nil {
		return found(err, "This impersonation")
	}
	if !signedIn || grant.Actor.ID != actor.ID {
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only "+grant.Actor.Name+" can start this impersonation, from the console in the browser they asked from.")
	}
	target, err := h.st.GetUser(ctx, grant.User.ID)
	if err != nil {
		return err
	}
	if err := h.impersonationRefusal(r, actor, target); err != nil {
		return err
	}
	var device []byte
	if token := httpx.DeviceToken(r); token != "" {
		device = secret.Hash(token)
	}
	until := time.Now().Add(store.ImpersonationLifetime)
	var token string
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		_, token, err = tx.StartImpersonation(ctx, grant.ID, store.NewSession{UserID: target.ID, Method: current.Method, SecondFactor: current.SecondFactor, IP: httpx.ClientIP(r), UserAgent: r.UserAgent(), DeviceHash: device}, until)
		if errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "This impersonation already started, ended or expired. Ask again if you still need it.")
		}
		if err != nil {
			return err
		}
		if err := tx.RevokeSession(ctx, current.ID, actor.ID); err != nil {
			return err
		}
		_, os, browser := store.ParseUserAgent(r.UserAgent())
		err = tx.RecordSignIn(ctx, store.SignInEvent{Result: "success", Method: current.Method, UserID: &target.ID, Email: target.Email, IP: httpx.ClientIP(r), Device: browser + " · " + os,
			Reason: "Impersonated by " + actor.Name + ": " + grant.Reason, ImpersonatorID: &actor.ID})
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "impersonation.start", Summary: "Started impersonating " + target.Name + " until " + until.UTC().Format("15:04 UTC") + ": " + grant.Reason, TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	h.email(r, halomail.Message{To: target.Email, Subject: actor.Name + " is viewing Halo as you", Text: "Hi " + target.Name + ",\n\n" + actor.Name + " (" + actor.Email + ") started viewing your Halo account as you, to help with: " + grant.Reason +
		"\n\nThey can look but not change anything, and their access ends by " + until.UTC().Format("15:04 UTC") + ". If you did not expect this, tell your IT team, or end it now by signing out of your other sessions: " + h.cfg.Issuer() + "/account/sessions"})
	h.auth.SetSession(w, token)
	return httpx.JSON(w, http.StatusOK, map[string]string{"redirect": "/account"})
}

func (h *handlers) endImpersonation(w http.ResponseWriter, r *http.Request) error {
	u, current, err := httpx.RequireUser(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	grant, err := h.st.GetImpersonation(ctx, r.PathValue("id"))
	if err != nil {
		return found(err, "This impersonation")
	}
	self := current.ImpersonationID == grant.ID
	if !self && !u.HasRole(impersonationEnders...) {
		return httpx.ErrForbidden
	}
	actor, summary := u, "Ended "+grant.Actor.Name+"'s impersonation of "+grant.User.Name
	if self {
		actor, summary = store.User{ID: grant.Actor.ID}, "Ended impersonating "+grant.User.Name
	}
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		ended, err := tx.EndImpersonation(ctx, grant.ID)
		if err != nil || !ended {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "impersonation.end", Summary: summary, TargetType: "user", TargetID: grant.User.ID, TargetLabel: grant.User.Name})
	})
	if err != nil {
		return err
	}
	if self {
		h.auth.ClearSession(w)
	}
	return noContent(w)
}

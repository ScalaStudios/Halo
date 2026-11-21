package auth

import (
	"context"
	"errors"
	"net/http"

	"halo/internal/httpx"
	"halo/internal/proofing"
	"halo/internal/store"
)

var (
	errProofRequired = httpx.Fail(http.StatusForbidden, "ERR_IDENTITY_CHECK_REQUIRED", "Your organization checks who you are before your account is activated. Finish the identity check on your setup page, or ask your administrator to check your identity in person.")
	errProofNotAsked = httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "This link doesn't need an identity check, or yours is already confirmed. Create your passkey.")
)

type proofState struct {
	Required bool   `json:"required"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
}

func (h *handler) proofState(ctx context.Context, settings store.Settings, required bool, userID, signUpID string) (proofState, error) {
	state := proofState{Required: required, Provider: proofing.Name(settings)}
	p, err := h.st.IdentityCheck(ctx, userID, signUpID)
	if errors.Is(err, store.ErrNotFound) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if p, err = proofing.Refresh(ctx, h.st, p); err != nil {
		return state, err
	}
	state.Status = p.Status
	return state, nil
}

func (h *handler) unproven(ctx context.Context, u store.User) (bool, error) {
	settings, err := h.st.Settings(ctx)
	if err != nil || u.Status != "invited" || !settings.ProofingRequired(u.GroupIDs, false) {
		return false, err
	}
	p, err := h.st.IdentityCheck(ctx, u.ID, "")
	if errors.Is(err, store.ErrNotFound) {
		return true, nil
	}
	return p.Status != "verified", err
}

func (h *handler) enrollProofState(ctx context.Context, u store.User) (proofState, error) {
	settings, err := h.st.Settings(ctx)
	if err != nil {
		return proofState{}, err
	}
	return h.proofState(ctx, settings, u.Status == "invited" && settings.ProofingRequired(u.GroupIDs, false), u.ID, "")
}

func (h *handler) enrollProof(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	u, _, err := h.enrollee(r, in.Token)
	if err != nil {
		return err
	}
	if missing, err := h.unproven(r.Context(), u); err != nil {
		return err
	} else if !missing {
		return errProofNotAsked
	}
	link, err := proofing.Begin(r.Context(), h.st, proofing.Subject{UserID: u.ID, Name: u.Name}, h.cfg.Issuer()+"/enroll?proofing=done", httpx.ClientIP(r))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"url": link})
}

func (h *handler) signUpProof(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	su, settings, err := h.pendingSignUp(r, in.Token)
	if err != nil {
		return err
	}
	if !settings.ProofingRequired(settings.SignUpGroupIDs, true) {
		return errProofNotAsked
	}
	if p, err := h.st.IdentityCheck(r.Context(), "", su.ID); err == nil && p.Status == "verified" {
		return errProofNotAsked
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	link, err := proofing.Begin(r.Context(), h.st, proofing.Subject{SignUpID: su.ID, Name: su.Name}, h.cfg.Issuer()+"/sign-up?proofing=done", httpx.ClientIP(r))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"url": link})
}

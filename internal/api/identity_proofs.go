package api

import (
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	documents = map[string]string{
		"passport":         "a passport",
		"national-id":      "a national identity card",
		"driving-licence":  "a driving licence",
		"residence-permit": "a residence permit",
		"other":            "another official document",
	}
	documentNumber = regexp.MustCompile(`[0-9]{5,}`)
)

type inPersonCheck struct {
	Document string `json:"document"`
	Note     string `json:"note"`
}

func (in *inPersonCheck) validate() error {
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case documents[in.Document] == "":
		return httpx.Invalid("Choose the document you checked: passport, national-id, driving-licence, residence-permit or other.")
	case in.Note == "" || utf8.RuneCountInString(in.Note) > 500:
		return httpx.Invalid("Describe how you checked their identity in up to 500 characters, for example where you met and what you compared.")
	case documentNumber.MatchString(in.Note):
		return httpx.Invalid("Leave the document number out of the note. Halo records which kind of document you checked, not its number.")
	}
	return nil
}

func (h *handlers) userIdentityProofs(w http.ResponseWriter, r *http.Request, _ store.User) error {
	u, err := h.user(r)
	if err != nil {
		return err
	}
	list, err := h.st.ListIdentityProofs(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, nonNil(list))
}

func (h *handlers) checkIdentity(w http.ResponseWriter, r *http.Request, actor store.User) error {
	target, err := h.user(r)
	if err != nil {
		return err
	}
	if target.ID == actor.ID {
		return httpx.Invalid("You can't confirm your own identity. Ask another administrator to check you in person.")
	}
	if err := guardAdmin(actor, target); err != nil {
		return err
	}
	if target.Kind != "person" || target.Status == "deprovisioned" {
		return conflict(target.Name + " is deprovisioned or is not a person, so there is no identity to confirm.")
	}
	var in inPersonCheck
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	var p store.IdentityProof
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if p, err = tx.CreateIdentityProof(r.Context(), store.NewIdentityProof{UserID: target.ID, Method: "in-person", Status: "verified", Document: in.Document, Note: in.Note, CheckedBy: &actor.ID}); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.identity_proof", Summary: "Checked the identity of " + target.Name + " in person with " + documents[in.Document], TargetType: "user", TargetID: target.ID, TargetLabel: target.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, p)
}

func (h *handlers) checkSignUpIdentity(w http.ResponseWriter, r *http.Request, actor store.User) error {
	su, err := h.st.GetSignUp(r.Context(), r.PathValue("id"))
	if err != nil {
		return found(err, "This sign-up")
	}
	var in inPersonCheck
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	var p store.IdentityProof
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		var err error
		if p, err = tx.CreateIdentityProof(r.Context(), store.NewIdentityProof{SignUpID: su.ID, Method: "in-person", Status: "verified", Document: in.Document, Note: in.Note, CheckedBy: &actor.ID}); err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "user.identity_proof", Summary: "Checked the identity of " + su.Name + " <" + su.Email + "> in person with " + documents[in.Document], TargetType: "sign_up", TargetID: su.ID, TargetLabel: su.Name})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, p)
}

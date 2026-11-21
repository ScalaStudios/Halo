package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

const guestMaxDays = 365

func (h *handlers) guestSponsor(ctx context.Context, sponsorID, guestID string) (store.User, error) {
	if sponsorID == "" {
		return store.User{}, httpx.Invalid("Choose a sponsor: the person at your organization who is responsible for this guest and extends their access.")
	}
	sponsor, err := h.st.GetUser(ctx, sponsorID)
	if errors.Is(err, store.ErrNotFound) || err == nil && (sponsor.ID == guestID || !sponsor.CanSponsor()) {
		return store.User{}, httpx.Invalid("A sponsor must be an active person at your organization, not a guest or service account. Pick someone else from the directory.")
	}
	return sponsor, err
}

func guestExpiry(at *time.Time) (time.Time, error) {
	switch {
	case at == nil:
		return time.Time{}, httpx.Invalid("Choose when the guest's access ends. Halo suspends the account then unless the sponsor extends it.")
	case !at.After(time.Now()):
		return time.Time{}, httpx.Invalid("Choose an end date for the guest's access in the future.")
	case at.After(time.Now().AddDate(0, 0, guestMaxDays)):
		return time.Time{}, httpx.Invalid("Choose an end date within the next 365 days. The sponsor can extend it again before it ends.")
	}
	return at.Truncate(time.Second), nil
}

func (h *handlers) updateGuest(w http.ResponseWriter, r *http.Request, actor store.User) error {
	guest, err := h.user(r)
	if err != nil {
		return err
	}
	admin := actor.Manages(guest, "user_admin")
	if !guest.Guest || !admin && (guest.Sponsor == nil || guest.Sponsor.ID != actor.ID) {
		return httpx.NotFound("This guest")
	}
	var in struct {
		SponsorID *string    `json:"sponsorId"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	switch {
	case in.SponsorID == nil && in.ExpiresAt == nil:
		return httpx.Invalid("Send expiresAt to change when the guest's access ends, sponsorId to change their sponsor, or both.")
	case in.SponsorID != nil && !admin:
		return httpx.Fail(http.StatusForbidden, "ERR_FORBIDDEN", "Only a user administrator can change who sponsors "+guest.Name+".")
	case guest.Expired() && !admin:
		return conflict(guest.Name + "'s access already ended, so only a user administrator can extend it and restore the account.")
	}
	sponsorID := ""
	if guest.Sponsor != nil {
		sponsorID = guest.Sponsor.ID
	}
	if in.SponsorID != nil {
		sponsorID = *in.SponsorID
	}
	var sponsor store.User
	newSponsor := guest.Sponsor == nil || sponsorID != guest.Sponsor.ID
	if newSponsor {
		if sponsor, err = h.guestSponsor(r.Context(), sponsorID, guest.ID); err != nil {
			return err
		}
	}
	expires := *guest.ExpiresAt
	if in.ExpiresAt != nil {
		if expires, err = guestExpiry(in.ExpiresAt); err != nil {
			return err
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.SetGuestAccess(r.Context(), guest.ID, sponsorID, expires); err != nil {
			return err
		}
		if !expires.Equal(*guest.ExpiresAt) {
			event := store.AuditEvent{Action: "guest.extend", Summary: "Set guest access to end " + expires.UTC().Format("2 Jan 2006, 15:04 UTC"), TargetType: "user", TargetID: guest.ID, TargetLabel: guest.Name,
				Changes: store.Diff(map[string]time.Time{"expiresAt": *guest.ExpiresAt}, map[string]time.Time{"expiresAt": expires})}
			if err := record(r, tx, actor, event); err != nil {
				return err
			}
		}
		if !newSponsor {
			return nil
		}
		return record(r, tx, actor, store.AuditEvent{Action: "guest.sponsor.change", Summary: "Changed the sponsor to " + sponsor.Name, TargetType: "user", TargetID: guest.ID, TargetLabel: guest.Name})
	})
	if err != nil {
		return err
	}
	updated, err := h.st.GetUser(r.Context(), guest.ID)
	if err != nil {
		return err
	}
	if !administrator(actor) {
		updated.Attributes = nil
	}
	return httpx.JSON(w, http.StatusOK, updated)
}

func (h *handlers) mySponsoredGuests(w http.ResponseWriter, r *http.Request, actor store.User) error {
	guests, err := h.st.ListSponsoredGuests(r.Context(), actor.ID)
	if err != nil {
		return err
	}
	for i := range guests {
		guests[i].Attributes = nil
	}
	return httpx.JSON(w, http.StatusOK, nonNil(guests))
}

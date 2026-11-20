package governance

import (
	"context"
	"errors"
	"time"

	"halo/internal/mail"
	"halo/internal/store"
)

const guestNoticeWindow = 14 * 24 * time.Hour

func expireGuests(ctx context.Context, st *store.Store, now time.Time) error {
	return st.Tx(ctx, func(tx *store.Store) error {
		expired, err := tx.ClaimExpiredGuests(ctx, now)
		if err != nil {
			return err
		}
		for _, g := range expired {
			n, err := tx.RevokeUserSessions(ctx, g.Guest.ID, "")
			if err != nil {
				return err
			}
			summary := "Suspended the guest account because its access ended " + formatTime(g.ExpiresAt) + ", and ended " + plural(n, "session")
			if err := tx.RecordAudit(ctx, store.AuditEvent{Action: "guest.expire", Summary: summary, TargetType: "user", TargetID: g.Guest.ID, TargetLabel: g.Guest.Name}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (h *handler) noticeGuestExpiry(ctx context.Context, now time.Time) error {
	due, err := h.st.ClaimGuestExpiryNotices(ctx, now, now.Add(guestNoticeWindow))
	if err != nil {
		return err
	}
	var errs []error
	for _, g := range due {
		to, link, err := h.guestContacts(ctx, g)
		errs = append(errs, err)
		for _, address := range to {
			h.send(ctx, mail.GuestExpiring(address, g.Guest.Name, g.Guest.Email, link, g.ExpiresAt))
		}
	}
	return errors.Join(errs...)
}

func (h *handler) guestContacts(ctx context.Context, g store.GuestExpiry) ([]string, string, error) {
	if g.SponsorID != nil {
		sponsor, err := h.st.GetUser(ctx, *g.SponsorID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, "", err
		}
		if err == nil && sponsor.CanSponsor() {
			return []string{sponsor.Email}, h.cfg.Issuer() + "/account/access", nil
		}
	}
	admins, err := h.st.GlobalAdminEmails(ctx, "")
	return admins, h.cfg.Issuer() + "/admin/users/" + g.Guest.ID, err
}

package api

import (
	"crypto/x509"
	"net/http"
	"slices"
	"time"

	"halo/internal/httpx"
	"halo/internal/oidc"
	"halo/internal/saml"
	"halo/internal/store"
)

type samlCertificate struct {
	ID          string     `json:"id"`
	Fingerprint string     `json:"fingerprint"`
	NotAfter    time.Time  `json:"notAfter"`
	CreatedAt   time.Time  `json:"createdAt"`
	RetiredAt   *time.Time `json:"retiredAt"`
}

func (h *handlers) listSigningKeys(w http.ResponseWriter, r *http.Request, _ store.User) error {
	keys, err := h.st.ListSigningKeyInfo(r.Context())
	if err != nil {
		return err
	}
	samlKeys, err := h.st.ListSAMLKeys(r.Context())
	if err != nil {
		return err
	}
	certificates := []samlCertificate{}
	for _, k := range samlKeys {
		cert, err := x509.ParseCertificate(k.Certificate)
		if err != nil {
			return err
		}
		certificates = append(certificates, samlCertificate{k.ID, saml.Fingerprint(cert.Raw), cert.NotAfter, k.CreatedAt, k.RetiredAt})
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"oidc": nonNil(keys), "saml": certificates})
}

func (h *handlers) rotateSigningKey(w http.ResponseWriter, r *http.Request, actor store.User) error {
	keys, err := h.st.ListSigningKeyInfo(r.Context())
	if err != nil {
		return err
	}
	algorithms := []string{"RS256"}
	for _, k := range keys {
		if k.Active && !slices.Contains(algorithms, k.Algorithm) {
			algorithms = append(algorithms, k.Algorithm)
		}
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		for _, alg := range algorithms {
			der, err := oidc.NewSigningKey(alg)
			if err != nil {
				return err
			}
			key, err := tx.CreateSigningKey(r.Context(), alg, der)
			if err != nil {
				return err
			}
			if err := record(r, tx, actor, store.AuditEvent{Action: "signing_key.rotate", Summary: "Rotated the OpenID Connect " + alg + " signing key. Previous keys stay published for 7 days", TargetType: "signing_key", TargetID: key.ID, TargetLabel: key.ID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return h.listSigningKeys(w, r, actor)
}

func (h *handlers) rotateSAMLCertificate(w http.ResponseWriter, r *http.Request, actor store.User) error {
	der, cert, err := saml.NewKey(h.cfg.Hostname())
	if err != nil {
		return err
	}
	err = h.st.Tx(r.Context(), func(tx *store.Store) error {
		keyID, err := tx.RotateSAMLKey(r.Context(), der, cert)
		if err != nil {
			return err
		}
		return record(r, tx, actor, store.AuditEvent{Action: "saml_certificate.rotate", Summary: "Rotated the SAML signing certificate. SAML applications need the new certificate", TargetType: "saml_certificate", TargetID: keyID, TargetLabel: saml.Fingerprint(cert)})
	})
	if err != nil {
		return err
	}
	return h.listSigningKeys(w, r, actor)
}

func (h *handlers) deleteSAMLCertificate(w http.ResponseWriter, r *http.Request, actor store.User) error {
	keyID := r.PathValue("id")
	err := h.st.Tx(r.Context(), func(tx *store.Store) error {
		if err := tx.DeleteSAMLKey(r.Context(), keyID); err != nil {
			return found(err, "This previous SAML certificate")
		}
		return record(r, tx, actor, store.AuditEvent{Action: "saml_certificate.delete", Summary: "Removed a previous SAML signing certificate", TargetType: "saml_certificate", TargetID: keyID, TargetLabel: keyID})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

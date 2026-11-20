package auth

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"
	"time"

	"halo/internal/httpx"
	"halo/internal/store"
)

var errBundle = httpx.Invalid("Paste the authority's certificate in PEM format, starting with -----BEGIN CERTIFICATE-----. Add more issuing authorities one after another.")

type certificateSummary struct {
	Subject   string    `json:"subject"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type authorityView struct {
	store.CertificateAuthority
	Chain []certificateSummary `json:"certificates"`
}

func parseBundle(bundle string) ([]*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	rest := []byte(bundle)
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		cert, err := x509.ParseCertificate(block.Bytes)
		if block.Type != "CERTIFICATE" || err != nil {
			return nil, errBundle
		}
		if !cert.IsCA {
			return nil, httpx.Invalid(cert.Subject.String() + " isn't a certificate authority. Upload the certificate of the authority that issues your client certificates, not a client certificate.")
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 || strings.TrimSpace(string(rest)) != "" {
		return nil, errBundle
	}
	return certs, nil
}

func viewAuthority(ca store.CertificateAuthority) authorityView {
	certs, _ := parseBundle(ca.Certificates)
	v := authorityView{CertificateAuthority: ca, Chain: []certificateSummary{}}
	for _, c := range certs {
		v.Chain = append(v.Chain, certificateSummary{Subject: c.Subject.String(), ExpiresAt: c.NotAfter})
	}
	return v
}

func (h *handler) listCertificateAuthorities(w http.ResponseWriter, r *http.Request) error {
	if _, err := httpx.RequireRole(r, providerReaders...); err != nil {
		return err
	}
	cas, err := h.st.ListCertificateAuthorities(r.Context())
	if err != nil {
		return err
	}
	views := make([]authorityView, len(cas))
	for i, ca := range cas {
		views[i] = viewAuthority(ca)
	}
	return httpx.JSON(w, http.StatusOK, views)
}

func (h *handler) createCertificateAuthority(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	var in struct {
		Name         string `json:"name"`
		Mapping      string `json:"mapping"`
		Certificates string `json:"certificates"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return httpx.Invalid("Enter a name for the certificate authority, for example “Corporate PKI”.")
	case len(name) > 200:
		return httpx.Invalid("Keep the certificate authority name under 200 characters.")
	case mappingFields[in.Mapping] == "":
		return httpx.Invalid(`Choose how certificates match people: "email", "upn" or "subject".`)
	}
	certs, err := parseBundle(in.Certificates)
	if err != nil {
		return err
	}
	var bundle []byte
	for _, c := range certs {
		if time.Now().After(c.NotAfter) {
			return httpx.Invalid(c.Subject.String() + " expired on " + c.NotAfter.UTC().Format(time.DateOnly) + ". Upload the authority's current certificate.")
		}
		bundle = append(bundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	ctx := r.Context()
	var ca store.CertificateAuthority
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		var err error
		ca, err = tx.CreateCertificateAuthority(ctx, store.CertificateAuthority{Name: name, Mapping: in.Mapping, Certificates: string(bundle)})
		if errors.Is(err, store.ErrConflict) {
			return httpx.Fail(http.StatusConflict, "ERR_CONFLICT", "A certificate authority named "+name+" already exists. Pick a different name.")
		}
		if err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "certificate_authority.create", Summary: "Trusted a certificate authority for client certificate sign-in, matching each certificate's " + mappingFields[ca.Mapping] + " to a person's email address", TargetType: "certificate_authority", TargetID: ca.ID, TargetLabel: ca.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, viewAuthority(ca))
}

func (h *handler) deleteCertificateAuthority(w http.ResponseWriter, r *http.Request) error {
	actor, err := httpx.RequireRole(r, "security_admin")
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = h.st.Tx(ctx, func(tx *store.Store) error {
		ca, err := tx.DeleteCertificateAuthority(ctx, r.PathValue("id"))
		if errors.Is(err, store.ErrNotFound) {
			return httpx.NotFound("This certificate authority")
		}
		if err != nil {
			return err
		}
		return tx.RecordAudit(ctx, store.AuditEvent{ActorID: &actor.ID, Action: "certificate_authority.delete", Summary: "Stopped trusting a certificate authority for client certificate sign-in", TargetType: "certificate_authority", TargetID: ca.ID, TargetLabel: ca.Name, IP: httpx.ClientIP(r)})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

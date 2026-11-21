package auth

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
	"golang.org/x/crypto/ocsp"

	"halo/internal/httpx"
	"halo/internal/store"
)

var (
	errCertificateMissing    = httpx.Fail(http.StatusUnauthorized, "ERR_CERTIFICATE_MISSING", "Your browser didn't send a client certificate. Insert your smart card or install your certificate, reload the page and try again.")
	errCertificateRejected   = httpx.Fail(http.StatusUnauthorized, "ERR_CERTIFICATE_REJECTED", "Halo couldn't sign you in with that certificate. Check that it hasn't expired, or sign in another way.")
	errRevocationUnavailable = httpx.Fail(http.StatusServiceUnavailable, "ERR_REVOCATION_UNAVAILABLE", "Halo couldn't check whether your certificate was revoked. Try again in a few minutes, or sign in another way.")

	revocationClient = &http.Client{Timeout: 5 * time.Second}
	oidSubjectAlt    = asn1.ObjectIdentifier{2, 5, 29, 17}
	oidPrincipalName = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 20, 2, 3}
	otherNameTag     = cbasn1.Tag(0).ContextSpecific().Constructed()
	mappingFields    = map[string]string{"email": "email address", "upn": "user principal name", "subject": "subject common name"}
)

func (h *handler) certificateSignIn(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AuthRequest string `json:"authRequest"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	app, err := h.target(ctx, in.AuthRequest)
	if err != nil {
		return err
	}
	a := attempt{method: "certificate", app: app, request: in.AuthRequest}
	if h.cfg.CertHeader == "" {
		return h.reject(r, a, "HALO_CLIENT_CERT_HEADER isn't set, so Halo reads no client certificates.", errCertificateMissing)
	}
	header := strings.TrimSpace(httpx.ProxyHeader(r, h.cfg.CertHeader))
	if header == "" || header == "::" {
		return h.reject(r, a, "The browser sent no client certificate.", errCertificateMissing)
	}
	cert, err := parseClientCertificate(header)
	if err != nil {
		return h.reject(r, a, "The client certificate could not be read.", errCertificateRejected)
	}
	if now := time.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return h.reject(r, a, fmt.Sprintf("The certificate is valid from %s to %s.", cert.NotBefore.UTC().Format(time.DateOnly), cert.NotAfter.UTC().Format(time.DateOnly)), errCertificateRejected)
	}
	cas, err := h.st.ListCertificateAuthorities(ctx)
	if err != nil {
		return err
	}
	ca, chain := trustedIssuer(cas, cert)
	if ca == nil {
		return h.reject(r, a, "No trusted certificate authority issued the certificate.", errCertificateRejected)
	}
	if a.email = mappedName(cert, ca.Mapping); a.email == "" {
		return h.reject(r, a, "The certificate has no "+mappingFields[ca.Mapping]+" to match a person.", errCertificateRejected)
	}
	if len(chain) > 1 {
		revoked, err := revocation(ctx, cert, chain[1])
		if err != nil {
			slog.WarnContext(ctx, "certificate revocation not checked", "authority", ca.Name, "error", err)
			return h.reject(r, a, "The certificate's revocation status could not be checked.", errRevocationUnavailable)
		}
		if revoked {
			return h.reject(r, a, "“"+ca.Name+"” revoked the certificate.", errCertificateRejected)
		}
	}
	u, err := h.st.GetUserByEmail(ctx, a.email)
	if errors.Is(err, store.ErrNotFound) || err == nil && u.Kind == "service" {
		return h.reject(r, a, "No person's email address matches the certificate's "+mappingFields[ca.Mapping]+".", errCertificateRejected)
	}
	if err != nil {
		return err
	}
	a.user = &u
	return h.finish(w, r, a, nil, nil)
}

func parseClientCertificate(header string) (*x509.Certificate, error) {
	if encoded, ok := strings.CutPrefix(header, ":"); ok {
		der, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(encoded, ":"))
		if err != nil {
			return nil, err
		}
		return x509.ParseCertificate(der)
	}
	decoded, err := url.PathUnescape(header)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(decoded))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func trustedIssuer(cas []store.CertificateAuthority, cert *x509.Certificate) (*store.CertificateAuthority, []*x509.Certificate) {
	for i, ca := range cas {
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM([]byte(ca.Certificates))
		if chains, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
			return &cas[i], chains[0]
		}
	}
	return nil, nil
}

func mappedName(cert *x509.Certificate, mapping string) string {
	switch mapping {
	case "email":
		if len(cert.EmailAddresses) > 0 {
			return cert.EmailAddresses[0]
		}
	case "upn":
		return principalName(cert)
	case "subject":
		return cert.Subject.CommonName
	}
	return ""
}

func principalName(cert *x509.Certificate) string {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidSubjectAlt) {
			continue
		}
		value, names := cryptobyte.String(ext.Value), cryptobyte.String(nil)
		if !value.ReadASN1(&names, cbasn1.SEQUENCE) {
			return ""
		}
		for !names.Empty() {
			var name, inner, upn cryptobyte.String
			var tag cbasn1.Tag
			var oid asn1.ObjectIdentifier
			if !names.ReadAnyASN1(&name, &tag) {
				return ""
			}
			if tag == otherNameTag && name.ReadASN1ObjectIdentifier(&oid) && oid.Equal(oidPrincipalName) && name.ReadASN1(&inner, otherNameTag) && inner.ReadASN1(&upn, cbasn1.UTF8String) {
				return string(upn)
			}
		}
	}
	return ""
}

func revocation(ctx context.Context, cert, issuer *x509.Certificate) (bool, error) {
	if responder := web(cert.OCSPServer); responder != "" {
		request, err := ocsp.CreateRequest(cert, issuer, nil)
		if err != nil {
			return false, err
		}
		body, err := fetch(ctx, responder, request)
		if err != nil {
			return false, err
		}
		res, err := ocsp.ParseResponseForCert(body, cert, issuer)
		if err != nil {
			return false, err
		}
		if !res.NextUpdate.IsZero() && time.Now().After(res.NextUpdate) {
			return false, errors.New("the OCSP response is out of date")
		}
		return res.Status != ocsp.Good, nil
	}
	if point := web(cert.CRLDistributionPoints); point != "" {
		body, err := fetch(ctx, point, nil)
		if err != nil {
			return false, err
		}
		list, err := x509.ParseRevocationList(body)
		if err != nil {
			return false, err
		}
		if err := list.CheckSignatureFrom(issuer); err != nil {
			return false, err
		}
		if !list.NextUpdate.IsZero() && time.Now().After(list.NextUpdate) {
			return false, errors.New("the CRL is out of date")
		}
		return slices.ContainsFunc(list.RevokedCertificateEntries, func(e x509.RevocationListEntry) bool { return e.SerialNumber.Cmp(cert.SerialNumber) == 0 }), nil
	}
	if len(cert.OCSPServer) > 0 || len(cert.CRLDistributionPoints) > 0 {
		return false, errors.New("the certificate names no OCSP responder or CRL over HTTP")
	}
	return false, nil
}

func web(urls []string) string {
	if i := slices.IndexFunc(urls, func(u string) bool { return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") }); i >= 0 {
		return urls[i]
	}
	return ""
}

func fetch(ctx context.Context, target string, ocspRequest []byte) ([]byte, error) {
	method := http.MethodGet
	if ocspRequest != nil {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(ocspRequest))
	if err != nil {
		return nil, err
	}
	if ocspRequest != nil {
		req.Header.Set("Content-Type", "application/ocsp-request")
	}
	res, err := revocationClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", target, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

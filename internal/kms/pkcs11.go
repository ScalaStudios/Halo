//go:build pkcs11

package kms

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/miekg/pkcs11"

	"halo/internal/config"
	"halo/internal/id"
)

const (
	ckmECEdwardsKeyPairGen = 0x1055
	ckmEdDSA               = 0x1057
	ckkECEdwards           = 0x40
)

var (
	oidP256    = []byte{0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07}
	oidEd25519 = []byte{0x06, 0x03, 0x2b, 0x65, 0x70}
	pkcs11Ref  = regexp.MustCompile(`^pkcs11:object=(halo-key_[0-9A-Z]+)$`)
	digestInfo = map[crypto.Hash][]byte{
		crypto.SHA256: {0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20},
		crypto.SHA384: {0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30},
		crypto.SHA512: {0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40},
	}
	lostSession = []pkcs11.Error{pkcs11.CKR_SESSION_HANDLE_INVALID, pkcs11.CKR_SESSION_CLOSED, pkcs11.CKR_USER_NOT_LOGGED_IN, pkcs11.CKR_DEVICE_REMOVED, pkcs11.CKR_DEVICE_ERROR, pkcs11.CKR_TOKEN_NOT_PRESENT, pkcs11.CKR_CRYPTOKI_NOT_INITIALIZED}
)

type PKCS11 struct {
	cfg         config.PKCS11
	wrappingKey string
	mu          sync.Mutex
	module      *pkcs11.Ctx
	session     pkcs11.SessionHandle
}

func NewPKCS11(cfg config.KMS) Provider {
	return &PKCS11{cfg: cfg.PKCS11, wrappingKey: cfg.WrappingKey}
}

func (p *PKCS11) UnwrapKey(ctx context.Context, wrapped string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(wrapped)
	if err != nil {
		return nil, errors.New("the wrapped key is not base64")
	}
	var plain []byte
	err = p.with("decrypt", func(m *pkcs11.Ctx, s pkcs11.SessionHandle) error {
		key, err := find(m, s, pkcs11.CKO_PRIVATE_KEY, p.wrappingKey)
		if err != nil {
			return err
		}
		oaep := pkcs11.NewMechanism(pkcs11.CKM_RSA_PKCS_OAEP, pkcs11.NewOAEPParams(pkcs11.CKM_SHA256, pkcs11.CKG_MGF1_SHA256, pkcs11.CKZ_DATA_SPECIFIED, nil))
		if err := m.DecryptInit(s, []*pkcs11.Mechanism{oaep}, key); err != nil {
			return err
		}
		plain, err = m.Decrypt(s, raw)
		return err
	})
	return plain, err
}

func (p *PKCS11) CreateKey(ctx context.Context, algorithm string) (string, error) {
	label := "halo-" + id.New("key")
	public := []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PUBLIC_KEY), pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true), pkcs11.NewAttribute(pkcs11.CKA_LABEL, label), pkcs11.NewAttribute(pkcs11.CKA_ID, label), pkcs11.NewAttribute(pkcs11.CKA_VERIFY, true)}
	private := []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY), pkcs11.NewAttribute(pkcs11.CKA_TOKEN, true), pkcs11.NewAttribute(pkcs11.CKA_LABEL, label), pkcs11.NewAttribute(pkcs11.CKA_ID, label),
		pkcs11.NewAttribute(pkcs11.CKA_SIGN, true), pkcs11.NewAttribute(pkcs11.CKA_PRIVATE, true), pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, true), pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, false)}
	var mechanism uint
	switch algorithm {
	case "RS256":
		mechanism = pkcs11.CKM_RSA_PKCS_KEY_PAIR_GEN
		public = append(public, pkcs11.NewAttribute(pkcs11.CKA_MODULUS_BITS, 2048), pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, []byte{1, 0, 1}))
	case "ES256":
		mechanism = pkcs11.CKM_EC_KEY_PAIR_GEN
		public = append(public, pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, oidP256))
	case "EdDSA":
		mechanism = ckmECEdwardsKeyPairGen
		public = append(public, pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, oidEd25519))
	default:
		return "", fmt.Errorf("Halo has no PKCS #11 key type for %s", algorithm)
	}
	err := p.with("create", func(m *pkcs11.Ctx, s pkcs11.SessionHandle) error {
		_, _, err := m.GenerateKeyPair(s, []*pkcs11.Mechanism{pkcs11.NewMechanism(mechanism, nil)}, public, private)
		return err
	})
	return "pkcs11:object=" + label, err
}

func (p *PKCS11) Signer(ctx context.Context, ref string) (crypto.Signer, error) {
	match := pkcs11Ref.FindStringSubmatch(ref)
	if match == nil {
		return nil, fmt.Errorf("%q is not a PKCS #11 key that Halo created", ref)
	}
	label := match[1]
	var public crypto.PublicKey
	err := p.with("read", func(m *pkcs11.Ctx, s pkcs11.SessionHandle) error {
		private, err := find(m, s, pkcs11.CKO_PRIVATE_KEY, label)
		if err != nil {
			return err
		}
		flags, err := m.GetAttributeValue(s, private, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, nil), pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, nil), pkcs11.NewAttribute(pkcs11.CKA_SIGN, nil)})
		if err != nil {
			return err
		}
		if !truthy(flags[0]) || truthy(flags[1]) || !truthy(flags[2]) {
			return fmt.Errorf("the token lets key %s be extracted or not sign, so Halo won't sign with it", label)
		}
		handle, err := find(m, s, pkcs11.CKO_PUBLIC_KEY, label)
		if err != nil {
			return err
		}
		attributes, err := m.GetAttributeValue(s, handle, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, nil)})
		if err != nil {
			return err
		}
		public, err = publicFromToken(m, s, handle, attributes[0].Value)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &signer{public: public, sign: func(digest []byte, hash crypto.Hash) ([]byte, error) {
		mechanism, data := uint(ckmEdDSA), digest
		switch public.(type) {
		case *rsa.PublicKey:
			prefix, ok := digestInfo[hash]
			if !ok {
				return nil, fmt.Errorf("Halo can't sign a %s digest with PKCS #11", hash)
			}
			mechanism, data = pkcs11.CKM_RSA_PKCS, append(slices.Clone(prefix), digest...)
		case *ecdsa.PublicKey:
			mechanism = pkcs11.CKM_ECDSA
		}
		var signature []byte
		err := p.with("sign", func(m *pkcs11.Ctx, s pkcs11.SessionHandle) error {
			key, err := find(m, s, pkcs11.CKO_PRIVATE_KEY, label)
			if err != nil {
				return err
			}
			if err := m.SignInit(s, []*pkcs11.Mechanism{pkcs11.NewMechanism(mechanism, nil)}, key); err != nil {
				return err
			}
			signature, err = m.Sign(s, data)
			return err
		})
		if err != nil {
			return nil, err
		}
		if mechanism == pkcs11.CKM_ECDSA {
			return asn1Signature(signature)
		}
		return signature, nil
	}}, nil
}

func (p *PKCS11) Ping(ctx context.Context) error {
	return p.with("ping", func(m *pkcs11.Ctx, s pkcs11.SessionHandle) error {
		_, err := m.GetSessionInfo(s)
		return err
	})
}

func (p *PKCS11) with(operation string, fn func(*pkcs11.Ctx, pkcs11.SessionHandle) error) error {
	return observe(operation, "pkcs #11", func() error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.module == nil {
			if err := p.open(); err != nil {
				return err
			}
		}
		err := fn(p.module, p.session)
		var code pkcs11.Error
		if errors.As(err, &code) && slices.Contains(lostSession, code) {
			_ = p.module.CloseSession(p.session)
			_ = p.module.Finalize()
			p.module.Destroy()
			p.module = nil
		}
		return err
	})
}

func (p *PKCS11) open() error {
	module := pkcs11.New(p.cfg.Module)
	if module == nil {
		return fmt.Errorf("Halo couldn't load the PKCS #11 module %s", p.cfg.Module)
	}
	if err := module.Initialize(); err != nil && !errors.Is(err, pkcs11.Error(pkcs11.CKR_CRYPTOKI_ALREADY_INITIALIZED)) {
		module.Destroy()
		return err
	}
	session, err := p.login(module)
	if err != nil {
		_ = module.Finalize()
		module.Destroy()
		return err
	}
	p.module, p.session = module, session
	return nil
}

func (p *PKCS11) login(module *pkcs11.Ctx) (pkcs11.SessionHandle, error) {
	slots, err := module.GetSlotList(true)
	if err != nil {
		return 0, err
	}
	for _, slot := range slots {
		info, err := module.GetTokenInfo(slot)
		if err != nil || strings.TrimRight(info.Label, " \x00") != p.cfg.TokenLabel {
			continue
		}
		session, err := module.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
		if err != nil {
			return 0, err
		}
		if err := module.Login(session, pkcs11.CKU_USER, p.cfg.PIN); err != nil && !errors.Is(err, pkcs11.Error(pkcs11.CKR_USER_ALREADY_LOGGED_IN)) {
			_ = module.CloseSession(session)
			return 0, err
		}
		return session, nil
	}
	return 0, fmt.Errorf("no PKCS #11 token is labelled %q", p.cfg.TokenLabel)
}

func find(m *pkcs11.Ctx, s pkcs11.SessionHandle, class uint, label string) (pkcs11.ObjectHandle, error) {
	if err := m.FindObjectsInit(s, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_CLASS, class), pkcs11.NewAttribute(pkcs11.CKA_LABEL, label)}); err != nil {
		return 0, err
	}
	objects, _, err := m.FindObjects(s, 2)
	if final := m.FindObjectsFinal(s); err == nil {
		err = final
	}
	if err != nil {
		return 0, err
	}
	if len(objects) != 1 {
		return 0, fmt.Errorf("the token holds %d keys labelled %s, not one", len(objects), label)
	}
	return objects[0], nil
}

func publicFromToken(m *pkcs11.Ctx, s pkcs11.SessionHandle, handle pkcs11.ObjectHandle, keyType []byte) (crypto.PublicKey, error) {
	is := func(kind uint) bool {
		return bytes.Equal(keyType, pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, kind).Value)
	}
	if is(pkcs11.CKK_RSA) {
		attributes, err := m.GetAttributeValue(s, handle, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_MODULUS, nil), pkcs11.NewAttribute(pkcs11.CKA_PUBLIC_EXPONENT, nil)})
		if err != nil {
			return nil, err
		}
		n, e := new(big.Int).SetBytes(attributes[0].Value), new(big.Int).SetBytes(attributes[1].Value)
		if n.BitLen() < 2048 || !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 {
			return nil, errors.New("the RSA public key is not usable")
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	}
	if !is(pkcs11.CKK_EC) && !is(ckkECEdwards) {
		return nil, errors.New("the key is neither RSA, ECDSA nor Ed25519")
	}
	attributes, err := m.GetAttributeValue(s, handle, []*pkcs11.Attribute{pkcs11.NewAttribute(pkcs11.CKA_EC_PARAMS, nil), pkcs11.NewAttribute(pkcs11.CKA_EC_POINT, nil)})
	if err != nil {
		return nil, err
	}
	var point []byte
	if _, err := asn1.Unmarshal(attributes[1].Value, &point); err != nil {
		point = attributes[1].Value
	}
	switch {
	case is(pkcs11.CKK_EC) && bytes.Equal(attributes[0].Value, oidP256):
		return ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	case is(ckkECEdwards) && len(point) == ed25519.PublicKeySize:
		return ed25519.PublicKey(point), nil
	}
	return nil, errors.New("the elliptic curve key is neither P-256 nor Ed25519")
}

func truthy(a *pkcs11.Attribute) bool {
	return len(a.Value) > 0 && a.Value[0] != 0
}

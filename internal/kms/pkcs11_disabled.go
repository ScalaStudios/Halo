//go:build !pkcs11

package kms

import (
	"context"
	"crypto"
	"errors"

	"halo/internal/config"
)

var errNoPKCS11 = errors.New("this Halo binary was built without PKCS #11 support. Build it with: CGO_ENABLED=1 go build -tags pkcs11 ./cmd/halo")

type noPKCS11 struct{}

func NewPKCS11(config.KMS) Provider {
	return noPKCS11{}
}

func (noPKCS11) UnwrapKey(context.Context, string) ([]byte, error)     { return nil, errNoPKCS11 }
func (noPKCS11) CreateKey(context.Context, string) (string, error)     { return "", errNoPKCS11 }
func (noPKCS11) Signer(context.Context, string) (crypto.Signer, error) { return nil, errNoPKCS11 }
func (noPKCS11) Ping(context.Context) error                            { return errNoPKCS11 }

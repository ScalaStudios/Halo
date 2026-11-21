package store

import (
	"context"
	"net/netip"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

type RADIUSClient struct {
	AppID     string
	Addresses []netip.Prefix
	Secrets   []RADIUSSecret
}

type RADIUSSecret struct {
	CredentialID string
	Value        []byte
}

func ClientAddress(raw string) (netip.Prefix, bool) {
	if p, err := netip.ParsePrefix(raw); err == nil {
		return p.Masked(), true
	}
	a, err := netip.ParseAddr(raw)
	return netip.PrefixFrom(a, a.BitLen()), err == nil && a.Zone() == ""
}

func (s *Store) AddSharedSecret(ctx context.Context, appID, label string, lifetime time.Duration) (Credential, string, error) {
	value := secret.Token(24)
	c := Credential{ID: id.New("crd"), Kind: "secret", Label: label, Hint: "…" + value[len(value)-4:], CreatedAt: time.Now(), ExpiresAt: time.Now().Add(lifetime)}
	_, err := s.db.Exec(ctx, `insert into app_credentials (id, app_id, kind, label, secret_sealed, hint, created_at, expires_at) values ($1, $2, 'secret', $3, $4, $5, $6, $7)`,
		c.ID, appID, c.Label, s.Sealer.Seal([]byte(value)), c.Hint, c.CreatedAt, c.ExpiresAt)
	if err != nil {
		return Credential{}, "", err
	}
	return c, value, nil
}

func (s *Store) RADIUSClients(ctx context.Context) ([]RADIUSClient, error) {
	rows, err := s.db.Query(ctx, `select a.id, a.redirect_uris, c.id, c.secret_sealed from applications a join app_credentials c on c.app_id = a.id
		where a.protocol = 'radius' and c.secret_sealed is not null and c.revoked_at is null and c.expires_at > now() order by a.id, c.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clients []RADIUSClient
	for rows.Next() {
		var appID, credentialID string
		var addresses []string
		var sealed []byte
		if err := rows.Scan(&appID, &addresses, &credentialID, &sealed); err != nil {
			return nil, err
		}
		value, err := s.Sealer.Open(sealed)
		if err != nil {
			return nil, err
		}
		if n := len(clients); n == 0 || clients[n-1].AppID != appID {
			c := RADIUSClient{AppID: appID}
			for _, raw := range addresses {
				if p, ok := ClientAddress(raw); ok {
					c.Addresses = append(c.Addresses, p)
				}
			}
			clients = append(clients, c)
		}
		clients[len(clients)-1].Secrets = append(clients[len(clients)-1].Secrets, RADIUSSecret{CredentialID: credentialID, Value: value})
	}
	return clients, rows.Err()
}

func (s *Store) TouchCredential(ctx context.Context, credentialID string) error {
	_, err := s.db.Exec(ctx, `update app_credentials set last_used_at = now() where id = $1`, credentialID)
	return err
}

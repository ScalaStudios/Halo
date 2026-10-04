package store

import "context"

func (s *Store) SeedInfra(ctx context.Context) error {
	for group, principals := range map[string][]string{"Infrastructure": {"ops"}, "Kubernetes cluster admins": {"root"}} {
		var groupID string
		if err := s.db.QueryRow(ctx, `select id from groups where name = $1`, group).Scan(&groupID); err != nil {
			return notFound(err)
		}
		if _, err := s.SaveSSHPrincipalMapping(ctx, SSHPrincipalMapping{GroupID: groupID, Principals: principals}); err != nil {
			return err
		}
	}
	var appID string
	if err := s.db.QueryRow(ctx, `select id from applications where name = 'Billing API'`).Scan(&appID); err != nil {
		return notFound(err)
	}
	billing, err := s.SaveAPIResource(ctx, APIResource{
		Name:           "Billing API",
		Identifier:     "https://billing.internal.example.com",
		Description:    "Invoices and usage records. Validates Halo access tokens against the JWKS.",
		AccessTokenTTL: 300,
		Scopes: []APIScope{
			{Name: "billing:read", Description: "Read invoices and usage records."},
			{Name: "billing:write", Description: "Create, update and void invoices."},
		},
	})
	if err != nil {
		return err
	}
	return s.SetAPIResourceGrant(ctx, billing.ID, appID, []string{"billing:read", "billing:write"})
}

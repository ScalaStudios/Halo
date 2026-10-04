package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"halo/internal/id"
	"halo/internal/secret"
)

type APIKey struct {
	ID                 string     `json:"id"`
	ServiceAccountID   string     `json:"serviceAccountId"`
	ServiceAccountName string     `json:"serviceAccountName"`
	Label              string     `json:"label"`
	Prefix             string     `json:"prefix"`
	Scopes             []string   `json:"scopes"`
	CreatedBy          *string    `json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	ExpiresAt          *time.Time `json:"expiresAt"`
	LastUsedAt         *time.Time `json:"lastUsedAt"`
	RevokedAt          *time.Time `json:"revokedAt"`
}

type ServiceAccount struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Description string    `json:"description"`
	OwnerID     *string   `json:"ownerId"`
	Status      string    `json:"status"`
	Roles       []string  `json:"roles"`
	Keys        []APIKey  `json:"keys"`
	CreatedBy   *string   `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

type NewServiceAccount struct {
	Name        string
	Description string
	OwnerID     *string
	CreatedBy   *string
	Roles       []string
}

const apiKeyQuery = `select k.id, k.service_account_id, u.name, k.label, k.prefix, k.scopes, k.created_by, k.created_at, k.expires_at, k.last_used_at, k.revoked_at
	from api_keys k join users u on u.id = k.service_account_id`

func scanAPIKey(row rowScanner) (APIKey, error) {
	var k APIKey
	err := row.Scan(&k.ID, &k.ServiceAccountID, &k.ServiceAccountName, &k.Label, &k.Prefix, &k.Scopes, &k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt)
	return k, err
}

func (s *Store) ListAPIKeys(ctx context.Context, serviceAccountID string) ([]APIKey, error) {
	rows, err := s.db.Query(ctx, apiKeyQuery+` where $1 = '' or k.service_account_id = $1 order by k.created_at desc, k.id`, serviceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) GetAPIKey(ctx context.Context, keyID string) (APIKey, error) {
	k, err := scanAPIKey(s.db.QueryRow(ctx, apiKeyQuery+` where k.id = $1`, keyID))
	return k, notFound(err)
}

func (s *Store) CreateAPIKey(ctx context.Context, serviceAccountID, label string, scopes []string, expiresAt *time.Time, createdBy *string) (APIKey, string, error) {
	value := "hlk_" + secret.Token(32)
	keyID := id.New("key")
	_, err := s.db.Exec(ctx, `insert into api_keys (id, service_account_id, label, prefix, key_hash, scopes, created_by, expires_at) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		keyID, serviceAccountID, label, value[:12], secret.Hash(value), scopes, createdBy, expiresAt)
	if err != nil {
		return APIKey{}, "", err
	}
	k, err := s.GetAPIKey(ctx, keyID)
	return k, value, err
}

func (s *Store) RevokeAPIKey(ctx context.Context, keyID string) error {
	tag, err := s.db.Exec(ctx, `update api_keys set revoked_at = now() where id = $1 and revoked_at is null`, keyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ResolveAPIKey(ctx context.Context, token string) (User, []string, error) {
	var keyID, accountID string
	var scopes []string
	var lastUsed *time.Time
	err := s.db.QueryRow(ctx, `select k.id, k.service_account_id, k.scopes, k.last_used_at from api_keys k join users u on u.id = k.service_account_id
		where k.key_hash = $1 and k.revoked_at is null and (k.expires_at is null or k.expires_at > now()) and u.status = 'active'`, secret.Hash(token)).Scan(&keyID, &accountID, &scopes, &lastUsed)
	if err != nil {
		return User{}, nil, notFound(err)
	}
	if lastUsed == nil || time.Since(*lastUsed) > time.Minute {
		_, _ = s.db.Exec(ctx, `update api_keys set last_used_at = now() where id = $1`, keyID)
	}
	u, err := s.GetUser(ctx, accountID)
	return u, scopes, err
}

func (s *Store) ListServiceAccounts(ctx context.Context) ([]ServiceAccount, error) {
	return s.serviceAccounts(ctx, "")
}

func (s *Store) GetServiceAccount(ctx context.Context, accountID string) (ServiceAccount, error) {
	accounts, err := s.serviceAccounts(ctx, accountID)
	if err != nil {
		return ServiceAccount{}, err
	}
	if len(accounts) == 0 {
		return ServiceAccount{}, ErrNotFound
	}
	return accounts[0], nil
}

func (s *Store) serviceAccounts(ctx context.Context, accountID string) ([]ServiceAccount, error) {
	rows, err := s.db.Query(ctx, `select u.id, u.name, u.email, a.description, a.owner_id, u.status, a.created_by, u.created_at,
		array(select r.role from user_roles r where r.user_id = u.id order by r.role)
		from service_accounts a join users u on u.id = a.user_id where $1 = '' or u.id = $1 order by lower(u.name), u.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := []ServiceAccount{}
	index := map[string]int{}
	for rows.Next() {
		a := ServiceAccount{Keys: []APIKey{}}
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Description, &a.OwnerID, &a.Status, &a.CreatedBy, &a.CreatedAt, &a.Roles); err != nil {
			return nil, err
		}
		index[a.ID] = len(accounts)
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys, err := s.ListAPIKeys(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if i, ok := index[k.ServiceAccountID]; ok {
			accounts[i].Keys = append(accounts[i].Keys, k)
		}
	}
	return accounts, nil
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Store) CreateServiceAccount(ctx context.Context, in NewServiceAccount) (ServiceAccount, error) {
	slug := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(in.Name), "-"), "-")
	if slug == "" {
		slug = "service"
	}
	var accountID string
	err := s.Tx(ctx, func(tx *Store) error {
		u, err := tx.CreateUser(ctx, NewUser{Email: slug + "@service.halo.invalid", Name: in.Name, Status: "active", Roles: in.Roles})
		if err != nil {
			return err
		}
		accountID = u.ID
		if _, err := tx.db.Exec(ctx, `update users set kind = 'service' where id = $1`, u.ID); err != nil {
			return err
		}
		_, err = tx.db.Exec(ctx, `insert into service_accounts (user_id, description, owner_id, created_by) values ($1, $2, $3, $4)`, u.ID, in.Description, in.OwnerID, in.CreatedBy)
		return err
	})
	if err != nil {
		return ServiceAccount{}, err
	}
	return s.GetServiceAccount(ctx, accountID)
}

func (s *Store) UpdateServiceAccount(ctx context.Context, accountID, name, description string, ownerID *string) error {
	return s.Tx(ctx, func(tx *Store) error {
		tag, err := tx.db.Exec(ctx, `update service_accounts set description = $2, owner_id = $3 where user_id = $1`, accountID, description, ownerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.db.Exec(ctx, `update users set name = $2, updated_at = now() where id = $1`, accountID, name)
		return err
	})
}

func (s *Store) SeedProvisioning(ctx context.Context) error {
	return s.Tx(ctx, func(tx *Store) error {
		person := func(email string) (*string, error) {
			u, err := tx.GetUserByEmail(ctx, email)
			if errors.Is(err, ErrNotFound) {
				return nil, nil
			}
			return &u.ID, err
		}
		marcus, err := person("marcus.okafor@example.com")
		if err != nil {
			return err
		}
		luna, err := person("luna@example.com")
		if err != nil {
			return err
		}
		days := func(n int) time.Time { return time.Now().Add(-time.Duration(n) * 24 * time.Hour) }
		minutes := func(n int) time.Time { return time.Now().Add(-time.Duration(n) * time.Minute) }

		workday, err := tx.CreateServiceAccount(ctx, NewServiceAccount{Name: "Workday connector", Description: "Creates, updates and deactivates people from Workday over SCIM.", OwnerID: marcus, CreatedBy: marcus, Roles: []string{"user_admin"}})
		if err != nil {
			return err
		}
		terraform, err := tx.CreateServiceAccount(ctx, NewServiceAccount{Name: "Terraform", Description: "Registers applications and assigns groups from the infrastructure repository.", OwnerID: luna, CreatedBy: luna, Roles: []string{"app_admin"}})
		if err != nil {
			return err
		}
		expires := time.Now().Add(76 * 24 * time.Hour)
		for _, k := range []struct {
			account, label, scope string
			by                    *string
			expires               *time.Time
			created, used         time.Time
		}{
			{workday.ID, "Workday tenant fernway", "scim", marcus, nil, days(190), minutes(12)},
			{terraform.ID, "Infrastructure CI", "api", luna, &expires, days(290), minutes(60 * 3)},
		} {
			key, _, err := tx.CreateAPIKey(ctx, k.account, k.label, []string{k.scope}, k.expires, k.by)
			if err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `update api_keys set created_at = $2, last_used_at = $3 where id = $1`, key.ID, k.created, k.used); err != nil {
				return err
			}
			if _, err := tx.db.Exec(ctx, `update users set created_at = $2, updated_at = $2 where id = $1`, k.account, k.created); err != nil {
				return err
			}
		}

		rows, err := tx.db.Query(ctx, `select id, name from users where source = 'SCIM · Workday' and kind = 'person' order by created_at, id`)
		if err != nil {
			return err
		}
		ids, names := []string{}, map[string]string{}
		for rows.Next() {
			var userID, name string
			if err := rows.Scan(&userID, &name); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, userID)
			names[name] = userID
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i, userID := range ids {
			if _, err := tx.db.Exec(ctx, `update users set external_id = $2 where id = $1`, userID, fmt.Sprintf("WD%06d", 104200+i*7)); err != nil {
				return err
			}
		}
		for _, e := range []struct {
			minutes        int
			action, person string
			summary        string
		}{
			{60 * 46, "scim.user.update", "Hana Kobayashi", "Updated title and department through SCIM"},
			{60 * 30, "scim.user.update", "Priya Raman", "Updated manager through SCIM"},
			{60 * 7, "scim.user.update", "Elena Vasquez", "Updated title through SCIM"},
		} {
			target, ok := names[e.person]
			if !ok {
				continue
			}
			if err := tx.RecordAudit(ctx, AuditEvent{Time: minutes(e.minutes), ActorID: &workday.ID, Action: e.action, Summary: e.summary, TargetType: "user", TargetID: target, TargetLabel: e.person}); err != nil {
				return err
			}
		}
		return nil
	})
}

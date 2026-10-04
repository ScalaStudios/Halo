package store

import (
	"context"
	"time"
)

type SCIMUser struct {
	ID          string
	Email       string
	Name        string
	Title       string
	Department  string
	Status      string
	ManagerID   *string
	ManagerName *string
	ExternalID  *string
	Privileged  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SCIMMember struct {
	ID   string
	Name string
}

type SCIMGroup struct {
	ID         string
	Name       string
	ExternalID *string
	CreatedAt  time.Time
	Members    []SCIMMember
}

const scimUserQuery = `select u.id, u.email, u.name, u.title, u.department, u.status, u.manager_id, m.name, u.external_id,
	exists (select 1 from user_roles r where r.user_id = u.id), u.created_at, u.updated_at
	from users u left join users m on m.id = u.manager_id where u.kind = 'person' and u.status <> 'deprovisioned'`

func scanSCIMUser(row rowScanner) (SCIMUser, error) {
	var u SCIMUser
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Title, &u.Department, &u.Status, &u.ManagerID, &u.ManagerName, &u.ExternalID, &u.Privileged, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) SCIMUsers(ctx context.Context, email, externalID string, offset, limit int) ([]SCIMUser, int, error) {
	where := ` and ($1 = '' or lower(u.email) = lower($1)) and ($2 = '' or u.external_id = $2)`
	var total int
	if err := s.db.QueryRow(ctx, `select count(*) from (`+scimUserQuery+where+`) matched`, email, externalID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, scimUserQuery+where+` order by u.created_at, u.id offset $3 limit $4`, email, externalID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := []SCIMUser{}
	for rows.Next() {
		u, err := scanSCIMUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (s *Store) GetSCIMUser(ctx context.Context, userID string) (SCIMUser, error) {
	u, err := scanSCIMUser(s.db.QueryRow(ctx, scimUserQuery+` and u.id = $1`, userID))
	return u, notFound(err)
}

func (s *Store) UpdateSCIMUser(ctx context.Context, u SCIMUser) error {
	tag, err := s.db.Exec(ctx, `update users set email = $2, email_verified = email_verified and lower(email) = lower($2), name = $3, title = $4, department = $5, manager_id = $6, external_id = $7, updated_at = now() where id = $1 and kind = 'person'`,
		u.ID, u.Email, u.Name, u.Title, u.Department, u.ManagerID, u.ExternalID)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SCIMPeople(ctx context.Context, ids []string) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `select id, name from users where id = any($1) and kind = 'person' and status <> 'deprovisioned'`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	people := map[string]string{}
	for rows.Next() {
		var userID, name string
		if err := rows.Scan(&userID, &name); err != nil {
			return nil, err
		}
		people[userID] = name
	}
	return people, rows.Err()
}

const scimGroupQuery = `select g.id, g.name, g.external_id, g.created_at from groups g where g.kind = 'assigned'`

func (s *Store) SCIMGroups(ctx context.Context, name, externalID string, offset, limit int) ([]SCIMGroup, int, error) {
	where := ` and ($1 = '' or lower(g.name) = lower($1)) and ($2 = '' or g.external_id = $2)`
	var total int
	if err := s.db.QueryRow(ctx, `select count(*) from (`+scimGroupQuery+where+`) matched`, name, externalID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, scimGroupQuery+where+` order by g.created_at, g.id offset $3 limit $4`, name, externalID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	groups := []SCIMGroup{}
	for rows.Next() {
		g := SCIMGroup{Members: []SCIMMember{}}
		if err := rows.Scan(&g.ID, &g.Name, &g.ExternalID, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, 0, err
		}
		groups = append(groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return groups, total, s.scimMembers(ctx, groups)
}

func (s *Store) GetSCIMGroup(ctx context.Context, groupID string) (SCIMGroup, error) {
	g := SCIMGroup{Members: []SCIMMember{}}
	if err := s.db.QueryRow(ctx, scimGroupQuery+` and g.id = $1`, groupID).Scan(&g.ID, &g.Name, &g.ExternalID, &g.CreatedAt); err != nil {
		return SCIMGroup{}, notFound(err)
	}
	groups := []SCIMGroup{g}
	if err := s.scimMembers(ctx, groups); err != nil {
		return SCIMGroup{}, err
	}
	return groups[0], nil
}

func (s *Store) scimMembers(ctx context.Context, groups []SCIMGroup) error {
	ids := make([]string, len(groups))
	index := map[string]int{}
	for i, g := range groups {
		ids[i] = g.ID
		index[g.ID] = i
	}
	rows, err := s.db.Query(ctx, `select m.group_id, u.id, u.name from group_members m join users u on u.id = m.user_id
		where m.group_id = any($1) and u.kind = 'person' and u.status <> 'deprovisioned' order by lower(u.name), u.id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var groupID string
		var m SCIMMember
		if err := rows.Scan(&groupID, &m.ID, &m.Name); err != nil {
			return err
		}
		groups[index[groupID]].Members = append(groups[index[groupID]].Members, m)
	}
	return rows.Err()
}

func (s *Store) UpdateSCIMGroup(ctx context.Context, groupID, name string, externalID *string) error {
	tag, err := s.db.Exec(ctx, `update groups set name = $2, external_id = $3 where id = $1 and kind = 'assigned'`, groupID, name, externalID)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type AppProvisioning struct {
	AppID       string     `json:"appId"`
	BaseURL     string     `json:"baseUrl"`
	Enabled     bool       `json:"enabled"`
	LastSyncAt  *time.Time `json:"lastSyncAt"`
	LastError   string     `json:"lastError"`
	Created     int        `json:"created"`
	Updated     int        `json:"updated"`
	Deactivated int        `json:"deactivated"`
	Provisioned int        `json:"provisioned"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	AppStatus   string     `json:"-"`
	TokenSealed []byte     `json:"-"`
}

type ProvisionedUser struct {
	RemoteID string
	Hash     string
	Active   bool
}

const appProvisioningQuery = `select p.app_id, p.base_url, p.enabled, p.last_sync_at, p.last_error, p.created_count, p.updated_count, p.deactivated_count,
	(select count(*) from app_provisioned_users pu where pu.app_id = p.app_id and pu.active), p.updated_at, a.status, p.token_sealed
	from app_provisioning p join applications a on a.id = p.app_id`

func scanAppProvisioning(row rowScanner) (AppProvisioning, error) {
	var p AppProvisioning
	err := row.Scan(&p.AppID, &p.BaseURL, &p.Enabled, &p.LastSyncAt, &p.LastError, &p.Created, &p.Updated, &p.Deactivated, &p.Provisioned, &p.UpdatedAt, &p.AppStatus, &p.TokenSealed)
	return p, err
}

func (s *Store) ListAppProvisioning(ctx context.Context) ([]AppProvisioning, error) {
	rows, err := s.db.Query(ctx, appProvisioningQuery+` order by a.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := []AppProvisioning{}
	for rows.Next() {
		p, err := scanAppProvisioning(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, p)
	}
	return configs, rows.Err()
}

func (s *Store) GetAppProvisioning(ctx context.Context, appID string) (AppProvisioning, error) {
	p, err := scanAppProvisioning(s.db.QueryRow(ctx, appProvisioningQuery+` where p.app_id = $1`, appID))
	return p, notFound(err)
}

func (s *Store) SaveAppProvisioning(ctx context.Context, appID, baseURL string, tokenSealed []byte, enabled bool) error {
	if tokenSealed == nil {
		tag, err := s.db.Exec(ctx, `update app_provisioning set base_url = $2, enabled = $3, updated_at = now() where app_id = $1`, appID, baseURL, enabled)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return err
	}
	_, err := s.db.Exec(ctx, `insert into app_provisioning (app_id, base_url, token_sealed, enabled) values ($1, $2, $3, $4)
		on conflict (app_id) do update set base_url = excluded.base_url, token_sealed = excluded.token_sealed, enabled = excluded.enabled, updated_at = now()`,
		appID, baseURL, tokenSealed, enabled)
	return err
}

func (s *Store) RecordProvisioningRun(ctx context.Context, appID string, created, updated, deactivated int, lastError string) error {
	_, err := s.db.Exec(ctx, `update app_provisioning set last_sync_at = now(), created_count = $2, updated_count = $3, deactivated_count = $4, last_error = $5 where app_id = $1`,
		appID, created, updated, deactivated, lastError)
	return err
}

func (s *Store) ListProvisionedUsers(ctx context.Context, appID string) (map[string]ProvisionedUser, error) {
	rows, err := s.db.Query(ctx, `select user_id, remote_id, attributes_hash, active from app_provisioned_users where app_id = $1`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := map[string]ProvisionedUser{}
	for rows.Next() {
		var userID string
		var u ProvisionedUser
		if err := rows.Scan(&userID, &u.RemoteID, &u.Hash, &u.Active); err != nil {
			return nil, err
		}
		users[userID] = u
	}
	return users, rows.Err()
}

func (s *Store) SaveProvisionedUser(ctx context.Context, appID, userID string, u ProvisionedUser) error {
	_, err := s.db.Exec(ctx, `insert into app_provisioned_users (app_id, user_id, remote_id, attributes_hash, active) values ($1, $2, $3, $4, $5)
		on conflict (app_id, user_id) do update set remote_id = excluded.remote_id, attributes_hash = excluded.attributes_hash, active = excluded.active, synced_at = now()`,
		appID, userID, u.RemoteID, u.Hash, u.Active)
	return err
}

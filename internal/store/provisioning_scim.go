package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"

	"halo/internal/scim/filter"
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

const scimUserSelect = `select u.id, u.email, u.name, u.title, u.department, u.status, u.manager_id, m.name, u.external_id,
	exists (select 1 from user_roles r where r.user_id = u.id) or exists (select 1 from role_eligibilities e where e.user_id = u.id), u.created_at, u.updated_at
	from users u left join users m on m.id = u.manager_id where u.kind = 'person'`

const scimUserQuery = scimUserSelect + ` and u.status <> 'deprovisioned'`

var SCIMUserFilter = filter.Schema{
	Prefixes: map[string]string{"urn:ietf:params:scim:schemas:core:2.0:user:": "", "urn:ietf:params:scim:schemas:extension:enterprise:2.0:user:": "enterprise:"},
	Multi: map[string]string{"schemas": `exists (select 1 from unnest(array_remove(array['urn:ietf:params:scim:schemas:core:2.0:User',
		case when u.department <> '' or u.manager_id is not null then 'urn:ietf:params:scim:schemas:extension:enterprise:2.0:User' end], null)) sc(uri) where %s)`},
	Attrs: map[string]filter.Attr{
		"schemas":                        {Expr: "sc.uri"},
		"id":                             {Expr: "u.id", Exact: true},
		"externalid":                     {Expr: "u.external_id", Exact: true},
		"username":                       {Expr: "u.email"},
		"emails":                         {Expr: "u.email"},
		"emails.value":                   {Expr: "u.email"},
		"emails.type":                    {Expr: "'work'"},
		"emails.primary":                 {Expr: "true", Kind: filter.Boolean},
		"displayname":                    {Expr: "u.name"},
		"name":                           {Expr: "nullif(u.name, '')", Kind: filter.Complex},
		"name.formatted":                 {Expr: "u.name"},
		"name.givenname":                 {Expr: "split_part(u.name, ' ', 1)"},
		"name.familyname":                {Expr: "substr(u.name, length(split_part(u.name, ' ', 1)) + 2)"},
		"title":                          {Expr: "u.title"},
		"active":                         {Expr: "u.status <> 'suspended'", Kind: filter.Boolean},
		"meta.created":                   {Expr: "u.created_at", Kind: filter.DateTime},
		"meta.lastmodified":              {Expr: "u.updated_at", Kind: filter.DateTime},
		"enterprise:department":          {Expr: "u.department"},
		"enterprise:manager":             {Expr: "u.manager_id", Exact: true},
		"enterprise:manager.value":       {Expr: "u.manager_id", Exact: true},
		"enterprise:manager.displayname": {Expr: "m.name"},
	},
}

var SCIMGroupFilter = filter.Schema{
	Prefixes: map[string]string{"urn:ietf:params:scim:schemas:core:2.0:group:": ""},
	Multi: map[string]string{"members": `exists (select 1 from group_members gm join users mu on mu.id = gm.user_id
		where gm.group_id = g.id and mu.kind = 'person' and mu.status <> 'deprovisioned' and %s)`},
	Attrs: map[string]filter.Attr{
		"schemas":           {Expr: "'urn:ietf:params:scim:schemas:core:2.0:Group'"},
		"id":                {Expr: "g.id", Exact: true},
		"externalid":        {Expr: "g.external_id", Exact: true},
		"displayname":       {Expr: "g.name"},
		"meta.created":      {Expr: "g.created_at", Kind: filter.DateTime},
		"meta.lastmodified": {Expr: "g.created_at", Kind: filter.DateTime},
		"members":           {Expr: "gm.user_id", Exact: true},
		"members.value":     {Expr: "gm.user_id", Exact: true},
		"members.display":   {Expr: "mu.name"},
		"members.type":      {Expr: "'User'"},
	},
}

func scanSCIMUser(row rowScanner) (SCIMUser, error) {
	var u SCIMUser
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Title, &u.Department, &u.Status, &u.ManagerID, &u.ManagerName, &u.ExternalID, &u.Privileged, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func scimPage(order string, args []any, offset, limit int) (string, []any) {
	return fmt.Sprintf(` order by %s offset $%d limit $%d`, order, len(args)+1, len(args)+2), append(slices.Clip(args), offset, limit)
}

func (s *Store) SCIMUsers(ctx context.Context, filter string, args []any, offset, limit int) ([]SCIMUser, int, error) {
	query := scimUserQuery + ` and (` + filter + `)`
	var total int
	if err := s.db.QueryRow(ctx, `select count(*) from (`+query+`) matched`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageArgs := scimPage("u.created_at, u.id", args, offset, limit)
	rows, err := s.db.Query(ctx, query+page, pageArgs...)
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

func (s *Store) ReclaimDeprovisionedSCIMUser(ctx context.Context, email string, externalID *string) (SCIMUser, error) {
	u, err := SCIMUser{}, ErrNotFound
	if externalID != nil {
		u, err = scanSCIMUser(s.db.QueryRow(ctx, scimUserSelect+` and u.status = 'deprovisioned' and u.external_id = $1 limit 1`, *externalID))
		err = notFound(err)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return u, err
	}
	return u, err
}

func (s *Store) FreeDeprovisionedEmail(ctx context.Context, email, keepID string) ([]User, error) {
	rows, err := s.db.Query(ctx, `update users set email = id || '@deprovisioned.invalid' where kind = 'person' and status = 'deprovisioned' and lower(email) = lower($1) and id <> $2 returning id, name`, email, keepID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var u User
		return u, row.Scan(&u.ID, &u.Name)
	})
}

func (s *Store) UpdateSCIMUser(ctx context.Context, u SCIMUser) error {
	var previous string
	err := s.db.QueryRow(ctx, `with old as (select email from users where id = $1)
		update users set email = $2, email_verified = email_verified and lower(email) = lower($2), name = $3, title = $4, department = $5, manager_id = $6, external_id = $7, updated_at = now() where id = $1 and kind = 'person' returning (select email from old)`,
		u.ID, u.Email, u.Name, u.Title, u.Department, u.ManagerID, u.ExternalID).Scan(&previous)
	if err != nil {
		return conflict(notFound(err))
	}
	return s.emailChanged(ctx, u.ID, previous, u.Email)
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

func (s *Store) SCIMGroups(ctx context.Context, filter string, args []any, offset, limit int) ([]SCIMGroup, int, error) {
	query := scimGroupQuery + ` and (` + filter + `)`
	var total int
	if err := s.db.QueryRow(ctx, `select count(*) from (`+query+`) matched`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, pageArgs := scimPage("g.created_at, g.id", args, offset, limit)
	rows, err := s.db.Query(ctx, query+page, pageArgs...)
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
	if err := s.checkRename(ctx, groupID, name); err != nil {
		return err
	}
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
	AppID         string     `json:"appId"`
	BaseURL       string     `json:"baseUrl"`
	Enabled       bool       `json:"enabled"`
	LastSyncAt    *time.Time `json:"lastSyncAt"`
	LastError     string     `json:"lastError"`
	Created       int        `json:"created"`
	Updated       int        `json:"updated"`
	Deactivated   int        `json:"deactivated"`
	Provisioned   int        `json:"provisioned"`
	GroupIDs      []string   `json:"groupIds"`
	GroupsCreated int        `json:"groupsCreated"`
	GroupsUpdated int        `json:"groupsUpdated"`
	GroupsDeleted int        `json:"groupsDeleted"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	AppStatus     string     `json:"-"`
	TokenSealed   []byte     `json:"-"`
}

type ProvisionedUser struct {
	RemoteID string
	Hash     string
	Active   bool
}

type ProvisionedGroup struct {
	RemoteID string
	Name     string
	Members  []string
}

const appProvisioningQuery = `select p.app_id, p.base_url, p.enabled, p.last_sync_at, p.last_error, p.created_count, p.updated_count, p.deactivated_count,
	(select count(*) from app_provisioned_users pu where pu.app_id = p.app_id and pu.active),
	array(select g.id from groups g where g.id = any(p.group_ids) order by g.name), p.groups_created_count, p.groups_updated_count, p.groups_deleted_count, p.updated_at, a.status, p.token_sealed
	from app_provisioning p join applications a on a.id = p.app_id`

func scanAppProvisioning(row rowScanner) (AppProvisioning, error) {
	var p AppProvisioning
	err := row.Scan(&p.AppID, &p.BaseURL, &p.Enabled, &p.LastSyncAt, &p.LastError, &p.Created, &p.Updated, &p.Deactivated, &p.Provisioned, &p.GroupIDs, &p.GroupsCreated, &p.GroupsUpdated, &p.GroupsDeleted, &p.UpdatedAt, &p.AppStatus, &p.TokenSealed)
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

func (s *Store) SetProvisioningGroups(ctx context.Context, appID string, groupIDs []string) error {
	_, err := s.db.Exec(ctx, `update app_provisioning set group_ids = $2, updated_at = now() where app_id = $1`, appID, groupIDs)
	return err
}

func (s *Store) RecordProvisioningRun(ctx context.Context, appID string, created, updated, deactivated, groupsCreated, groupsUpdated, groupsDeleted int, lastError string) error {
	_, err := s.db.Exec(ctx, `update app_provisioning set last_sync_at = now(), created_count = $2, updated_count = $3, deactivated_count = $4,
		groups_created_count = $5, groups_updated_count = $6, groups_deleted_count = $7, last_error = $8 where app_id = $1`,
		appID, created, updated, deactivated, groupsCreated, groupsUpdated, groupsDeleted, lastError)
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

func (s *Store) ListProvisionedGroups(ctx context.Context, appID string) (map[string]ProvisionedGroup, error) {
	rows, err := s.db.Query(ctx, `select group_id, remote_id, display_name, members from app_provisioned_groups where app_id = $1`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := map[string]ProvisionedGroup{}
	for rows.Next() {
		var groupID string
		var g ProvisionedGroup
		if err := rows.Scan(&groupID, &g.RemoteID, &g.Name, &g.Members); err != nil {
			return nil, err
		}
		groups[groupID] = g
	}
	return groups, rows.Err()
}

func (s *Store) SaveProvisionedGroup(ctx context.Context, appID, groupID string, g ProvisionedGroup) error {
	_, err := s.db.Exec(ctx, `insert into app_provisioned_groups (app_id, group_id, remote_id, display_name, members) values ($1, $2, $3, $4, $5)
		on conflict (app_id, group_id) do update set remote_id = excluded.remote_id, display_name = excluded.display_name, members = excluded.members, synced_at = now()`,
		appID, groupID, g.RemoteID, g.Name, g.Members)
	return err
}

func (s *Store) DeleteProvisionedGroup(ctx context.Context, appID, groupID string) error {
	_, err := s.db.Exec(ctx, `delete from app_provisioned_groups where app_id = $1 and group_id = $2`, appID, groupID)
	return err
}

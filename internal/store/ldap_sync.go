package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type LDAPMapping struct {
	ID         string            `json:"id"`
	Email      string            `json:"email"`
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	Department string            `json:"department"`
	Location   string            `json:"location"`
	Manager    string            `json:"manager"`
	GroupName  string            `json:"groupName"`
	Member     string            `json:"member"`
	Attributes map[string]string `json:"attributes"`
}

type LDAPDirectory struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	URL             string      `json:"url"`
	Security        string      `json:"security"`
	CACertificate   string      `json:"caCertificate"`
	BindDN          string      `json:"bindDn"`
	UserBaseDN      string      `json:"userBaseDn"`
	UserFilter      string      `json:"userFilter"`
	GroupBaseDN     string      `json:"groupBaseDn"`
	GroupFilter     string      `json:"groupFilter"`
	Mapping         LDAPMapping `json:"mapping"`
	Enabled         bool        `json:"enabled"`
	IntervalMinutes int         `json:"intervalMinutes"`
	SyncedAt        *time.Time  `json:"syncedAt"`
	SyncError       string      `json:"syncError"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	People          int         `json:"people"`
	Groups          int         `json:"groups"`
	Sealed          []byte      `json:"-"`
}

type LDAPPerson struct {
	EntryID   string
	UserID    string
	DN        string
	Suspended bool
}

type LDAPCounts struct {
	Created        int `json:"created"`
	Updated        int `json:"updated"`
	Suspended      int `json:"suspended"`
	Restored       int `json:"restored"`
	GroupsCreated  int `json:"groupsCreated"`
	GroupsRenamed  int `json:"groupsRenamed"`
	MembersAdded   int `json:"membersAdded"`
	MembersRemoved int `json:"membersRemoved"`
}

type LDAPSync struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt time.Time  `json:"finishedAt"`
	Result     string     `json:"result"`
	Error      string     `json:"error"`
	Counts     LDAPCounts `json:"counts"`
	Skipped    []string   `json:"skipped"`
}

const directoryQuery = `select d.id, d.name, d.url, d.security, d.ca_certificate, d.bind_dn, d.user_base_dn, d.user_filter, d.group_base_dn, d.group_filter, d.mapping,
	d.enabled, d.interval_minutes, d.synced_at, d.sync_error, d.created_at, d.updated_at,
	(select count(*) from ldap_people p where p.directory_id = d.id)::int, (select count(*) from ldap_groups g where g.directory_id = d.id)::int, d.password_sealed from ldap_directories d`

func (s *Store) ListLDAPDirectories(ctx context.Context) ([]LDAPDirectory, error) {
	rows, err := s.db.Query(ctx, directoryQuery+` order by lower(d.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[LDAPDirectory])
}

func (s *Store) GetLDAPDirectory(ctx context.Context, directoryID string) (LDAPDirectory, error) {
	rows, err := s.db.Query(ctx, directoryQuery+` where d.id = $1`, directoryID)
	if err != nil {
		return LDAPDirectory{}, err
	}
	d, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[LDAPDirectory])
	return d, notFound(err)
}

func (s *Store) SetLDAPConfiguredBy(ctx context.Context, directoryID, userID string) error {
	_, err := s.db.Exec(ctx, `update ldap_directories set configured_by = $2 where id = $1`, directoryID, userID)
	return err
}

func (s *Store) LDAPConfiguredBy(ctx context.Context, directoryID string) (*string, error) {
	var id *string
	err := s.db.QueryRow(ctx, `select configured_by from ldap_directories where id = $1`, directoryID).Scan(&id)
	return id, notFound(err)
}

func (s *Store) CreateLDAPDirectory(ctx context.Context, d LDAPDirectory, password string) (string, error) {
	directoryID := id.New("ldp")
	_, err := s.db.Exec(ctx, `insert into ldap_directories (id, name, url, security, ca_certificate, bind_dn, password_sealed, user_base_dn, user_filter, group_base_dn, group_filter, mapping, enabled, interval_minutes)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		directoryID, d.Name, d.URL, d.Security, d.CACertificate, d.BindDN, s.Sealer.Seal([]byte(password)), d.UserBaseDN, d.UserFilter, d.GroupBaseDN, d.GroupFilter, d.Mapping, d.Enabled, d.IntervalMinutes)
	return directoryID, conflict(err)
}

func (s *Store) UpdateLDAPDirectory(ctx context.Context, d LDAPDirectory, password string) error {
	var sealed []byte
	if password != "" {
		sealed = s.Sealer.Seal([]byte(password))
	}
	tag, err := s.db.Exec(ctx, `update ldap_directories set name = $2, url = $3, security = $4, ca_certificate = $5, bind_dn = $6, password_sealed = coalesce($7, password_sealed), user_base_dn = $8,
		user_filter = $9, group_base_dn = $10, group_filter = $11, mapping = $12, enabled = $13, interval_minutes = $14, updated_at = now() where id = $1`,
		d.ID, d.Name, d.URL, d.Security, d.CACertificate, d.BindDN, sealed, d.UserBaseDN, d.UserFilter, d.GroupBaseDN, d.GroupFilter, d.Mapping, d.Enabled, d.IntervalMinutes)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteLDAPDirectory(ctx context.Context, directoryID string) error {
	tag, err := s.db.Exec(ctx, `delete from ldap_directories where id = $1`, directoryID)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

func (s *Store) LockLDAPDirectory(ctx context.Context, directoryID string) error {
	tag, err := s.db.Exec(ctx, `select 1 from ldap_directories where id = $1 for update`, directoryID)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

func (s *Store) LDAPPeople(ctx context.Context, directoryID string) ([]LDAPPerson, error) {
	rows, err := s.db.Query(ctx, `select entry_id, user_id, dn, suspended from ldap_people where directory_id = $1`, directoryID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[LDAPPerson])
}

func (s *Store) SaveLDAPPerson(ctx context.Context, directoryID string, p LDAPPerson) error {
	_, err := s.db.Exec(ctx, `insert into ldap_people (directory_id, entry_id, user_id, dn, suspended) values ($1, $2, $3, $4, $5)
		on conflict (directory_id, entry_id) do update set dn = excluded.dn, suspended = excluded.suspended`, directoryID, p.EntryID, p.UserID, p.DN, p.Suspended)
	return err
}

func (s *Store) LDAPGroups(ctx context.Context, directoryID string) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `select entry_id, group_id from ldap_groups where directory_id = $1`, directoryID)
	if err != nil {
		return nil, err
	}
	links := map[string]string{}
	var entryID, groupID string
	_, err = pgx.ForEachRow(rows, []any{&entryID, &groupID}, func() error {
		links[entryID] = groupID
		return nil
	})
	return links, err
}

func (s *Store) LinkLDAPGroup(ctx context.Context, directoryID, entryID, groupID string) error {
	_, err := s.db.Exec(ctx, `insert into ldap_groups (directory_id, entry_id, group_id) values ($1, $2, $3)`, directoryID, entryID, groupID)
	return err
}

func (s *Store) RecordLDAPSync(ctx context.Context, directoryID string, r LDAPSync) error {
	if _, err := s.db.Exec(ctx, `insert into ldap_syncs (id, directory_id, started_at, finished_at, result, error, counts, skipped) values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, directoryID, r.StartedAt, r.FinishedAt, r.Result, r.Error, r.Counts, r.Skipped); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `update ldap_directories set synced_at = $2, sync_error = $3 where id = $1`, directoryID, r.StartedAt, r.Error)
	return err
}

func (s *Store) ListLDAPSyncs(ctx context.Context, directoryID string, limit int) ([]LDAPSync, error) {
	rows, err := s.db.Query(ctx, `select id, started_at, finished_at, result, error, counts, skipped from ldap_syncs where directory_id = $1 order by started_at desc limit $2`, directoryID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[LDAPSync])
}

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"halo/internal/id"
)

var (
	ErrPOSIXName = errors.New("reserved or invalid posix name")
	ErrPOSIXHome = errors.New("home directory in use")
)

type POSIXAccount struct {
	Username      string `json:"username"`
	UIDNumber     int    `json:"uidNumber"`
	GIDNumber     int    `json:"gidNumber"`
	LoginShell    string `json:"loginShell"`
	HomeDirectory string `json:"homeDirectory"`
}

type POSIXGroup struct {
	Name      string `json:"name"`
	GIDNumber int    `json:"gidNumber"`
}

type POSIXSettings struct {
	UIDMin     int    `json:"uidMin"`
	UIDMax     int    `json:"uidMax"`
	GIDMin     int    `json:"gidMin"`
	GIDMax     int    `json:"gidMax"`
	NextUID    int    `json:"nextUid"`
	NextGID    int    `json:"nextGid"`
	LoginShell string `json:"loginShell"`
}

func (s *Store) GetPOSIXSettings(ctx context.Context) (POSIXSettings, error) {
	var p POSIXSettings
	err := s.db.QueryRow(ctx, `select uid_min, uid_max, gid_min, gid_max,
		coalesce((select max(number) + 1 from posix_ids where number between uid_min and uid_max), uid_min),
		coalesce((select max(number) + 1 from posix_ids where number between gid_min and gid_max), gid_min), login_shell from posix_settings`).Scan(&p.UIDMin, &p.UIDMax, &p.GIDMin, &p.GIDMax, &p.NextUID, &p.NextGID, &p.LoginShell)
	return p, err
}

func (s *Store) SavePOSIXSettings(ctx context.Context, p POSIXSettings) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `update posix_settings set uid_min = $1, uid_max = $2, gid_min = $3, gid_max = $4, login_shell = $5`, p.UIDMin, p.UIDMax, p.GIDMin, p.GIDMax, p.LoginShell); err != nil {
			return err
		}
		_, err := tx.db.Exec(ctx, `select posix_backfill()`)
		return err
	})
}

func (s *Store) SetPOSIXAccount(ctx context.Context, userID string, a POSIXAccount) error {
	var taken bool
	if err := s.db.QueryRow(ctx, `select exists (select 1 from posix_ids where home_directory = $2 and user_id <> $1)`, userID, a.HomeDirectory).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrPOSIXHome
	}
	tag, err := s.db.Exec(ctx, `update posix_ids set name = $2, login_shell = $3, home_directory = $4, updated_at = now() where user_id = $1`, userID, a.Username, a.LoginShell, a.HomeDirectory)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ErrPOSIXName
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return conflict(err)
}

type HostRule struct {
	GroupID string   `json:"groupId"`
	Sudo    []string `json:"sudo"`
}

type HostGroup struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Rules       []HostRule `json:"rules"`
	HostCount   int        `json:"hostCount"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (s *Store) ListHostGroups(ctx context.Context) ([]HostGroup, error) {
	rows, err := s.db.Query(ctx, `select h.id, h.name, h.description,
		coalesce((select json_agg(json_build_object('groupId', r.group_id, 'sudo', r.sudo) order by lower(g.name)) from host_group_rules r join groups g on g.id = r.group_id where r.host_group_id = h.id), '[]'),
		(select count(*) from ssh_hosts s where s.host_group_id = h.id and s.revoked_at is null), h.created_at, h.updated_at
		from host_groups h order by lower(h.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[HostGroup])
}

func (s *Store) GetHostGroup(ctx context.Context, hostGroupID string) (HostGroup, error) {
	groups, err := s.ListHostGroups(ctx)
	if err != nil {
		return HostGroup{}, err
	}
	for _, g := range groups {
		if g.ID == hostGroupID {
			return g, nil
		}
	}
	return HostGroup{}, ErrNotFound
}

func (s *Store) SaveHostGroup(ctx context.Context, g HostGroup) (string, error) {
	if g.ID == "" {
		g.ID = id.New("hgr")
	}
	err := s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.db.Exec(ctx, `insert into host_groups (id, name, description) values ($1, $2, $3)
			on conflict (id) do update set name = excluded.name, description = excluded.description, updated_at = now()`, g.ID, g.Name, g.Description); err != nil {
			return conflict(err)
		}
		if _, err := tx.db.Exec(ctx, `delete from host_group_rules where host_group_id = $1`, g.ID); err != nil {
			return err
		}
		for _, r := range g.Rules {
			if _, err := tx.db.Exec(ctx, `insert into host_group_rules (host_group_id, group_id, sudo) values ($1, $2, $3)`, g.ID, r.GroupID, r.Sudo); err != nil {
				return err
			}
		}
		return nil
	})
	return g.ID, err
}

func (s *Store) DeleteHostGroup(ctx context.Context, hostGroupID string) error {
	tag, err := s.db.Exec(ctx, `delete from host_groups where id = $1`, hostGroupID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetSSHHostGroup(ctx context.Context, hostID string, hostGroupID *string) (SSHHost, error) {
	return s.sshHost(ctx, `update ssh_hosts set host_group_id = $2 where id = $1 and revoked_at is null returning `+sshHostColumns, hostID, hostGroupID)
}

func (s *Store) HostRules(ctx context.Context) (map[string][]HostRule, error) {
	rows, err := s.db.Query(ctx, `select host_group_id, group_id, sudo from host_group_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := map[string][]HostRule{}
	for rows.Next() {
		var hostGroupID string
		var r HostRule
		if err := rows.Scan(&hostGroupID, &r.GroupID, &r.Sudo); err != nil {
			return nil, err
		}
		rules[hostGroupID] = append(rules[hostGroupID], r)
	}
	return rules, rows.Err()
}

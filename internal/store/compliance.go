package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"halo/internal/id"
)

type ComplianceSource struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	TenantID      string     `json:"tenantId"`
	ClientID      string     `json:"clientId"`
	MaxAgeMinutes int        `json:"maxAgeMinutes"`
	ReportedAt    *time.Time `json:"reportedAt"`
	SyncError     string     `json:"syncError"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	Records       int        `json:"records"`
	Linked        int        `json:"linked"`
	Sealed        []byte     `json:"-"`
}

type ComplianceRecord struct {
	ExternalID string    `json:"id"`
	Name       string    `json:"name"`
	Owner      string    `json:"owner"`
	State      string    `json:"state"`
	OSVersion  string    `json:"osVersion"`
	Encrypted  *bool     `json:"encrypted"`
	PatchLevel string    `json:"patchLevel"`
	ReportedAt time.Time `json:"reportedAt"`
}

type DeviceCompliance struct {
	State      string    `json:"state"`
	SourceID   string    `json:"sourceId"`
	Source     string    `json:"source"`
	ExternalID string    `json:"externalId"`
	Record     string    `json:"record"`
	ReportedAt time.Time `json:"reportedAt"`
	Stale      bool      `json:"stale"`
}

const sourceQuery = `select s.id, s.name, s.kind, s.tenant_id, s.client_id, s.max_age_minutes, s.reported_at, s.sync_error, s.created_at, s.updated_at,
	(select count(*) from compliance_records r where r.source_id = s.id)::int,
	(select count(*) from devices d where d.compliance_source_id = s.id)::int, s.secret_sealed from compliance_sources s`

func (s *Store) ListComplianceSources(ctx context.Context) ([]ComplianceSource, error) {
	rows, err := s.db.Query(ctx, sourceQuery+` order by lower(s.name)`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ComplianceSource])
}

func (s *Store) GetComplianceSource(ctx context.Context, sourceID string) (ComplianceSource, error) {
	rows, err := s.db.Query(ctx, sourceQuery+` where s.id = $1`, sourceID)
	if err != nil {
		return ComplianceSource{}, err
	}
	src, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[ComplianceSource])
	return src, notFound(err)
}

func (s *Store) CreateComplianceSource(ctx context.Context, src ComplianceSource, plain string) (string, error) {
	sourceID := id.New("cmp")
	_, err := s.db.Exec(ctx, `insert into compliance_sources (id, name, kind, tenant_id, client_id, secret_sealed, max_age_minutes) values ($1, $2, $3, $4, $5, $6, $7)`,
		sourceID, src.Name, src.Kind, src.TenantID, src.ClientID, s.Sealer.Seal([]byte(plain)), src.MaxAgeMinutes)
	return sourceID, conflict(err)
}

func (s *Store) UpdateComplianceSource(ctx context.Context, src ComplianceSource, plain string) error {
	var sealed []byte
	if plain != "" {
		sealed = s.Sealer.Seal([]byte(plain))
	}
	tag, err := s.db.Exec(ctx, `update compliance_sources set name = $2, tenant_id = $3, client_id = $4, secret_sealed = coalesce($5, secret_sealed), max_age_minutes = $6, updated_at = now() where id = $1`,
		src.ID, src.Name, src.TenantID, src.ClientID, sealed, src.MaxAgeMinutes)
	if err != nil {
		return conflict(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteComplianceSource(ctx context.Context, sourceID string) error {
	tag, err := s.db.Exec(ctx, `delete from compliance_sources where id = $1`, sourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetComplianceSyncError(ctx context.Context, sourceID, message string) error {
	_, err := s.db.Exec(ctx, `update compliance_sources set sync_error = $2 where id = $1`, sourceID, message)
	return err
}

func (s *Store) ReportCompliance(ctx context.Context, src ComplianceSource, records []ComplianceRecord, at time.Time) error {
	for _, r := range records {
		var previous string
		err := s.db.QueryRow(ctx, `with old as (select state from compliance_records where source_id = $1 and external_id = $2)
			insert into compliance_records (source_id, external_id, name, owner, state, os_version, encrypted, patch_level, reported_at) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			on conflict (source_id, external_id) do update set name = excluded.name, owner = excluded.owner, state = excluded.state, os_version = excluded.os_version,
			encrypted = excluded.encrypted, patch_level = excluded.patch_level, reported_at = excluded.reported_at
			returning coalesce((select state from old), '')`,
			src.ID, r.ExternalID, r.Name, r.Owner, r.State, r.OSVersion, r.Encrypted, r.PatchLevel, at).Scan(&previous)
		if err != nil {
			return err
		}
		if previous == "" || previous == r.State {
			continue
		}
		rows, err := s.db.Query(ctx, `select d.id, d.user_agent, u.id, u.name from devices d join users u on u.id = d.user_id where d.compliance_source_id = $1 and d.compliance_external_id = $2`, src.ID, r.ExternalID)
		if err != nil {
			return err
		}
		type linked struct{ ID, UserAgent, UserID, UserName string }
		devices, err := pgx.CollectRows(rows, pgx.RowToStructByPos[linked])
		if err != nil {
			return err
		}
		for _, d := range devices {
			err := s.RecordAudit(ctx, AuditEvent{Action: "device.compliance", Summary: src.Name + " reported " + DeviceName(d.UserAgent) + " as " + r.State + ", was " + previous,
				TargetType: "user", TargetID: d.UserID, TargetLabel: d.UserName})
			if err != nil {
				return err
			}
			if status := map[string]string{"compliant": "compliant", "non-compliant": "not-compliant"}; status[previous] != "" && status[r.State] != "" {
				err := s.signal(ctx, deviceComplianceChange, map[string]any{"previous_status": status[previous], "current_status": status[r.State]}, `select $1::text, '', $2::text, ''`, d.UserID, d.ID)
				if err != nil {
					return err
				}
			}
		}
	}
	_, err := s.db.Exec(ctx, `update compliance_sources set reported_at = $2, sync_error = '' where id = $1`, src.ID, at)
	return err
}

const recordColumns = `external_id, name, owner, state, os_version, encrypted, patch_level, reported_at`

func (s *Store) ListComplianceRecords(ctx context.Context, sourceID, search string) ([]ComplianceRecord, error) {
	rows, err := s.db.Query(ctx, `select `+recordColumns+` from compliance_records where source_id = $1
		and ($2 = '' or strpos(lower(name || ' ' || owner || ' ' || external_id), lower($2)) > 0) order by lower(owner), lower(name), external_id limit 100`, sourceID, search)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ComplianceRecord])
}

func (s *Store) ComplianceRecordsExcept(ctx context.Context, sourceID string, keep []string) ([]ComplianceRecord, error) {
	rows, err := s.db.Query(ctx, `select `+recordColumns+` from compliance_records where source_id = $1 and state <> 'unknown' and not (external_id = any($2))`, sourceID, keep)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ComplianceRecord])
}

func (s *Store) GetComplianceRecord(ctx context.Context, sourceID, externalID string) (ComplianceRecord, error) {
	rows, err := s.db.Query(ctx, `select `+recordColumns+` from compliance_records where source_id = $1 and external_id = $2`, sourceID, externalID)
	if err != nil {
		return ComplianceRecord{}, err
	}
	r, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[ComplianceRecord])
	return r, notFound(err)
}

func (s *Store) LinkDeviceCompliance(ctx context.Context, deviceID string, sourceID, externalID *string) error {
	tag, err := s.db.Exec(ctx, `update devices set compliance_source_id = $2, compliance_external_id = $3 where id = $1`, deviceID, sourceID, externalID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

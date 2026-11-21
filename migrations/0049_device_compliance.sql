create table compliance_sources (
  id text primary key,
  name text not null unique,
  kind text not null check (kind in ('intune', 'agent')),
  tenant_id text not null default '',
  client_id text not null default '',
  secret_sealed bytea not null,
  max_age_minutes integer not null default 1440 check (max_age_minutes between 60 and 43200),
  reported_at timestamptz,
  sync_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table compliance_records (
  source_id text not null references compliance_sources (id) on delete cascade,
  external_id text not null,
  name text not null default '',
  owner text not null default '',
  state text not null check (state in ('compliant', 'non-compliant', 'unknown')),
  os_version text not null default '',
  encrypted boolean,
  patch_level text not null default '',
  reported_at timestamptz not null,
  primary key (source_id, external_id)
);

alter table devices
  add column compliance_source_id text,
  add column compliance_external_id text,
  add constraint devices_compliance_record foreign key (compliance_source_id, compliance_external_id)
    references compliance_records (source_id, external_id) on delete set null;

create index devices_compliance_record on devices (compliance_source_id, compliance_external_id);

update access_policies set conditions = conditions || '{"compliance": "any"}' where not conditions ? 'compliance';

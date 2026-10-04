create table device_authorizations (
  device_code_hash bytea primary key,
  user_code text not null unique,
  client_id text not null,
  scopes text[] not null default '{}',
  ip text not null default '',
  user_agent text not null default '',
  state text not null default 'pending' check (state in ('pending', 'approved', 'denied', 'used')),
  user_id text references users (id) on delete cascade,
  amr text[] not null default '{}',
  auth_time timestamptz,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);

create table ssh_authority (
  id boolean primary key default true check (id),
  private_key_sealed bytea not null,
  public_key text not null,
  certificate_lifetime integer not null default 28800 check (certificate_lifetime between 300 and 86400),
  created_at timestamptz not null default now()
);

create table ssh_principal_mappings (
  id text primary key,
  group_id text not null unique references groups (id) on delete cascade,
  principals text[] not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create sequence ssh_certificate_serials;

create table ssh_certificates (
  serial bigint primary key,
  key_id text not null,
  user_id text references users (id) on delete set null,
  principals text[] not null,
  fingerprint text not null,
  key_type text not null,
  ip text not null default '',
  valid_after timestamptz not null,
  valid_before timestamptz not null,
  created_at timestamptz not null default now()
);

create index ssh_certificates_created on ssh_certificates (created_at desc);

create table api_resources (
  id text primary key,
  name text not null,
  identifier text not null unique,
  description text not null default '',
  access_token_ttl integer not null default 900 check (access_token_ttl between 60 and 86400),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index api_resources_name_lower on api_resources (lower(name));

create table api_resource_scopes (
  resource_id text not null references api_resources (id) on delete cascade,
  name text not null unique,
  description text not null default '',
  primary key (resource_id, name)
);

create table api_resource_grants (
  app_id text not null references applications (id) on delete cascade,
  resource_id text not null,
  scope text not null,
  created_at timestamptz not null default now(),
  primary key (app_id, resource_id, scope),
  foreign key (resource_id, scope) references api_resource_scopes (resource_id, name) on delete cascade
);

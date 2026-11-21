create table ssh_host_authority (
  id boolean primary key default true check (id),
  private_key_sealed bytea not null,
  public_key text not null,
  certificate_lifetime integer not null default 2592000 check (certificate_lifetime between 86400 and 31536000),
  created_at timestamptz not null default now()
);

create table ssh_hosts (
  id text primary key,
  hostnames text[] not null,
  public_key text unique,
  fingerprint text not null default '',
  key_type text not null default '',
  serial bigint,
  valid_before timestamptz,
  enrollment_token_hash bytea unique,
  enrollment_expires_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz not null default now()
);

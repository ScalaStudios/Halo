create table saml_keys (
  id text primary key,
  private_key_sealed bytea not null,
  certificate bytea not null,
  created_at timestamptz not null default now(),
  retired_at timestamptz
);

create unique index saml_keys_one_active on saml_keys ((retired_at is null)) where retired_at is null;

create table saml_service_providers (
  app_id text primary key references applications (id) on delete cascade,
  name_id_format text not null default 'email' check (name_id_format in ('email', 'persistent', 'unspecified')),
  sign_response boolean not null default false,
  attributes jsonb not null,
  metadata_xml text not null default '',
  updated_at timestamptz not null default now()
);

create table saml_requests (
  id text primary key,
  request bytea not null,
  relay_state text not null default '',
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);

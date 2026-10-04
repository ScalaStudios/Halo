create table users (
  id text primary key,
  email text not null,
  name text not null,
  title text not null default '',
  department text not null default '',
  location text not null default '',
  status text not null check (status in ('active', 'suspended', 'invited', 'deprovisioned')),
  manager_id text references users (id) on delete set null,
  source text not null default 'Halo',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_sign_in_at timestamptz
);

create unique index users_email_lower on users (lower(email));

create table user_roles (
  user_id text not null references users (id) on delete cascade,
  role text not null check (role in ('global_admin', 'security_admin', 'user_admin', 'helpdesk_admin', 'app_admin', 'auditor')),
  created_at timestamptz not null default now(),
  primary key (user_id, role)
);

create table groups (
  id text primary key,
  name text not null,
  description text not null default '',
  kind text not null check (kind in ('assigned', 'dynamic')),
  rule text,
  source text not null default 'Halo',
  created_at timestamptz not null default now(),
  check ((kind = 'dynamic') = (rule is not null))
);

create unique index groups_name_lower on groups (lower(name));

create table group_members (
  group_id text not null references groups (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (group_id, user_id)
);

create index group_members_user on group_members (user_id);

create table webauthn_credentials (
  id text primary key,
  user_id text not null references users (id) on delete cascade,
  kind text not null check (kind in ('passkey', 'security-key')),
  label text not null,
  credential_id bytea not null unique,
  public_key bytea not null,
  attestation_type text not null default '',
  aaguid bytea,
  sign_count bigint not null default 0,
  transports text[] not null default '{}',
  backup_eligible boolean not null default false,
  backup_state boolean not null default false,
  created_at timestamptz not null default now(),
  last_used_at timestamptz
);

create index webauthn_credentials_user on webauthn_credentials (user_id);

create table totp_secrets (
  id text primary key,
  user_id text not null references users (id) on delete cascade,
  label text not null,
  secret_sealed bytea not null,
  confirmed boolean not null default false,
  last_step bigint not null default 0,
  created_at timestamptz not null default now(),
  last_used_at timestamptz
);

create index totp_secrets_user on totp_secrets (user_id);

create table recovery_codes (
  id text primary key,
  user_id text not null references users (id) on delete cascade,
  code_hash bytea not null unique,
  created_at timestamptz not null default now(),
  used_at timestamptz
);

create index recovery_codes_user on recovery_codes (user_id);

create table enrollment_tokens (
  token_hash bytea primary key,
  user_id text not null references users (id) on delete cascade,
  purpose text not null check (purpose in ('invite', 'reset')),
  created_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  used_at timestamptz
);

create table auth_ceremonies (
  id text primary key,
  kind text not null check (kind in ('passkey-login', 'passkey-register', 'totp-enroll')),
  user_id text references users (id) on delete cascade,
  data jsonb not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);

create table sessions (
  id text primary key,
  token_hash bytea not null unique,
  user_id text not null references users (id) on delete cascade,
  method text not null check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes')),
  ip text not null default '',
  location text not null default '',
  user_agent text not null default '',
  created_at timestamptz not null default now(),
  last_active_at timestamptz not null default now(),
  expires_at timestamptz not null,
  revoked_at timestamptz
);

create index sessions_user on sessions (user_id) where revoked_at is null;

create table applications (
  id text primary key,
  name text not null,
  description text not null default '',
  protocol text not null check (protocol in ('oidc', 'saml', 'oauth')),
  app_type text not null check (app_type in ('web', 'spa', 'native', 'service')),
  status text not null default 'active' check (status in ('active', 'disabled')),
  client_id text not null unique,
  homepage text not null default '',
  redirect_uris text[] not null default '{}',
  post_logout_uris text[] not null default '{}',
  scopes text[] not null default '{openid,profile,email}',
  access_token_ttl integer not null default 900,
  refresh_token_ttl integer not null default 28800,
  id_token_ttl integer not null default 900,
  refresh_rotation boolean not null default true,
  setup_guide text not null default 'generic-oidc',
  owner_id text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table app_credentials (
  id text primary key,
  app_id text not null references applications (id) on delete cascade,
  kind text not null check (kind in ('secret', 'certificate')),
  label text not null,
  secret_hash bytea,
  hint text not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_used_at timestamptz,
  revoked_at timestamptz
);

create index app_credentials_app on app_credentials (app_id) where revoked_at is null;

create table app_groups (
  app_id text not null references applications (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  primary key (app_id, group_id)
);

create table oidc_auth_requests (
  id text primary key,
  client_id text not null,
  redirect_uri text not null,
  scopes text[] not null,
  state text not null default '',
  nonce text not null default '',
  response_type text not null,
  response_mode text not null default '',
  code_challenge text not null default '',
  code_challenge_method text not null default '',
  prompt text[] not null default '{}',
  max_age integer,
  login_hint text not null default '',
  user_id text references users (id) on delete cascade,
  session_id text references sessions (id) on delete set null,
  auth_time timestamptz,
  amr text[] not null default '{}',
  done boolean not null default false,
  code text unique,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null
);

create table oidc_tokens (
  id text primary key,
  kind text not null check (kind in ('access', 'refresh')),
  client_id text not null,
  user_id text references users (id) on delete cascade,
  session_id text references sessions (id) on delete set null,
  refresh_token_hash bytea unique,
  refresh_id text references oidc_tokens (id) on delete cascade,
  scopes text[] not null default '{}',
  audience text[] not null default '{}',
  amr text[] not null default '{}',
  auth_time timestamptz,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  revoked_at timestamptz
);

create index oidc_tokens_user_client on oidc_tokens (user_id, client_id) where revoked_at is null;

create table signing_keys (
  id text primary key,
  algorithm text not null,
  private_key_sealed bytea not null,
  created_at timestamptz not null default now(),
  retired_at timestamptz
);

create table sign_in_events (
  id text primary key,
  time timestamptz not null default now(),
  user_id text references users (id) on delete set null,
  email text not null default '',
  app_id text references applications (id) on delete set null,
  result text not null check (result in ('success', 'failure', 'interrupted')),
  method text not null check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes')),
  ip text not null default '',
  location text not null default '',
  device text not null default '',
  risk text not null default 'none' check (risk in ('none', 'low', 'medium', 'high')),
  reason text not null default ''
);

create index sign_in_events_time on sign_in_events (time desc);
create index sign_in_events_user on sign_in_events (user_id, time desc);

create table audit_events (
  id text primary key,
  time timestamptz not null default now(),
  actor_id text references users (id) on delete set null,
  action text not null,
  summary text not null,
  target_type text not null default '',
  target_id text not null default '',
  target_label text not null default '',
  ip text not null default ''
);

create index audit_events_time on audit_events (time desc);

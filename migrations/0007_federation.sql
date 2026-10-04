create table identity_providers (
  id text primary key,
  kind text not null check (kind in ('google', 'microsoft', 'github', 'oidc')),
  name text not null,
  issuer text not null,
  client_id text not null,
  client_secret_sealed bytea,
  scopes text[] not null default '{}',
  enabled boolean not null default false,
  show_on_sign_in boolean not null default true,
  allowed_domains text[] not null default '{}',
  jit boolean not null default false,
  jit_group_ids text[] not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index identity_providers_name_lower on identity_providers (lower(name));

create table federated_identities (
  id text primary key,
  provider_id text not null references identity_providers (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  subject text not null,
  email text not null default '',
  created_at timestamptz not null default now(),
  last_used_at timestamptz,
  unique (provider_id, subject)
);

create index federated_identities_user on federated_identities (user_id);

create table federation_states (
  state_hash bytea primary key,
  provider_id text not null references identity_providers (id) on delete cascade,
  code_verifier text not null,
  nonce text not null,
  auth_request text not null default '',
  next text not null default '',
  expires_at timestamptz not null
);

alter table sessions drop constraint sessions_method_check,
  add constraint sessions_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated'));

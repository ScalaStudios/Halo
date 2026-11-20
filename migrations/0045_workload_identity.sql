create table federated_credentials (
  id text primary key,
  service_account_id text not null references service_accounts (user_id) on delete cascade,
  name text not null,
  issuer text not null,
  subject text not null,
  audience text not null,
  conditions jsonb not null default '{}',
  scopes text[] not null check (cardinality(scopes) > 0 and scopes <@ array['api', 'scim', 'registration']),
  created_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  last_used_at timestamptz,
  unique (service_account_id, issuer, subject, audience)
);

alter table api_keys add column credential_id text references federated_credentials (id) on delete cascade;
create index api_keys_credential on api_keys (credential_id) where credential_id is not null;

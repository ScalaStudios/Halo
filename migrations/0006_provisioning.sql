alter table users add column kind text not null default 'person' check (kind in ('person', 'service'));
alter table users add column external_id text;
create unique index users_external_id on users (external_id);

alter table groups add column external_id text;
create unique index groups_external_id on groups (external_id);

create table service_accounts (
  user_id text primary key references users (id) on delete cascade,
  description text not null default '',
  owner_id text references users (id) on delete set null,
  created_by text references users (id) on delete set null
);

create table api_keys (
  id text primary key,
  service_account_id text not null references service_accounts (user_id) on delete cascade,
  label text not null,
  prefix text not null,
  key_hash bytea not null unique,
  scopes text[] not null check (cardinality(scopes) > 0 and scopes <@ array['api', 'scim']),
  created_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  expires_at timestamptz,
  last_used_at timestamptz,
  revoked_at timestamptz
);

create index api_keys_service_account on api_keys (service_account_id);

create table app_provisioning (
  app_id text primary key references applications (id) on delete cascade,
  base_url text not null,
  token_sealed bytea not null,
  enabled boolean not null default true,
  last_sync_at timestamptz,
  last_error text not null default '',
  created_count integer not null default 0,
  updated_count integer not null default 0,
  deactivated_count integer not null default 0,
  updated_at timestamptz not null default now()
);

create table app_provisioned_users (
  app_id text not null references app_provisioning (app_id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  remote_id text not null,
  attributes_hash text not null,
  active boolean not null,
  synced_at timestamptz not null default now(),
  primary key (app_id, user_id)
);

create function forbid_service_account_sign_in() returns trigger language plpgsql as $$
begin
  if exists (select 1 from users where id = new.user_id and kind = 'service') then
    raise exception 'service accounts cannot sign in interactively' using errcode = 'check_violation';
  end if;
  return new;
end
$$;

create trigger sessions_people_only before insert on sessions for each row execute function forbid_service_account_sign_in();
create trigger webauthn_credentials_people_only before insert on webauthn_credentials for each row execute function forbid_service_account_sign_in();
create trigger totp_secrets_people_only before insert on totp_secrets for each row execute function forbid_service_account_sign_in();
create trigger recovery_codes_people_only before insert on recovery_codes for each row execute function forbid_service_account_sign_in();
create trigger enrollment_tokens_people_only before insert on enrollment_tokens for each row execute function forbid_service_account_sign_in();

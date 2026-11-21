create table kerberos_source (
  singleton boolean primary key default true check (singleton),
  service_principal text not null,
  realms jsonb not null,
  zone_ids text[] not null default '{}',
  keytab_sealed bytea not null,
  updated_at timestamptz not null default now()
);

create table kerberos_principals (
  principal text primary key,
  user_id text not null references users (id) on delete cascade,
  created_at timestamptz not null default now()
);

create index kerberos_principals_user on kerberos_principals (user_id);

create table kerberos_authenticators (
  digest bytea primary key,
  expires_at timestamptz not null
);

alter table applications add column kerberos_sign_in boolean not null default true;

alter table sessions drop constraint sessions_method_check,
  add constraint sessions_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'certificate', 'kerberos'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'access-pass', 'certificate', 'token', 'kerberos'));

alter table sign_in_method_settings drop constraint sign_in_method_settings_method_check,
  add constraint sign_in_method_settings_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'password', 'certificate', 'kerberos'));

insert into sign_in_method_settings (method, enabled) values ('kerberos', false);

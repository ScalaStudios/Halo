create table application_hooks (
  app_id text primary key references applications (id) on delete cascade,
  host text not null,
  url_sealed bytea not null,
  secret_sealed bytea not null,
  claims text[] not null default '{}',
  fail_open boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table sign_in_events add column hook text not null default '' check (hook in ('', 'allowed', 'denied', 'failed_open', 'failed_closed'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'access-pass', 'certificate', 'token'));

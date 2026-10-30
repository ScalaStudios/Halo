create table passwords (
  user_id text primary key references users (id) on delete cascade,
  hash text not null,
  must_change boolean not null default false,
  created_at timestamptz not null default now(),
  last_used_at timestamptz
);

alter table sessions drop constraint sessions_method_check,
  add constraint sessions_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password'));

alter table sign_in_method_settings drop constraint sign_in_method_settings_method_check,
  add constraint sign_in_method_settings_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'password'));

insert into sign_in_method_settings (method, enabled) values ('password', false);

create table certificate_authorities (
  id text primary key,
  name text not null,
  certificates text not null,
  mapping text not null check (mapping in ('email', 'upn', 'subject')),
  created_at timestamptz not null default now()
);

create unique index certificate_authorities_name_lower on certificate_authorities (lower(name));

alter table sessions drop constraint sessions_method_check,
  add constraint sessions_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'certificate'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'access-pass', 'certificate'));

alter table sign_in_method_settings drop constraint sign_in_method_settings_method_check,
  add constraint sign_in_method_settings_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'password', 'certificate'));

insert into sign_in_method_settings (method, enabled) values ('certificate', false);

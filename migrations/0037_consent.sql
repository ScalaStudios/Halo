alter table applications add column requires_consent boolean not null default false;
update applications set requires_consent = true where registration_hash is not null;

alter table oidc_auth_requests add column consent text check (consent in ('granted', 'denied'));

create table consent_grants (
  user_id text not null references users(id) on delete cascade,
  app_id text not null references applications(id) on delete cascade,
  scopes text[] not null,
  granted_at timestamptz not null default now(),
  primary key (user_id, app_id)
);

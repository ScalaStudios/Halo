create table terms (
  id text primary key,
  name text not null,
  version integer not null default 1,
  text text not null default '',
  url text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index terms_name_lower on terms (lower(name));

create table terms_acceptances (
  user_id text not null references users (id) on delete cascade,
  terms_id text not null,
  name text not null,
  version integer not null,
  accepted_at timestamptz not null default now(),
  primary key (user_id, terms_id, version)
);

alter table access_policies add column terms_id text references terms (id),
  drop constraint access_policies_effect_check,
  add constraint access_policies_effect_check check (effect in ('allow', 'require-phishing-resistant', 'require-terms', 'block')),
  add constraint access_policies_terms_check check ((effect = 'require-terms') = (terms_id is not null));

alter table auth_ceremonies drop constraint auth_ceremonies_kind_check,
  add constraint auth_ceremonies_kind_check check (kind in ('passkey-login', 'passkey-register', 'totp-enroll', 'terms'));

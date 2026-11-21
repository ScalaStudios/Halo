alter table identity_providers drop constraint identity_providers_kind_check,
  add constraint identity_providers_kind_check check (kind in ('google', 'microsoft', 'github', 'oidc', 'saml')),
  add column saml_metadata text not null default '',
  add column email_attribute text not null default '';

create table saml_assertions (
  provider_id text not null references identity_providers (id) on delete cascade,
  id text not null,
  expires_at timestamptz not null,
  primary key (provider_id, id)
);

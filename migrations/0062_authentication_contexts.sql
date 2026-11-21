create table authentication_contexts (
  id text primary key,
  name text not null,
  methods text[] not null default '{}',
  max_age integer not null default 0 check (max_age >= 0),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table oidc_auth_requests add column acr_values text[] not null default '{}',
  add column acr text not null default '';

alter table access_policies add column context_id text references authentication_contexts (id),
  drop constraint access_policies_effect_check,
  add constraint access_policies_effect_check check (effect in ('allow', 'require-phishing-resistant', 'require-terms', 'require-context', 'block')),
  add constraint access_policies_context_check check ((effect = 'require-context') = (context_id is not null));

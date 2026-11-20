alter table applications add column token_exchange boolean not null default false;
alter table oidc_tokens add column subject_token_id text;

create table app_exchange_sources (
  app_id text not null references applications (id) on delete cascade,
  resource_id text not null references api_resources (id) on delete cascade,
  primary key (app_id, resource_id)
);

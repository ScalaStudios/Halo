alter table applications
  add column token_endpoint_auth_method text not null default 'client_secret_basic' check (token_endpoint_auth_method in ('client_secret_basic', 'private_key_jwt')),
  add column jwks jsonb,
  add column jwks_uri text not null default '',
  add column fapi2_profile boolean not null default false;

alter table oidc_auth_requests add column dpop_jkt text not null default '';

create table client_assertions (
  client_id text not null,
  jti text not null,
  expires_at timestamptz not null,
  primary key (client_id, jti)
);

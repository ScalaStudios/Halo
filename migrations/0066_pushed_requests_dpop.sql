alter table applications
  add column require_par boolean not null default false,
  add column require_pkce boolean not null default false,
  add column require_dpop boolean not null default false;

alter table oidc_tokens add column dpop_jkt text not null default '';

create table oidc_pushed_requests (
  request_hash bytea primary key,
  client_id text not null,
  params text not null,
  expires_at timestamptz not null
);

create table dpop_proofs (
  jkt text not null,
  jti text not null,
  expires_at timestamptz not null,
  primary key (jkt, jti)
);

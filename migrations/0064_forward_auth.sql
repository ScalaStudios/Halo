alter table applications drop constraint applications_protocol_check;
alter table applications add constraint applications_protocol_check check (protocol in ('oidc', 'saml', 'oauth', 'bookmark', 'forward-auth'));

create table forward_auth_sessions (
  token_hash bytea primary key,
  app_id text not null references applications (id) on delete cascade,
  session_id text not null references sessions (id) on delete cascade,
  state_hash bytea not null,
  return_to text not null,
  exchanged boolean not null default false,
  expires_at timestamptz not null
);

create index forward_auth_sessions_session on forward_auth_sessions (session_id);

alter table applications drop constraint applications_protocol_check;
alter table applications add constraint applications_protocol_check check (protocol in ('oidc', 'saml', 'oauth', 'bookmark', 'forward-auth', 'radius', 'ldap', 'remote'));

create table outposts (
  id text primary key,
  name text not null unique,
  key_hash bytea unique,
  enrollment_token_hash bytea unique,
  enrollment_expires_at timestamptz,
  enrolled_at timestamptz,
  last_seen_at timestamptz,
  last_ip text not null default '',
  version text not null default '',
  created_at timestamptz not null default now()
);

create table remote_targets (
  app_id text primary key references applications (id) on delete cascade,
  outpost_id text not null references outposts (id),
  protocol text not null check (protocol in ('ssh')),
  host text not null,
  port integer not null check (port between 1 and 65535),
  username text not null default '',
  auth text not null check (auth in ('certificate', 'password', 'key')),
  credential_sealed bytea,
  host_key text not null default '',
  session_limit integer not null default 3600 check (session_limit between 1 and 86400),
  record boolean not null default false
);

create table remote_sessions (
  id text primary key,
  time timestamptz not null default now(),
  app_id text references applications (id) on delete set null,
  app_name text not null,
  user_id text references users (id) on delete set null,
  email text not null,
  session_id text not null,
  outpost_id text references outposts (id) on delete set null,
  protocol text not null,
  address text not null,
  username text not null,
  ip text not null default '',
  limit_seconds integer not null,
  record boolean not null,
  status text not null default 'pending' check (status in ('pending', 'active', 'ended')),
  started_at timestamptz,
  seen_at timestamptz,
  ended_at timestamptz,
  end_reason text not null default '',
  bytes_in bigint not null default 0,
  bytes_out bigint not null default 0,
  recording_sealed bytea,
  terminated_by text references users (id) on delete set null
);

create index remote_sessions_time on remote_sessions (time desc);
create index remote_sessions_app on remote_sessions (app_id, time desc);

create table ldap_directories (
  id text primary key,
  name text not null unique,
  url text not null,
  security text not null check (security in ('ldaps', 'starttls', 'none')),
  ca_certificate text not null default '',
  bind_dn text not null,
  password_sealed bytea not null,
  user_base_dn text not null,
  user_filter text not null,
  group_base_dn text not null default '',
  group_filter text not null default '',
  mapping jsonb not null,
  enabled boolean not null default false,
  interval_minutes integer not null default 60 check (interval_minutes between 15 and 1440),
  synced_at timestamptz,
  sync_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table ldap_people (
  directory_id text not null references ldap_directories (id) on delete cascade,
  entry_id text not null,
  user_id text not null unique references users (id) on delete cascade,
  dn text not null,
  suspended boolean not null default false,
  primary key (directory_id, entry_id)
);

create table ldap_groups (
  directory_id text not null references ldap_directories (id) on delete cascade,
  entry_id text not null,
  group_id text not null unique references groups (id) on delete cascade,
  primary key (directory_id, entry_id)
);

create table ldap_syncs (
  id text primary key,
  directory_id text not null references ldap_directories (id) on delete cascade,
  started_at timestamptz not null,
  finished_at timestamptz not null,
  result text not null check (result in ('succeeded', 'failed')),
  error text not null default '',
  counts jsonb not null default '{}',
  skipped text[] not null default '{}'
);

create index ldap_syncs_directory on ldap_syncs (directory_id, started_at desc);

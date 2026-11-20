alter table applications drop constraint applications_protocol_check;
alter table applications add constraint applications_protocol_check check (protocol in ('oidc', 'saml', 'oauth', 'bookmark'));

create table collections (
  id text primary key,
  name text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index collections_name_lower on collections (lower(name));

create table collection_apps (
  collection_id text not null references collections (id) on delete cascade,
  app_id text not null references applications (id) on delete cascade,
  primary key (collection_id, app_id)
);

create table collection_groups (
  collection_id text not null references collections (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  primary key (collection_id, group_id)
);

create table app_preferences (
  user_id text not null references users (id) on delete cascade,
  app_id text not null references applications (id) on delete cascade,
  position integer,
  favourite boolean not null default false,
  primary key (user_id, app_id)
);

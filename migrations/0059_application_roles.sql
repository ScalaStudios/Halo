create table app_roles (
  id text primary key,
  app_id text not null references applications (id) on delete cascade,
  key text not null,
  name text not null,
  description text not null default '',
  created_at timestamptz not null default now(),
  unique (app_id, key)
);

create table app_role_groups (
  role_id text not null references app_roles (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  primary key (role_id, group_id)
);

create table app_role_users (
  role_id text not null references app_roles (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  primary key (role_id, user_id)
);

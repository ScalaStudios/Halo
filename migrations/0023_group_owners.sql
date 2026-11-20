alter table groups add column join_policy text not null default 'closed' check (join_policy in ('closed', 'request', 'open')),
  add constraint groups_join_policy_assigned check (kind = 'assigned' or join_policy = 'closed');

create table group_owners (
  group_id text not null references groups (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (group_id, user_id)
);

create index group_owners_user on group_owners (user_id);

create table group_join_requests (
  group_id text not null references groups (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (group_id, user_id)
);

create table authz_models (
  app_id text primary key references applications (id) on delete cascade,
  types jsonb not null,
  updated_at timestamptz not null default now()
);

create table authz_tuples (
  app_id text not null references applications (id) on delete cascade,
  object_type text not null,
  object_id text not null,
  relation text not null,
  subject_type text not null,
  subject_id text not null,
  subject_relation text not null default '',
  user_id text generated always as (case when subject_type = 'user' then subject_id end) stored references users (id) on delete cascade,
  group_id text generated always as (case when subject_type = 'group' then subject_id end) stored references groups (id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (app_id, object_type, object_id, relation, subject_type, subject_id, subject_relation)
);

create index authz_tuples_subject on authz_tuples (app_id, subject_type, subject_id, subject_relation);
create index authz_tuples_user on authz_tuples (user_id) where user_id is not null;
create index authz_tuples_group on authz_tuples (group_id) where group_id is not null;

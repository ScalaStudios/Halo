alter table app_provisioning add column group_ids text[] not null default '{}';
alter table app_provisioning add column groups_created_count integer not null default 0;
alter table app_provisioning add column groups_updated_count integer not null default 0;
alter table app_provisioning add column groups_deleted_count integer not null default 0;

create table app_provisioned_groups (
  app_id text not null references app_provisioning (app_id) on delete cascade,
  group_id text not null,
  remote_id text not null,
  display_name text not null,
  members text[] not null default '{}',
  synced_at timestamptz not null default now(),
  primary key (app_id, group_id)
);

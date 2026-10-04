alter table users add column email_verified boolean not null default false;

alter table enrollment_tokens add column emailed boolean not null default false;

delete from recovery_codes;

alter table sessions add column device_hash bytea, add column client_id text;

alter table device_authorizations add column method text, add column last_polled_at timestamptz;

create index device_authorizations_ip on device_authorizations (ip, created_at);

alter table access_reviews add column group_name text not null default '',
  alter column group_id drop not null,
  drop constraint access_reviews_group_id_fkey,
  add constraint access_reviews_group_id_fkey foreign key (group_id) references groups (id) on delete set null;

create index app_groups_group on app_groups (group_id);

create index sign_in_events_app on sign_in_events (app_id, time desc);

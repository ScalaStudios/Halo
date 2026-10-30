alter table applications add column logo_updated_at timestamptz;

create table app_logos (
  app_id text primary key references applications (id) on delete cascade,
  content_type text not null,
  data bytea not null
);

alter table users add column avatar_updated_at timestamptz;

create table user_avatars (
  user_id text primary key references users (id) on delete cascade,
  content_type text not null,
  data bytea not null
);

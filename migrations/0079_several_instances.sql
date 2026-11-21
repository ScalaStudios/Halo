create function notify_settings_changed() returns trigger language plpgsql as $$
begin
  perform pg_notify('halo_settings', '');
  return null;
end
$$;

create trigger settings_changed after insert or update or delete or truncate on settings for each statement execute function notify_settings_changed();

create table radius_replies (
  key bytea primary key,
  reply bytea,
  created_at timestamptz not null default now()
);

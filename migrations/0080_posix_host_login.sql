create table posix_settings (
  id boolean primary key default true check (id),
  uid_min integer not null default 1000000,
  uid_max integer not null default 1999999,
  gid_min integer not null default 2000000,
  gid_max integer not null default 2999999,
  login_shell text not null default '/bin/bash',
  check (uid_min >= 65536 and uid_min <= uid_max and gid_min >= 65536 and gid_min <= gid_max and (uid_max < gid_min or gid_max < uid_min))
);

insert into posix_settings default values;

create function posix_name_ok(name text) returns boolean language sql immutable as $$
  select name ~ '^[a-z_][a-z0-9_.-]{0,31}$' and name <> all (array['root', 'daemon', 'bin', 'sys', 'sync', 'games', 'man', 'lp', 'mail', 'news', 'uucp', 'proxy', 'www-data', 'backup', 'list', 'irc', 'gnats', 'nobody', 'nogroup', 'nfsnobody', 'adm', 'admin', 'wheel', 'sudo', 'staff', 'users', 'shadow', 'utmp', 'tty', 'disk', 'kmem', 'dialout', 'cdrom', 'floppy', 'audio', 'video', 'plugdev', 'netdev', 'input', 'render', 'docker', 'lxd', 'sshd', 'halt', 'shutdown', 'operator', 'ftp', 'polkitd', 'messagebus', 'syslog', 'postfix', 'systemd-network', 'systemd-resolve', 'systemd-timesync', 'systemd-journal'])
$$;

create table posix_ids (
  user_id text unique references users (id) on delete cascade,
  group_id text unique references groups (id) on delete cascade,
  name text unique check (posix_name_ok(name)),
  number integer not null unique,
  login_shell text,
  home_directory text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (user_id is null or group_id is null),
  check ((user_id is null and group_id is null) = (name is null))
);

create function posix_allocate(owner_user text, owner_group text, wanted text) returns void language plpgsql as $$
declare
  s posix_settings;
  low integer;
  high integer;
  n integer;
  base text;
  candidate text;
  i integer := 1;
begin
  select * into s from posix_settings for update;
  if owner_user is not null then
    low := s.uid_min;
    high := s.uid_max;
  else
    low := s.gid_min;
    high := s.gid_max;
  end if;
  n := coalesce((select max(number) + 1 from posix_ids where number between low and high), low);
  if n > high then
    return;
  end if;
  base := trim(both '-' from regexp_replace(lower(coalesce(wanted, '')), '[^a-z0-9_.-]+', '-', 'g'));
  if base !~ '^[a-z_]' then
    base := case when owner_user is not null then 'u' else 'g' end || base;
  end if;
  base := left(base, 32);
  candidate := base;
  loop
    if posix_name_ok(candidate) then
      insert into posix_ids (user_id, group_id, name, number, login_shell, home_directory)
      values (owner_user, owner_group, candidate, n, case when owner_user is not null then s.login_shell end, case when owner_user is not null then '/home/' || candidate end)
      on conflict (name) do nothing;
      exit when found;
    end if;
    i := i + 1;
    candidate := left(base, 32 - length(i::text)) || i;
  end loop;
end
$$;

create function posix_allocate_user() returns trigger language plpgsql as $$
begin
  perform posix_allocate(new.id, null, split_part(new.email, '@', 1));
  return null;
end
$$;

create function posix_allocate_group() returns trigger language plpgsql as $$
begin
  perform posix_allocate(null, new.id, new.name);
  return null;
end
$$;

create function posix_retire() returns trigger language plpgsql as $$
begin
  update posix_ids set user_id = null, group_id = null, name = null, login_shell = null, home_directory = null, updated_at = now() where user_id = old.id or group_id = old.id;
  return old;
end
$$;

create trigger users_posix after insert on users for each row when (new.kind = 'person') execute function posix_allocate_user();
create trigger groups_posix after insert on groups for each row execute function posix_allocate_group();
create trigger users_posix_retire before delete on users for each row execute function posix_retire();
create trigger groups_posix_retire before delete on groups for each row execute function posix_retire();

create function posix_backfill() returns void language plpgsql as $$
declare
  r record;
begin
  for r in select id, email from users u where kind = 'person' and not exists (select 1 from posix_ids p where p.user_id = u.id) order by created_at, id loop
    perform posix_allocate(r.id, null, split_part(r.email, '@', 1));
  end loop;
  for r in select id, name from groups g where not exists (select 1 from posix_ids p where p.group_id = g.id) order by created_at, id loop
    perform posix_allocate(null, r.id, r.name);
  end loop;
end
$$;

select posix_backfill();

create table host_groups (
  id text primary key,
  name text not null,
  description text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index host_groups_name on host_groups (lower(name));

create table host_group_rules (
  host_group_id text not null references host_groups (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  sudo text[] not null default '{}',
  primary key (host_group_id, group_id)
);

alter table ssh_hosts add column host_group_id text references host_groups (id) on delete set null;

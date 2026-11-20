alter table users
  add column guest boolean not null default false,
  add column sponsor_id text references users (id) on delete set null,
  add column expires_at timestamptz,
  add column expiry_notice_at timestamptz,
  add constraint users_guest_expiry check (not guest or expires_at is not null);

create index users_guest_expiry on users (expires_at) where guest;

create function forbid_guest_roles() returns trigger language plpgsql as $$
begin
  if exists (select 1 from users where id = new.user_id and guest) then
    raise exception 'guests cannot hold administrator roles' using errcode = 'check_violation';
  end if;
  return new;
end
$$;

create trigger user_roles_no_guests before insert on user_roles for each row execute function forbid_guest_roles();
create trigger role_eligibilities_no_guests before insert on role_eligibilities for each row execute function forbid_guest_roles();

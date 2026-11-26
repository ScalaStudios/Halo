alter table ldap_directories add column configured_by text references users (id) on delete set null;

update ldap_directories d
set configured_by = (
  select a.actor_id from audit_events a
  where a.target_type = 'ldap_directory' and a.target_id = d.id and a.action in ('ldap_directory.create', 'ldap_directory.update') and a.actor_id is not null
  order by a.time desc limit 1
);

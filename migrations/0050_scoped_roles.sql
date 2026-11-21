alter table user_roles
  add column scope text[],
  add constraint user_roles_scope check (scope is null or (role in ('user_admin', 'helpdesk_admin') and cardinality(scope) > 0));

alter table role_eligibilities
  add column scope text[],
  add constraint role_eligibilities_scope check (scope is null or (role in ('user_admin', 'helpdesk_admin') and cardinality(scope) > 0));

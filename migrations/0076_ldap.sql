alter table applications drop constraint applications_protocol_check;
alter table applications add constraint applications_protocol_check check (protocol in ('oidc', 'saml', 'oauth', 'bookmark', 'forward-auth', 'radius', 'ldap'));
alter table applications add column ldap_secret_sealed bytea;

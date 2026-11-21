alter table applications drop constraint applications_protocol_check;
alter table applications add constraint applications_protocol_check check (protocol in ('oidc', 'saml', 'oauth', 'bookmark', 'forward-auth', 'radius'));

alter table auth_ceremonies drop constraint auth_ceremonies_kind_check,
  add constraint auth_ceremonies_kind_check check (kind in ('passkey-login', 'passkey-register', 'totp-enroll', 'terms', 'radius'));

alter table app_credentials add column secret_sealed bytea;

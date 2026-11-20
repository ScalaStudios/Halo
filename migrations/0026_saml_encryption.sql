alter table saml_service_providers
  add column encryption text not null default 'off' check (encryption in ('off', 'aes256-gcm', 'aes256-cbc')),
  add column encryption_certificate text not null default '';

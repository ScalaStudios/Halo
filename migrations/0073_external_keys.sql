alter table signing_keys alter column private_key_sealed drop not null,
  add column external_key text,
  add check ((private_key_sealed is null) <> (external_key is null));

alter table saml_keys alter column private_key_sealed drop not null,
  add column external_key text,
  add check ((private_key_sealed is null) <> (external_key is null));

alter table ssh_authority alter column private_key_sealed drop not null,
  add column external_key text,
  add check ((private_key_sealed is null) <> (external_key is null));

alter table ssh_host_authority alter column private_key_sealed drop not null,
  add column external_key text,
  add check ((private_key_sealed is null) <> (external_key is null));

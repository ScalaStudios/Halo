alter table applications add column registration_hash bytea;
alter table applications add column awaiting_approval boolean not null default false;

alter table api_keys drop constraint api_keys_scopes_check;
alter table api_keys add constraint api_keys_scopes_check check (cardinality(scopes) > 0 and scopes <@ array['api', 'scim', 'registration']);

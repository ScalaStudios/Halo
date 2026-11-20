alter table applications add column token_exchange boolean not null default false;
alter table oidc_tokens add column subject_token_id text;

alter table applications add column id_token_alg text not null default 'RS256' check (id_token_alg in ('RS256', 'ES256', 'EdDSA'));
alter table api_resources add column access_token_alg text not null default 'RS256' check (access_token_alg in ('RS256', 'ES256', 'EdDSA'));

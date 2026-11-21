alter table identity_providers add column mapping jsonb not null default '{"profile": [], "groupClaim": "", "groups": []}';

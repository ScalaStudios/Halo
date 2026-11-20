alter table applications add column max_auth_age integer not null default 0 check (max_auth_age >= 0);

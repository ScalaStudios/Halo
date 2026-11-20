alter table magic_links add column code_hash bytea,
  add column code_used_at timestamptz,
  add column attempts integer not null default 0;

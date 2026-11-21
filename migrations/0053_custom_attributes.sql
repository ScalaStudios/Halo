create table user_attributes (
  id text primary key,
  name text not null unique,
  label text not null,
  type text not null check (type in ('string', 'number', 'boolean', 'date', 'list')),
  self_service boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table users add column attributes jsonb not null default '{}';
alter table applications add column claim_mappings jsonb not null default '[]';

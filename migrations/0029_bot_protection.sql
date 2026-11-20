create table bot_protection (
  singleton boolean primary key default true check (singleton),
  secret_sealed bytea not null,
  updated_at timestamptz not null default now()
);

alter table sign_in_events add column challenge text not null default '' check (challenge in ('', 'passed', 'failed', 'unavailable'));

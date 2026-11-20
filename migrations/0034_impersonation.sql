create table impersonations (
  id text primary key,
  actor_id text not null references users (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  reason text not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  started_at timestamptz
);

create index impersonations_user on impersonations (user_id, created_at desc);

alter table sessions add column impersonation_id text references impersonations (id) on delete cascade;

alter table sign_in_events add column impersonator_id text references users (id) on delete set null;

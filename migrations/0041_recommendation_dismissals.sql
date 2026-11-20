create table recommendation_dismissals (
  check_id text primary key,
  reason text not null,
  dismissed_by text references users (id) on delete set null,
  dismissed_at timestamptz not null default now()
);

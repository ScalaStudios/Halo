create table approval_requests (
  id text primary key,
  action text not null,
  summary text not null,
  target_type text not null,
  target_id text not null default '',
  target_label text not null,
  method text not null,
  path text not null,
  body_sealed bytea not null,
  state text not null,
  requested_by text not null references users (id) on delete cascade,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  status text not null default 'pending' check (status in ('pending', 'approved', 'rejected', 'failed')),
  decided_by text references users (id) on delete set null,
  decided_at timestamptz,
  outcome text not null default ''
);

create index approval_requests_created on approval_requests (created_at desc);

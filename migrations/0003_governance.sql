create table access_packages (
  id text primary key,
  name text not null,
  description text not null default '',
  owner_id text references users (id) on delete set null,
  max_days integer check (max_days between 1 and 365),
  require_justification boolean not null default true,
  archived boolean not null default false,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index access_packages_name_lower on access_packages (lower(name));

create table access_package_groups (
  package_id text not null references access_packages (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  primary key (package_id, group_id)
);

create table access_package_approvers (
  package_id text not null references access_packages (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  primary key (package_id, user_id)
);

create table access_requests (
  id text primary key,
  package_id text not null references access_packages (id),
  requester_id text not null references users (id) on delete cascade,
  justification text not null default '',
  duration_days integer check (duration_days between 1 and 365),
  status text not null default 'pending' check (status in ('pending', 'approved', 'denied', 'cancelled')),
  decided_by text references users (id) on delete set null,
  decided_at timestamptz,
  decision_note text not null default '',
  expires_at timestamptz,
  ended_at timestamptz,
  end_reason text check (end_reason in ('expired', 'revoked')),
  created_at timestamptz not null default now()
);

create index access_requests_status on access_requests (status, created_at desc);
create index access_requests_requester on access_requests (requester_id, created_at desc);
create unique index access_requests_one_pending on access_requests (package_id, requester_id) where status = 'pending';
create index access_requests_active_grants on access_requests (expires_at) where status = 'approved' and ended_at is null;

create table access_grant_groups (
  request_id text not null references access_requests (id) on delete cascade,
  group_id text not null references groups (id) on delete cascade,
  added boolean not null,
  primary key (request_id, group_id)
);

create table access_reviews (
  id text primary key,
  name text not null,
  group_id text not null references groups (id) on delete cascade,
  due_at timestamptz not null,
  auto_apply boolean not null default false,
  status text not null default 'in-progress' check (status in ('in-progress', 'overdue', 'completed')),
  created_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  completed_by text references users (id) on delete set null,
  completed_at timestamptz
);

create table access_review_reviewers (
  review_id text not null references access_reviews (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  primary key (review_id, user_id)
);

create table access_review_items (
  review_id text not null references access_reviews (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  decision text check (decision in ('keep', 'remove')),
  note text not null default '',
  decided_by text references users (id) on delete set null,
  decided_at timestamptz,
  outcome text check (outcome in ('kept', 'removed', 'not-removed', 'no-decision')),
  primary key (review_id, user_id)
);

create table lifecycle_rules (
  id text primary key,
  name text not null,
  trigger text not null check (trigger in ('joiner', 'mover', 'leaver')),
  condition text,
  actions jsonb not null,
  enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table lifecycle_snapshots (
  user_id text primary key references users (id) on delete cascade,
  department text not null,
  title text not null,
  location text not null,
  status text not null
);

insert into lifecycle_snapshots (user_id, department, title, location, status) select id, department, title, location, status from users;

create table lifecycle_runs (
  id text primary key,
  rule_id text not null references lifecycle_rules (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  trigger text not null,
  change text not null,
  steps text[] not null,
  result text not null check (result in ('succeeded', 'failed')),
  created_at timestamptz not null default now()
);

create index lifecycle_runs_time on lifecycle_runs (created_at desc);

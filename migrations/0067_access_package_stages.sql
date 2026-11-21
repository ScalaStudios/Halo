create table access_package_stages (
  package_id text not null references access_packages (id) on delete cascade,
  stage integer not null check (stage between 1 and 3),
  approver text not null default 'people' check (approver in ('people', 'manager', 'group-owners')),
  group_id text references groups (id) on delete set null,
  timeout_days integer check (timeout_days between 1 and 30),
  primary key (package_id, stage)
);

insert into access_package_stages (package_id, stage) select id, 1 from access_packages;

alter table access_package_approvers
  add column stage integer not null default 1 check (stage between 1 and 3),
  add column fallback boolean not null default false,
  drop constraint access_package_approvers_pkey,
  add primary key (package_id, stage, fallback, user_id);

alter table access_packages add column questions jsonb not null default '[]';

alter table access_requests
  add column requested_by text references users (id) on delete set null,
  add column extends_id text references access_requests (id) on delete set null,
  add column answers jsonb not null default '[]',
  add column stage integer not null default 1,
  add column stage_started_at timestamptz not null default now(),
  add column escalated boolean not null default false,
  drop constraint access_requests_status_check,
  add constraint access_requests_status_check check (status in ('pending', 'approved', 'denied', 'cancelled', 'expired')),
  drop constraint access_requests_end_reason_check,
  add constraint access_requests_end_reason_check check (end_reason in ('expired', 'revoked', 'extended'));

update access_requests set stage_started_at = created_at;

create index access_requests_requested_by on access_requests (requested_by, created_at desc) where requested_by is not null;

create table access_request_approvals (
  request_id text not null references access_requests (id) on delete cascade,
  stage integer not null,
  decided_by text references users (id) on delete set null,
  note text not null default '',
  decided_at timestamptz not null,
  primary key (request_id, stage)
);

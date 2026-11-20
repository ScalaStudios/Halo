create table role_eligibilities (
  user_id text not null references users (id) on delete cascade,
  role text not null check (role in ('global_admin', 'security_admin', 'user_admin', 'helpdesk_admin', 'app_admin', 'auditor')),
  created_at timestamptz not null default now(),
  primary key (user_id, role)
);

create table role_activations (
  id text primary key,
  user_id text not null references users (id) on delete cascade,
  role text not null check (role in ('global_admin', 'security_admin', 'user_admin', 'helpdesk_admin', 'app_admin', 'auditor')),
  justification text not null,
  expires_at timestamptz not null,
  ended_at timestamptz,
  end_reason text check (end_reason in ('expired', 'deactivated', 'revoked')),
  created_at timestamptz not null default now()
);

create unique index role_activations_one_open on role_activations (user_id, role) where ended_at is null;
create index role_activations_open_expiry on role_activations (expires_at) where ended_at is null;

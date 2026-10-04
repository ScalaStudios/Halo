create table access_policies (
  id text primary key,
  name text not null,
  description text not null default '',
  priority integer not null,
  enabled boolean not null default true,
  mode text not null check (mode in ('enforce', 'report')),
  effect text not null check (effect in ('allow', 'require-phishing-resistant', 'block')),
  conditions jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index access_policies_name_lower on access_policies (lower(name));

create table network_zones (
  id text primary key,
  name text not null,
  kind text not null check (kind in ('trusted', 'risky')),
  cidrs text[] not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index network_zones_name_lower on network_zones (lower(name));

create table sign_in_method_settings (
  method text primary key check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes')),
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);

insert into sign_in_method_settings (method) values ('passkey'), ('security-key'), ('totp'), ('magic-link'), ('recovery-codes');

create table devices (
  id text primary key,
  user_id text not null references users (id) on delete cascade,
  token_hash bytea not null,
  user_agent text not null default '',
  trust text not null default 'unknown' check (trust in ('unknown', 'trusted', 'blocked')),
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  last_ip text not null default '',
  sign_in_count integer not null default 0,
  unique (user_id, token_hash)
);

create table risk_events (
  id text primary key,
  time timestamptz not null default now(),
  type text not null check (type in ('risky-network', 'failed-sign-ins', 'new-device')),
  level text not null check (level in ('medium', 'high')),
  user_id text references users (id) on delete set null,
  ip text not null default '',
  device_id text references devices (id) on delete set null,
  sign_in_id text,
  detail text not null default '',
  status text not null default 'open' check (status in ('open', 'resolved', 'dismissed')),
  resolved_by text references users (id) on delete set null,
  resolved_at timestamptz
);

create index risk_events_time on risk_events (time desc);

create table policy_evaluations (
  sign_in_id text primary key,
  time timestamptz not null default now(),
  risk text not null check (risk in ('none', 'low', 'medium', 'high')),
  effect text not null,
  matches jsonb not null default '[]'
);

create index policy_evaluations_time on policy_evaluations (time desc);

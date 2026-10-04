create table settings (
  key text primary key,
  value jsonb not null,
  updated_at timestamptz not null default now()
);

create table branding_logo (
  singleton boolean primary key default true check (singleton),
  content_type text not null check (content_type in ('image/png', 'image/jpeg', 'image/webp')),
  data bytea not null check (octet_length(data) <= 262144),
  updated_at timestamptz not null default now()
);

create table domains (
  id text primary key,
  name text not null,
  token text not null,
  created_at timestamptz not null default now(),
  verified_at timestamptz
);

create unique index domains_name_lower on domains (lower(name));

create table webhook_endpoints (
  id text primary key,
  url text not null,
  description text not null default '',
  events text[] not null check (cardinality(events) > 0),
  secret_sealed bytea not null,
  enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table webhook_deliveries (
  id text primary key,
  endpoint_id text not null references webhook_endpoints (id) on delete cascade,
  event_id text not null,
  event_type text not null,
  body text not null,
  status text not null default 'pending' check (status in ('pending', 'delivered', 'failed')),
  attempts integer not null default 0,
  next_attempt_at timestamptz not null default now(),
  last_attempt_at timestamptz,
  response_status integer,
  error text not null default '',
  created_at timestamptz not null default now()
);

create unique index webhook_deliveries_event on webhook_deliveries (endpoint_id, event_id);
create index webhook_deliveries_due on webhook_deliveries (next_attempt_at) where status = 'pending';
create index webhook_deliveries_endpoint on webhook_deliveries (endpoint_id, created_at desc);

create table webhook_cursors (
  source text primary key check (source in ('audit', 'sign_in')),
  time timestamptz not null,
  id text not null
);

insert into webhook_cursors (source, time, id) values ('audit', now(), ''), ('sign_in', now(), '');

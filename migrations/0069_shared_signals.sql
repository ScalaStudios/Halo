create table ssf_receivers (
  app_id text primary key references applications (id) on delete cascade,
  subject_format text not null default 'iss_sub' check (subject_format in ('iss_sub', 'email', 'opaque')),
  created_at timestamptz not null default now()
);

create table ssf_streams (
  id text primary key,
  app_id text not null unique references ssf_receivers (app_id) on delete cascade,
  description text not null default '',
  events_requested text[] not null default '{}',
  delivery_method text not null check (delivery_method in ('urn:ietf:rfc:8935', 'urn:ietf:rfc:8936')),
  endpoint_url text not null default '',
  authorization_sealed bytea,
  status text not null default 'enabled' check (status in ('enabled', 'paused', 'disabled')),
  reason text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

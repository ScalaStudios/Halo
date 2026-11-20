create table sign_ups (
  id text primary key,
  user_id text not null unique,
  email text not null,
  name text not null,
  token_hash bytea unique,
  expires_at timestamptz not null,
  credential jsonb,
  verified_at timestamptz,
  created_at timestamptz not null default now()
);

create unique index sign_ups_email on sign_ups (lower(email));

alter table users add column awaiting_activation boolean not null default false;

update users set awaiting_activation = true
where kind = 'person' and (status = 'invited' or (status in ('suspended', 'deprovisioned') and last_sign_in_at is null));

create table identity_proofs (
  id text primary key,
  user_id text references users (id) on delete cascade,
  sign_up_id text references sign_ups (id) on delete cascade,
  method text not null check (method in ('in-person', 'provider')),
  provider text not null default '',
  reference text not null default '',
  status text not null check (status in ('pending', 'verified', 'failed')),
  document text not null default '',
  note text not null default '',
  checked_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  completed_at timestamptz,
  check (num_nonnulls(user_id, sign_up_id) = 1)
);

create index identity_proofs_user on identity_proofs (user_id);
create index identity_proofs_sign_up on identity_proofs (sign_up_id);
create unique index identity_proofs_reference on identity_proofs (provider, reference) where reference <> '';

create table identity_proofing_secrets (
  singleton boolean primary key default true check (singleton),
  api_key_sealed bytea not null,
  webhook_secret_sealed bytea not null,
  updated_at timestamptz not null default now()
);

create table identity_proofing_events (
  id text primary key,
  expires_at timestamptz not null
);

create table access_passes (
  user_id text primary key references users (id) on delete cascade,
  pass_hash bytea not null,
  uses_left integer not null check (uses_left >= 0),
  expires_at timestamptz not null,
  created_by text references users (id) on delete set null,
  created_at timestamptz not null default now(),
  last_used_at timestamptz
);

create trigger access_passes_people_only before insert on access_passes for each row execute function forbid_service_account_sign_in();

alter table enrollment_tokens drop constraint enrollment_tokens_purpose_check,
  add constraint enrollment_tokens_purpose_check check (purpose in ('invite', 'reset', 'access-pass'));

alter table sign_in_events drop constraint sign_in_events_method_check,
  add constraint sign_in_events_method_check check (method in ('passkey', 'security-key', 'totp', 'magic-link', 'recovery-codes', 'federated', 'password', 'access-pass'));

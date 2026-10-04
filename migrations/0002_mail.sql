create table mail_outbox (
  id text primary key,
  recipient text not null,
  subject text not null,
  body_sealed bytea not null,
  attempts integer not null default 0,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  next_attempt_at timestamptz not null default now(),
  sent_at timestamptz
);

create index mail_outbox_due on mail_outbox (next_attempt_at) where sent_at is null;
create index mail_outbox_created on mail_outbox (created_at desc);

create table magic_links (
  token_hash bytea primary key,
  user_id text not null references users (id) on delete cascade,
  auth_request text,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  used_at timestamptz
);

create index magic_links_user on magic_links (user_id, created_at desc);

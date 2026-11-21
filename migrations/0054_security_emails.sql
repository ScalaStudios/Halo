create table security_email_sends (
  user_id text not null references users (id) on delete cascade,
  kind text not null,
  subject text not null,
  sent_at timestamptz not null,
  primary key (user_id, kind, subject)
);

create table security_email_opt_outs (
  user_id text not null references users (id) on delete cascade,
  kind text not null,
  primary key (user_id, kind)
);

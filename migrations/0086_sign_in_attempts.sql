create table sign_in_attempts (
  id text primary key,
  subject text not null,
  time timestamptz not null default now()
);

create index sign_in_attempts_subject on sign_in_attempts (subject, time);

alter table sign_in_events add column mismatch boolean not null default false;

alter table service_accounts add column rate_limit integer not null default 600 check (rate_limit between 1 and 100000);

create unlogged table rate_limit_windows (
  subject text not null,
  scope text not null,
  window_start timestamptz not null,
  requests integer not null,
  rejected integer not null,
  primary key (subject, scope, window_start)
);

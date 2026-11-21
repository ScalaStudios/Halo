create table log_export (
  singleton boolean primary key default true check (singleton),
  protocol text not null check (protocol in ('syslog-tls', 'syslog-udp', 'otlp-http')),
  endpoint text not null,
  allow_private boolean not null default false,
  headers_sealed bytea,
  failures integer not null default 0,
  next_attempt_at timestamptz not null default now(),
  last_sent_at timestamptz,
  last_error text not null default '',
  updated_at timestamptz not null default now()
);

alter table webhook_cursors drop constraint webhook_cursors_source_check;
alter table webhook_cursors add constraint webhook_cursors_source_check check (source in ('audit', 'sign_in', 'log_audit', 'log_sign_in'));
insert into webhook_cursors (source, time, id) values ('log_audit', now(), ''), ('log_sign_in', now(), '');

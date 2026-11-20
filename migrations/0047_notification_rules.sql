create table notification_rules (
  id text primary key,
  name text not null,
  destination text not null check (destination in ('webhook', 'slack', 'discord')),
  host text not null,
  url_sealed bytea not null,
  secret_sealed bytea not null,
  events text[] not null check (cardinality(events) > 0),
  min_risk text not null default '' check (min_risk in ('', 'low', 'medium', 'high')),
  role text not null default '',
  threshold integer not null default 1 check (threshold between 1 and 100),
  window_minutes integer not null default 10 check (window_minutes between 1 and 1440),
  quiet_minutes integer not null default 15 check (quiet_minutes between 1 and 1440),
  enabled boolean not null default true,
  hits timestamptz[] not null default '{}',
  last_sent_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table webhook_deliveries
  alter column endpoint_id drop not null,
  add column rule_id text references notification_rules (id) on delete cascade,
  add constraint webhook_deliveries_target check ((endpoint_id is null) <> (rule_id is null));

create unique index webhook_deliveries_rule_event on webhook_deliveries (rule_id, event_id);
create index webhook_deliveries_rule on webhook_deliveries (rule_id, created_at desc);

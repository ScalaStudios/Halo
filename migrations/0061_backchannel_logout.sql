alter table applications
  add column backchannel_logout_uri text not null default '',
  add column backchannel_logout_session_required boolean not null default true;

alter table webhook_deliveries
  add column app_id text references applications (id) on delete cascade,
  drop constraint webhook_deliveries_target,
  add constraint webhook_deliveries_target check (num_nonnulls(endpoint_id, rule_id, app_id) = 1);

create unique index webhook_deliveries_app_event on webhook_deliveries (app_id, event_id);
create index webhook_deliveries_app on webhook_deliveries (app_id, created_at desc);
create index oidc_tokens_session on oidc_tokens (session_id);

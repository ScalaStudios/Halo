alter table oidc_tokens add column rotated boolean not null default false;

create index oidc_tokens_revoked on oidc_tokens (revoked_at) where kind = 'access' and not rotated and revoked_at is not null;

create index oidc_tokens_rotated on oidc_tokens (user_id, client_id) where rotated;

alter table webhook_cursors drop constraint webhook_cursors_source_check, add constraint webhook_cursors_source_check check (source in ('audit', 'sign_in', 'log_audit', 'log_sign_in', 'token'));

insert into webhook_cursors (source, time, id) values ('token', now(), '');

alter table risk_events add column responded_at timestamptz;

update risk_events set responded_at = now();

create index risk_events_unanswered on risk_events (time) where responded_at is null;

alter table users add column passkey_required boolean not null default false;

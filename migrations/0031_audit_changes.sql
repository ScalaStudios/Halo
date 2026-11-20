alter table audit_events add column changes jsonb not null default '[]';

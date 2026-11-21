alter table lifecycle_rules drop constraint lifecycle_rules_trigger_check;
alter table lifecycle_rules add constraint lifecycle_rules_trigger_check check (trigger in ('joiner', 'mover', 'leaver', 'date', 'inactivity'));
alter table lifecycle_rules add column attribute text;
alter table lifecycle_rules add column days integer;

create table lifecycle_occurrences (
  rule_id text not null references lifecycle_rules (id) on delete cascade,
  user_id text not null references users (id) on delete cascade,
  occurrence text not null,
  primary key (rule_id, user_id, occurrence)
);

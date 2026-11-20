alter table sign_in_events drop constraint sign_in_events_result_check,
  add constraint sign_in_events_result_check check (result in ('success', 'failure', 'interrupted', 'ended'));

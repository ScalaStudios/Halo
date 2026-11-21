create index sign_in_events_success on sign_in_events (time) include (user_id, app_id, method) where result = 'success' and impersonator_id is null;

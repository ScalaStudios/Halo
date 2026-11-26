update applications
set scopes = scopes || array(select s from unnest(array['profile', 'email', 'offline_access']) s where not s = any(scopes))
where protocol in ('oidc', 'oauth') and app_type <> 'service' and client_id <> 'halo-cli';

alter table identity_providers add column route_by_domain boolean not null default false;

alter table oidc_auth_requests add column domain_hint text not null default '';

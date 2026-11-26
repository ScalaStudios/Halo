create table ssh_host_revoked_keys (
  public_key text primary key,
  host_id text not null,
  revoked_at timestamptz not null default now()
);

update remote_targets set host = lower(host);

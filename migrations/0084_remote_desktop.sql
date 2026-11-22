alter table remote_targets drop constraint remote_targets_protocol_check;
alter table remote_targets add constraint remote_targets_protocol_check check (protocol in ('ssh', 'rdp', 'vnc'));
alter table remote_targets add column rdp_security text not null default 'nla' check (rdp_security in ('nla', 'tls'));

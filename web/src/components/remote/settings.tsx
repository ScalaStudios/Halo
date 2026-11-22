"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { api } from "@/lib/api/client";
import { formatDuration } from "@/lib/format";
import type { Outpost, RDPSecurity, RemoteAuth, RemoteProtocol, RemoteTarget } from "@/lib/infra-types";
import { REMOTE_AUTH } from "@/lib/labels";

export type RemoteDraft = { protocol: RemoteProtocol; rdpSecurity: RDPSecurity; outpostId: string; host: string; port: string; username: string; auth: RemoteAuth; credential: string; hostKey: string; sessionLimit: number; record: boolean };
export type RemoteErrors = Partial<Record<"outpostId" | "host" | "port" | "username" | "credential" | "hostKey", string>>;

const DEFAULT_PORT: Record<RemoteProtocol, number> = { ssh: 22, rdp: 3389, vnc: 5900 };

const LIMITS = [900, 1800, 3600, 7200, 14400, 28800, 43200, 86400];

export function remoteDraft(outposts: Outpost[], target?: RemoteTarget): RemoteDraft {
  return {
    protocol: target?.protocol ?? "ssh",
    rdpSecurity: target?.rdpSecurity ?? "nla",
    outpostId: target?.outpostId ?? outposts[0]?.id ?? "",
    host: target?.host ?? "",
    port: String(target?.port ?? 22),
    username: target?.username ?? "",
    auth: target?.auth ?? "certificate",
    credential: "",
    hostKey: target?.hostKey ?? "",
    sessionLimit: target?.sessionLimit ?? 3600,
    record: target?.record ?? false,
  };
}

export function remoteErrors(draft: RemoteDraft, stored: boolean): RemoteErrors {
  const port = Number(draft.port);
  const keeps = stored && draft.auth !== "certificate";
  return {
    outpostId: draft.outpostId ? undefined : "Choose the outpost that reaches this host.",
    host: /^[A-Za-z0-9.:-]+$/.test(draft.host.trim()) ? undefined : "Enter the host name or IP address the outpost connects to, like db1.internal, without a port.",
    port: Number.isInteger(port) && port >= 1 && port <= 65535 ? undefined : "Enter a port between 1 and 65535.",
    username: draft.protocol !== "vnc" && draft.auth !== "certificate" && !draft.username.trim() ? "Enter the account Halo signs in as." : undefined,
    hostKey: draft.protocol === "rdp" && !/^[a-f0-9]{64}$/i.test(draft.hostKey.trim().replaceAll(":", "")) ? "Enter the SHA-256 certificate fingerprint of the RDP server." : undefined,
    credential:
      draft.auth === "certificate" || draft.credential.trim() || keeps
        ? undefined
        : draft.auth === "password"
          ? "Enter the password Halo signs in with."
          : "Paste the private key Halo signs in with.",
  };
}

export function remoteBody(draft: RemoteDraft) {
  return {
    protocol: draft.protocol,
    rdpSecurity: draft.rdpSecurity,
    outpostId: draft.outpostId,
    host: draft.host.trim(),
    port: Number(draft.port),
    username: draft.username.trim(),
    auth: draft.auth,
    credential: draft.auth !== "certificate" && draft.credential ? draft.credential : undefined,
    hostKey: draft.hostKey.trim(),
    sessionLimit: draft.sessionLimit,
    record: draft.record,
  };
}

export function RemoteFields({
  draft,
  onChange,
  errors,
  outposts,
  stored = false,
}: {
  draft: RemoteDraft;
  onChange: (draft: RemoteDraft) => void;
  errors: RemoteErrors;
  outposts: Outpost[];
  stored?: boolean;
}) {
  const set = (patch: Partial<RemoteDraft>) => onChange({ ...draft, ...patch });
  const limits = LIMITS.includes(draft.sessionLimit) ? LIMITS : [...LIMITS, draft.sessionLimit].sort((a, b) => a - b);

  return (
    <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
      <Field
        label="Outpost"
        error={errors.outpostId}
        hint={
          outposts.length ? (
            "The outpost connects to the host for Halo. Halo never connects to it directly."
          ) : (
            <>
              No outpost is registered yet.{" "}
              <Link href="/admin/remote-access" className="text-link underline underline-offset-3 hover:text-ember">
                Register one on the Remote access page
              </Link>{" "}
              first.
            </>
          )
        }
        className="md:col-span-2"
      >
        {(props) => (
          <Select {...props} value={draft.outpostId} disabled={!outposts.length} onChange={(event) => set({ outpostId: event.target.value })}>
            {outposts.length ? null : <option value="">No outposts</option>}
            {outposts.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name} · {o.connected ? "connected" : "not connected"}
              </option>
            ))}
          </Select>
        )}
      </Field>
      <Field label="Protocol">
        {(props) => (
          <Select {...props} value={draft.protocol} onChange={(event) => {
            const protocol = event.target.value as RemoteProtocol;
            set({ protocol, port: String(DEFAULT_PORT[protocol]), username: "", auth: protocol === "ssh" ? "certificate" : "password", credential: "", hostKey: "", record: false });
          }}>
            <option value="ssh">SSH</option>
            <option value="rdp">RDP</option>
            <option value="vnc">VNC</option>
          </Select>
        )}
      </Field>
      {draft.protocol === "rdp" ? (
        <Field label="RDP security">
          {(props) => (
            <Select {...props} value={draft.rdpSecurity} onChange={(event) => set({ rdpSecurity: event.target.value as RDPSecurity })}>
              <option value="nla">Network Level Authentication</option>
              <option value="tls">TLS</option>
            </Select>
          )}
        </Field>
      ) : null}
      <Field label="Host" error={errors.host} hint="The name or address as the outpost resolves it.">
        {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.host} onChange={(event) => set({ host: event.target.value })} placeholder="db1.internal" />}
      </Field>
      <Field label="Port" error={errors.port}>
        {(props) => <Input {...props} type="number" min={1} max={65535} inputMode="numeric" className="tnum" value={draft.port} onChange={(event) => set({ port: event.target.value })} />}
      </Field>
      {draft.protocol === "ssh" ? <Field
        label="Sign in with"
        hint={
          draft.auth === "certificate"
            ? "Halo signs a certificate for each session that lasts one minute and names the person's SSH principals, so nothing is stored. The host must trust Halo's SSH CA."
            : "Halo stores it encrypted, signs in for the person and never shows it to anyone, in the console or the browser."
        }
      >
        {(props) => (
          <Select {...props} value={draft.auth} onChange={(event) => set({ auth: event.target.value as RemoteAuth, credential: "" })}>
            {(Object.keys(REMOTE_AUTH) as RemoteAuth[]).map((auth) => (
              <option key={auth} value={auth}>
                {REMOTE_AUTH[auth]}
              </option>
            ))}
          </Select>
        )}
      </Field> : null}
      <Field
        label="Account on the host"
        error={errors.username}
        hint={draft.auth === "certificate" ? "Leave empty to sign each person in as their first SSH principal." : draft.protocol === "vnc" ? "Optional for password-only VNC servers." : draft.protocol === "rdp" ? "Use DOMAIN\\username for a domain account." : "Everyone who connects signs in as this account."}
      >
        {(props) => (
          <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.username} onChange={(event) => set({ username: event.target.value })} placeholder={draft.auth === "certificate" ? "Their principal" : "deploy"} />
        )}
      </Field>
      {draft.auth === "password" ? (
        <Field label="Password" error={errors.credential} hint={stored ? "Halo has one stored. Leave this empty to keep it." : "Halo stores it encrypted and never shows it again."} className="md:col-span-2">
          {(props) => <PasswordInput {...props} autoComplete="new-password" value={draft.credential} onChange={(event) => set({ credential: event.target.value })} />}
        </Field>
      ) : null}
      {draft.auth === "key" ? (
        <Field
          label="Private key"
          error={errors.credential}
          hint={stored ? "Halo has one stored. Leave this empty to keep it." : "An unencrypted OpenSSH or PEM key. Halo stores it encrypted and never shows it again."}
          className="md:col-span-2"
        >
          {(props) => (
            <Textarea
              {...props}
              mono
              rows={6}
              autoComplete="off"
              spellCheck={false}
              value={draft.credential}
              onChange={(event) => set({ credential: event.target.value })}
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            />
          )}
        </Field>
      ) : null}
      {draft.protocol === "rdp" ? (
        <Field label="RDP certificate fingerprint" error={errors.hostKey} hint="SHA-256 fingerprint of the certificate presented by the RDP server." className="md:col-span-2">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.hostKey} onChange={(event) => set({ hostKey: event.target.value })} />}
        </Field>
      ) : null}
      {draft.protocol === "ssh" ? <Field
        label="Host key"
        hint="Paste the output of ssh-keyscan -t ed25519 for the host. Leave it empty only for hosts with a Halo host certificate: Halo connects to nothing else."
        className="md:col-span-2"
      >
        {(props) => (
          <Textarea {...props} mono rows={3} autoComplete="off" spellCheck={false} value={draft.hostKey} onChange={(event) => set({ hostKey: event.target.value })} placeholder="db1.internal ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…" />
        )}
      </Field> : null}
      <Field label="Session time limit" hint="Halo ends every session after this long.">
        {(props) => (
          <Select {...props} value={draft.sessionLimit} onChange={(event) => set({ sessionLimit: Number(event.target.value) })}>
            {limits.map((seconds) => (
              <option key={seconds} value={seconds}>
                {formatDuration(seconds)}
              </option>
            ))}
          </Select>
        )}
      </Field>
      {draft.protocol === "ssh" ? <Field label="Recording" hint="Recordings keep the terminal output, not keystrokes, for as long as the sign-in log.">
        {(props) => (
          <Select {...props} value={draft.record ? "on" : "off"} onChange={(event) => set({ record: event.target.value === "on" })}>
            <option value="off">Don&apos;t record</option>
            <option value="on">Record terminal output</option>
          </Select>
        )}
      </Field> : null}
    </div>
  );
}

export function RemoteSettingsForm({ appId, appName, target, outposts }: { appId: string; appName: string; target: RemoteTarget; outposts: Outpost[] }) {
  const [draft, setDraft] = useState(() => remoteDraft(outposts, target));
  const [errors, setErrors] = useState<RemoteErrors>({});
  const { pending, run, errorFor, toast } = useMutation();
  const stored = target.hasCredential && draft.protocol === target.protocol && draft.auth === target.auth;

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = remoteErrors(draft, stored);
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    void run(
      "remote",
      () => api(`/applications/${appId}/remote`, { method: "PUT", body: remoteBody(draft) }),
      () => {
        setDraft((d) => d.credential === draft.credential ? { ...d, credential: "" } : d);
        toast({ title: "Connection saved", description: `Sessions to ${appName} that start from now on use it. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title="Connection" description={`Where ${appName} connects and how Halo signs in to the host.`} />
      <form onSubmit={submit} noValidate>
        <CardBody className="flex flex-col gap-6">
          <RemoteFields draft={draft} onChange={setDraft} errors={errors} outposts={outposts} stored={stored} />
          {draft.protocol === "ssh" && !draft.hostKey.trim() ? (
            <SecretWarning>Without a host key, Halo only connects when the host presents a Halo host certificate for {draft.host.trim() || "its name"}. Sessions to any other host stop before sign-in.</SecretWarning>
          ) : null}
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("remote")} />
          <Button type="submit" variant="primary" loading={pending === "remote"}>
            Save connection
          </Button>
        </div>
      </form>
    </Card>
  );
}

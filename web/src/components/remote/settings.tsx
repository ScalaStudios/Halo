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
import type { Outpost, RemoteAuth, RemoteTarget } from "@/lib/infra-types";
import { REMOTE_AUTH } from "@/lib/labels";

export type RemoteDraft = { outpostId: string; host: string; port: string; username: string; auth: RemoteAuth; credential: string; hostKey: string; sessionLimit: number; record: boolean };
export type RemoteErrors = Partial<Record<"outpostId" | "host" | "port" | "username" | "credential", string>>;

const LIMITS = [900, 1800, 3600, 7200, 14400, 28800, 43200, 86400];

export function remoteDraft(outposts: Outpost[], target?: RemoteTarget): RemoteDraft {
  return {
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
    port: Number.isInteger(port) && port >= 1 && port <= 65535 ? undefined : "Enter a port between 1 and 65535. SSH usually listens on 22.",
    username: draft.auth !== "certificate" && !draft.username.trim() ? "Enter the account Halo signs in as." : undefined,
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
  storedAuth,
}: {
  draft: RemoteDraft;
  onChange: (draft: RemoteDraft) => void;
  errors: RemoteErrors;
  outposts: Outpost[];
  stored?: boolean;
  storedAuth?: RemoteAuth;
}) {
  const set = (patch: Partial<RemoteDraft>) => onChange({ ...draft, ...patch });
  const keeps = stored && draft.auth === storedAuth;
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
      <Field label="Host" error={errors.host} hint="The name or address as the outpost resolves it.">
        {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.host} onChange={(event) => set({ host: event.target.value })} placeholder="db1.internal" />}
      </Field>
      <Field label="Port" error={errors.port}>
        {(props) => <Input {...props} type="number" min={1} max={65535} inputMode="numeric" className="tnum" value={draft.port} onChange={(event) => set({ port: event.target.value })} />}
      </Field>
      <Field
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
      </Field>
      <Field
        label="Account on the host"
        error={errors.username}
        hint={draft.auth === "certificate" ? "Leave empty to sign each person in as their first SSH principal." : "Everyone who connects signs in as this account."}
      >
        {(props) => (
          <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.username} onChange={(event) => set({ username: event.target.value })} placeholder={draft.auth === "certificate" ? "Their principal" : "deploy"} />
        )}
      </Field>
      {draft.auth === "password" ? (
        <Field label="Password" error={errors.credential} hint={keeps ? "Halo has one stored. Leave this empty to keep it." : "Halo stores it encrypted and never shows it again."} className="md:col-span-2">
          {(props) => <PasswordInput {...props} autoComplete="new-password" value={draft.credential} onChange={(event) => set({ credential: event.target.value })} />}
        </Field>
      ) : null}
      {draft.auth === "key" ? (
        <Field
          label="Private key"
          error={errors.credential}
          hint={keeps ? "Halo has one stored. Leave this empty to keep it." : "An unencrypted OpenSSH or PEM key. Halo stores it encrypted and never shows it again."}
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
      <Field
        label="Host key"
        hint="Paste the output of ssh-keyscan -t ed25519 for the host. Leave it empty only for hosts with a Halo host certificate: Halo connects to nothing else."
        className="md:col-span-2"
      >
        {(props) => (
          <Textarea {...props} mono rows={3} autoComplete="off" spellCheck={false} value={draft.hostKey} onChange={(event) => set({ hostKey: event.target.value })} placeholder="db1.internal ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…" />
        )}
      </Field>
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
      <Field label="Recording" hint="Recordings keep the terminal output, not keystrokes, for as long as the sign-in log.">
        {(props) => (
          <Select {...props} value={draft.record ? "on" : "off"} onChange={(event) => set({ record: event.target.value === "on" })}>
            <option value="off">Don&apos;t record</option>
            <option value="on">Record terminal output</option>
          </Select>
        )}
      </Field>
    </div>
  );
}

export function RemoteSettingsForm({ appId, appName, target, outposts }: { appId: string; appName: string; target: RemoteTarget; outposts: Outpost[] }) {
  const [draft, setDraft] = useState(() => remoteDraft(outposts, target));
  const [errors, setErrors] = useState<RemoteErrors>({});
  const { pending, run, errorFor, toast } = useMutation();
  const stored = target.hasCredential && draft.auth === target.auth;

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = remoteErrors(draft, stored);
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    void run(
      "remote",
      () => api(`/applications/${appId}/remote`, { method: "PUT", body: remoteBody(draft) }),
      () => {
        setDraft((d) => ({ ...d, credential: "" }));
        toast({ title: "Connection saved", description: `Sessions to ${appName} that start from now on use it. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title="Connection" description={`Where ${appName} connects and how Halo signs in to the host. People only ever see the terminal.`} />
      <form onSubmit={submit} noValidate>
        <CardBody className="flex flex-col gap-6">
          <RemoteFields draft={draft} onChange={setDraft} errors={errors} outposts={outposts} stored={target.hasCredential} storedAuth={target.auth} />
          {!draft.hostKey.trim() ? (
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

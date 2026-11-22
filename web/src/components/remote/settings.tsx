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
import { useLocale } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import type { Locale } from "@/lib/i18n/locale";
import type { Outpost, RDPSecurity, RemoteAuth, RemoteProtocol, RemoteTarget } from "@/lib/infra-types";

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

export function remoteErrors(draft: RemoteDraft, stored: boolean, locale: Locale): RemoteErrors {
  const t = remoteMessages[locale].settings;
  const port = Number(draft.port);
  const keeps = stored && draft.auth !== "certificate";
  return {
    outpostId: draft.outpostId ? undefined : t.outpostError,
    host: /^[A-Za-z0-9.:-]+$/.test(draft.host.trim()) ? undefined : t.hostError,
    port: Number.isInteger(port) && port >= 1 && port <= 65535 ? undefined : t.portError,
    username: draft.protocol !== "vnc" && draft.auth !== "certificate" && !draft.username.trim() ? t.usernameError : undefined,
    hostKey: draft.protocol === "rdp" && !/^[a-f0-9]{64}$/i.test(draft.hostKey.trim().replaceAll(":", "")) ? t.hostKeyError : undefined,
    credential:
      draft.auth === "certificate" || draft.credential.trim() || keeps
        ? undefined
        : draft.auth === "password"
          ? t.passwordError
          : t.keyError,
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
  const locale = useLocale();
  const t = remoteMessages[locale].settings;
  const limits = LIMITS.includes(draft.sessionLimit) ? LIMITS : [...LIMITS, draft.sessionLimit].sort((a, b) => a - b);

  return (
    <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
      <Field
        label={t.outpost}
        error={errors.outpostId}
        hint={
          outposts.length ? (
            t.outpostHint
          ) : (
            t.noOutpost((text) => (
              <Link key="link" href="/admin/remote-access" className="text-link underline underline-offset-3 hover:text-ember">
                {text}
              </Link>
            ))
          )
        }
        className="md:col-span-2"
      >
        {(props) => (
          <Select {...props} value={draft.outpostId} disabled={!outposts.length} onChange={(event) => set({ outpostId: event.target.value })}>
            {outposts.length ? null : <option value="">{t.noOutposts}</option>}
            {outposts.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name} · {o.connected ? t.connected : t.notConnected}
              </option>
            ))}
          </Select>
        )}
      </Field>
      <Field label={t.protocol}>
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
        <Field label={t.rdpSecurity}>
          {(props) => (
            <Select {...props} value={draft.rdpSecurity} onChange={(event) => set({ rdpSecurity: event.target.value as RDPSecurity })}>
              <option value="nla">{t.nla}</option>
              <option value="tls">TLS</option>
            </Select>
          )}
        </Field>
      ) : null}
      <Field label={t.host} error={errors.host} hint={t.hostHint}>
        {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.host} onChange={(event) => set({ host: event.target.value })} placeholder="db1.internal" />}
      </Field>
      <Field label={t.port} error={errors.port}>
        {(props) => <Input {...props} type="number" min={1} max={65535} inputMode="numeric" className="tnum" value={draft.port} onChange={(event) => set({ port: event.target.value })} />}
      </Field>
      {draft.protocol === "ssh" ? <Field
        label={t.signInWith}
        hint={draft.auth === "certificate" ? t.certificateHint : t.storedHint}
      >
        {(props) => (
          <Select {...props} value={draft.auth} onChange={(event) => set({ auth: event.target.value as RemoteAuth, credential: "" })}>
            {(Object.keys(t.auth) as RemoteAuth[]).map((auth) => (
              <option key={auth} value={auth}>
                {t.auth[auth]}
              </option>
            ))}
          </Select>
        )}
      </Field> : null}
      <Field
        label={t.account}
        error={errors.username}
        hint={draft.auth === "certificate" ? t.accountCertificateHint : draft.protocol === "vnc" ? t.accountVncHint : draft.protocol === "rdp" ? t.accountRdpHint : t.accountHint}
      >
        {(props) => (
          <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.username} onChange={(event) => set({ username: event.target.value })} placeholder={draft.auth === "certificate" ? t.principalPlaceholder : "deploy"} />
        )}
      </Field>
      {draft.auth === "password" ? (
        <Field label={t.password} error={errors.credential} hint={stored ? t.keepStored : t.passwordHint} className="md:col-span-2">
          {(props) => <PasswordInput {...props} autoComplete="new-password" value={draft.credential} onChange={(event) => set({ credential: event.target.value })} />}
        </Field>
      ) : null}
      {draft.auth === "key" ? (
        <Field
          label={t.privateKey}
          error={errors.credential}
          hint={stored ? t.keepStored : t.privateKeyHint}
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
        <Field label={t.fingerprint} error={errors.hostKey} hint={t.fingerprintHint} className="md:col-span-2">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.hostKey} onChange={(event) => set({ hostKey: event.target.value })} />}
        </Field>
      ) : null}
      {draft.protocol === "ssh" ? <Field
        label={t.hostKey}
        hint={t.hostKeyHint}
        className="md:col-span-2"
      >
        {(props) => (
          <Textarea {...props} mono rows={3} autoComplete="off" spellCheck={false} value={draft.hostKey} onChange={(event) => set({ hostKey: event.target.value })} placeholder="db1.internal ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…" />
        )}
      </Field> : null}
      <Field label={t.limit} hint={t.limitHint}>
        {(props) => (
          <Select {...props} value={draft.sessionLimit} onChange={(event) => set({ sessionLimit: Number(event.target.value) })}>
            {limits.map((seconds) => (
              <option key={seconds} value={seconds}>
                {infrastructureMessages[locale].duration(seconds)}
              </option>
            ))}
          </Select>
        )}
      </Field>
      {draft.protocol === "ssh" ? <Field label={t.recording} hint={t.recordingHint}>
        {(props) => (
          <Select {...props} value={draft.record ? "on" : "off"} onChange={(event) => set({ record: event.target.value === "on" })}>
            <option value="off">{t.dontRecord}</option>
            <option value="on">{t.record}</option>
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
  const locale = useLocale();
  const t = remoteMessages[locale].settings;
  const stored = target.hasCredential && draft.protocol === target.protocol && draft.auth === target.auth;

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = remoteErrors(draft, stored, locale);
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    void run(
      "remote",
      () => api(`/applications/${appId}/remote`, { method: "PUT", body: remoteBody(draft) }),
      () => {
        setDraft((d) => d.credential === draft.credential ? { ...d, credential: "" } : d);
        toast({ title: t.saved, description: t.savedHint(appName) });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title={t.connection} description={t.connectionHint(appName)} />
      <form onSubmit={submit} noValidate>
        <CardBody className="flex flex-col gap-6">
          <RemoteFields draft={draft} onChange={setDraft} errors={errors} outposts={outposts} stored={stored} />
          {draft.protocol === "ssh" && !draft.hostKey.trim() ? (
            <SecretWarning>{t.noHostKey(draft.host.trim() || null)}</SecretWarning>
          ) : null}
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("remote")} />
          <Button type="submit" variant="primary" loading={pending === "remote"}>
            {t.save}
          </Button>
        </div>
      </form>
    </Card>
  );
}

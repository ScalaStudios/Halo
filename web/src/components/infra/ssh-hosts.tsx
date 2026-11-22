"use client";

import { Ban, FilePen, KeyRound, Plus, Server } from "lucide-react";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime, now } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import type { HostGroup, SshHost } from "@/lib/infra-types";

type Enrollment = { host: SshHost; token: string };
type Issued = { certificate: string; serial: number; validBefore: string };

function hostLabel(host: SshHost, fallback: string) {
  return host.hostnames[0] ?? fallback;
}

export function HostsSection({ hosts, hostGroups, issuer }: { hosts: SshHost[]; hostGroups: HostGroup[]; issuer: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [enrolling, setEnrolling] = useState<SshHost | "new" | null>(null);
  const [signing, setSigning] = useState<SshHost | null>(null);
  const [revoking, setRevoking] = useState<SshHost | null>(null);
  const current = now();
  const locale = useLocale();
  const t = infrastructureMessages[locale];
  const label = (host: SshHost) => hostLabel(host, t.hosts.thisHost);

  function move(host: SshHost, hostGroupId: string) {
    const group = hostGroups.find((g) => g.id === hostGroupId);
    void run(`move-${host.id}`, () => api(`/ssh/hosts/${host.id}/host-group`, { method: "PUT", body: { hostGroupId: hostGroupId || null } }), () =>
      toast({
        title: group ? t.hosts.moved(label(host), group.name) : t.hosts.ungrouped(label(host)),
        description: group ? t.hosts.movedHint : t.hosts.ungroupedHint,
      }),
    );
  }

  function revoke() {
    const host = revoking;
    if (!host) return;
    void run("revoke", () => api(`/ssh/hosts/${host.id}/revoke`, { method: "POST" }), () => {
      setRevoking(null);
      toast({ title: t.hosts.revokedToast(label(host)), description: t.hosts.revokedHint });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.hosts.title}
        description={t.hosts.hint}
        action={
          <Button size="sm" variant="primary" onClick={() => setEnrolling("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.hosts.register}
          </Button>
        }
      />
      {hosts.length ? (
        <SimpleTable
          caption={t.hosts.title}
          head={t.hosts.head}
          rows={hosts.map((host) => [
            <span key="hostnames" className="flex flex-wrap gap-1">
              {host.hostnames.map((name) => (
                <Tag key={name}>{name}</Tag>
              ))}
            </span>,
            host.publicKey ? (
              <span key="key" className="flex flex-col">
                <span className="font-mono text-code-sm break-all text-fg-2">{host.fingerprint}</span>
                <span className="text-caption text-fg-3">{host.keyType}</span>
              </span>
            ) : (
              <span key="key" className="flex flex-col whitespace-nowrap">
                <span className="text-fg-2">{t.hosts.notEnrolled}</span>
                {host.enrollmentExpiresAt && new Date(host.enrollmentExpiresAt) > current ? (
                  <span className="text-caption text-fg-3">{t.enrollment.tokenUntil(formatDateTime(host.enrollmentExpiresAt, locale))}</span>
                ) : null}
              </span>
            ),
            <span key="certificate" className="flex flex-col items-start gap-1 whitespace-nowrap">
              {host.revokedAt ? (
                <Badge tone="danger" dot>
                  {t.revoked}
                </Badge>
              ) : !host.validBefore ? (
                <Badge dot>{t.enrollment.waiting}</Badge>
              ) : new Date(host.validBefore) > current ? (
                <Badge tone="success" dot>
                  {t.valid}
                </Badge>
              ) : (
                <Badge tone="warning" dot>
                  {t.expired}
                </Badge>
              )}
              {host.revokedAt ? (
                <span className="text-caption text-fg-3">{formatDateTime(host.revokedAt, locale)}</span>
              ) : host.validBefore ? (
                <span className="text-caption text-fg-3">
                  {t.hosts.serialUntil(
                    <span key="serial" className="tnum">
                      {host.serial}
                    </span>,
                    formatDateTime(host.validBefore, locale),
                  )}
                </span>
              ) : null}
            </span>,
            host.revokedAt ? (
              <span key="group" className="text-fg-3">
                —
              </span>
            ) : (
              <span key="group" className="block w-48">
                <Select
                  size="sm"
                  aria-label={t.hosts.hostGroupOf(label(host))}
                  value={host.hostGroupId ?? ""}
                  disabled={pending === `move-${host.id}`}
                  onChange={(event) => move(host, event.target.value)}
                >
                  <option value="">{t.hosts.noHostGroup}</option>
                  {hostGroups.map((g) => (
                    <option key={g.id} value={g.id}>
                      {g.name}
                    </option>
                  ))}
                </Select>
              </span>
            ),
            host.revokedAt ? (
              <span key="actions" />
            ) : (
              <span key="actions" className="flex gap-1">
                <Button size="icon-sm" variant="quiet" aria-label={t.enrollment.newToken(label(host))} onClick={() => setEnrolling(host)}>
                  <KeyRound aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={t.hosts.signKey(label(host))} onClick={() => setSigning(host)}>
                  <FilePen aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={t.hosts.revoke(label(host))} onClick={() => setRevoking(host)}>
                  <Ban aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </span>
            ),
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Server}
            title={t.hosts.noHosts}
            description={t.hosts.noHostsHint}
          />
        </Card>
      )}
      {enrolling ? <EnrollmentDialog key={enrolling === "new" ? "new" : enrolling.id} host={enrolling === "new" ? null : enrolling} issuer={issuer} onClose={() => setEnrolling(null)} /> : null}
      {signing ? <SignDialog key={signing.id} host={signing} onClose={() => setSigning(null)} /> : null}
      <Dialog
        open={revoking !== null}
        onOpenChange={(open) => {
          if (open) return;
          setRevoking(null);
          clearError();
        }}
        error={errorFor("revoke")}
        title={t.hosts.revokeTitle(revoking ? label(revoking) : t.hosts.thisHost)}
        description={t.hosts.revokeHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRevoking(null)}>
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "revoke"} onClick={revoke}>
              {t.hosts.revokeConfirm}
            </Button>
          </>
        }
      />
    </div>
  );
}

function EnrollmentDialog({ host, issuer, onClose }: { host: SshHost | null; issuer: string; onClose: () => void }) {
  const { pending, run, errorFor } = useMutation();
  const [hostnames, setHostnames] = useState("");
  const [error, setError] = useState<string>();
  const [issued, setIssued] = useState<Enrollment | null>(null);
  const locale = useLocale();
  const t = infrastructureMessages[locale];

  function submit() {
    const list = hostnames
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean);
    if (!host && !list.length) {
      setError(t.hosts.hostnamesError);
      return;
    }
    setError(undefined);
    void run(
      "enroll",
      () => (host ? api<Enrollment>(`/ssh/hosts/${host.id}/enrollment-token`, { method: "POST" }) : api<Enrollment>("/ssh/hosts", { body: { hostnames: list } })),
      setIssued,
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("enroll")}
      title={issued ? t.enrollment.enroll(hostLabel(issued.host, t.hosts.thisHost)) : host ? t.enrollment.newToken(hostLabel(host, t.hosts.thisHost)) : t.hosts.register}
      description={issued ? t.hosts.enrollHint : host ? t.hosts.tokenHint : t.hosts.registerHint}
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "enroll"} onClick={submit}>
              {host ? t.enrollment.createToken : t.hosts.register}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.enrollment.command} value={`sudo halo ssh-host enroll --server ${issuer} --token ${issued.token}`} secret />
          <SecretWarning>{t.enrollment.onlyOnce(issued.host.enrollmentExpiresAt ? formatDateTime(issued.host.enrollmentExpiresAt, locale) : null)}</SecretWarning>
        </div>
      ) : host ? null : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.hosts.hostnames} error={error} hint={t.hosts.hostnamesHint}>
            {(props) => <Input {...props} autoFocus mono autoComplete="off" spellCheck={false} value={hostnames} onChange={(event) => setHostnames(event.target.value)} placeholder="web1.example.com" />}
          </Field>
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}

function SignDialog({ host, onClose }: { host: SshHost; onClose: () => void }) {
  const { pending, run, errorFor } = useMutation();
  const [publicKey, setPublicKey] = useState("");
  const [error, setError] = useState<string>();
  const [issued, setIssued] = useState<Issued | null>(null);
  const locale = useLocale();
  const t = infrastructureMessages[locale];
  const name = hostLabel(host, t.hosts.thisHost);

  function submit() {
    if (!publicKey.trim()) {
      setError(t.hosts.publicKeyError);
      return;
    }
    setError(undefined);
    void run("sign", () => api<Issued>(`/ssh/hosts/${host.id}/certificate`, { body: { publicKey: publicKey.trim() } }), setIssued, "inline");
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("sign")}
      title={issued ? t.hosts.certificateTitle(issued.serial, name) : t.hosts.signKey(name)}
      description={issued ? t.hosts.signedHint(formatDateTime(issued.validBefore, locale)) : t.hosts.signHint}
      width="wide"
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "sign"} onClick={submit}>
              {t.hosts.signConfirm}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="animate-enter">
          <CopyField label={t.hosts.hostCertificate} value={issued.certificate} />
        </div>
      ) : (
        <Field label={t.hosts.hostPublicKey} error={error}>
          {(props) => (
            <Textarea {...props} mono autoFocus rows={4} spellCheck={false} value={publicKey} onChange={(event) => setPublicKey(event.target.value)} placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…" />
          )}
        </Field>
      )}
    </Dialog>
  );
}

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
import type { HostGroup, SshHost } from "@/lib/infra-types";

type Enrollment = { host: SshHost; token: string };
type Issued = { certificate: string; serial: number; validBefore: string };

function hostLabel(host: SshHost) {
  return host.hostnames[0] ?? "this host";
}

export function HostsSection({ hosts, hostGroups, issuer }: { hosts: SshHost[]; hostGroups: HostGroup[]; issuer: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [enrolling, setEnrolling] = useState<SshHost | "new" | null>(null);
  const [signing, setSigning] = useState<SshHost | null>(null);
  const [revoking, setRevoking] = useState<SshHost | null>(null);
  const current = now();

  function move(host: SshHost, hostGroupId: string) {
    const group = hostGroups.find((g) => g.id === hostGroupId);
    void run(`move-${host.id}`, () => api(`/ssh/hosts/${host.id}/host-group`, { method: "PUT", body: { hostGroupId: hostGroupId || null } }), () =>
      toast({
        title: group ? `${hostLabel(host)} moved to ${group.name}` : `${hostLabel(host)} is in no host group`,
        description: group ? "halo-hostd applies its rules at the next refresh. Recorded in the audit log." : "halo-hostd lets nobody in from the next refresh. Recorded in the audit log.",
      }),
    );
  }

  function revoke() {
    const host = revoking;
    if (!host) return;
    void run("revoke", () => api(`/ssh/hosts/${host.id}/revoke`, { method: "POST" }), () => {
      setRevoking(null);
      toast({ title: `${hostLabel(host)} revoked`, description: "Its key is on the revocation list, and it can no longer renew. Recorded in the audit log." });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Hosts"
        description="Servers whose host keys Halo signs. A certificate names every hostname of its host, and ssh trusts it only for those names."
        action={
          <Button size="sm" variant="primary" onClick={() => setEnrolling("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Register host
          </Button>
        }
      />
      {hosts.length ? (
        <SimpleTable
          caption="Hosts"
          head={["Hostnames", "Host key", "Certificate", "Host group", "Actions"]}
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
                <span className="text-fg-2">Not enrolled</span>
                {host.enrollmentExpiresAt && new Date(host.enrollmentExpiresAt) > current ? (
                  <span className="text-caption text-fg-3">Token works until {formatDateTime(host.enrollmentExpiresAt)}</span>
                ) : null}
              </span>
            ),
            <span key="certificate" className="flex flex-col items-start gap-1 whitespace-nowrap">
              {host.revokedAt ? (
                <Badge tone="danger" dot>
                  Revoked
                </Badge>
              ) : !host.validBefore ? (
                <Badge dot>Waiting for enrollment</Badge>
              ) : new Date(host.validBefore) > current ? (
                <Badge tone="success" dot>
                  Valid
                </Badge>
              ) : (
                <Badge tone="warning" dot>
                  Expired
                </Badge>
              )}
              {host.revokedAt ? (
                <span className="text-caption text-fg-3">{formatDateTime(host.revokedAt)}</span>
              ) : host.validBefore ? (
                <span className="text-caption text-fg-3">
                  Serial <span className="tnum">{host.serial}</span>, until {formatDateTime(host.validBefore)}
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
                  aria-label={`Host group of ${hostLabel(host)}`}
                  value={host.hostGroupId ?? ""}
                  disabled={pending === `move-${host.id}`}
                  onChange={(event) => move(host, event.target.value)}
                >
                  <option value="">No host group</option>
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
                <Button size="icon-sm" variant="quiet" aria-label={`New enrollment token for ${hostLabel(host)}`} onClick={() => setEnrolling(host)}>
                  <KeyRound aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={`Sign a host key for ${hostLabel(host)}`} onClick={() => setSigning(host)}>
                  <FilePen aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={`Revoke ${hostLabel(host)}`} onClick={() => setRevoking(host)}>
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
            title="No hosts registered yet"
            description="Register a server to sign its host key, so people connect without the first-connection prompt."
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
        title={`Revoke ${revoking ? hostLabel(revoking) : "this host"}?`}
        description="Its key joins the revocation list and it can no longer renew. Clients without RevokedHostKeys keep trusting its current certificate until it expires. A revoked host can't be restored, and its key can't be enrolled again."
        footer={
          <>
            <Button variant="secondary" onClick={() => setRevoking(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "revoke"} onClick={revoke}>
              Revoke host
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

  function submit() {
    const list = hostnames
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean);
    if (!host && !list.length) {
      setError("Add at least one hostname, such as web1.example.com.");
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
      title={issued ? `Enroll ${hostLabel(issued.host)}` : host ? `New enrollment token for ${hostLabel(host)}` : "Register host"}
      description={
        issued
          ? "Run this command as root on the host. It sends the host's public key with the token and writes the certificate next to the key."
          : host
            ? "The new token replaces any unused one. Enrolling again with a new host key replaces the host's key."
            : "Hostnames are the names people type after ssh. They become the certificate's principals."
      }
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "enroll"} onClick={submit}>
              {host ? "Create token" : "Register host"}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Enrollment command" value={`sudo halo ssh-host enroll --server ${issuer} --token ${issued.token}`} secret />
          <SecretWarning>
            This is the only time Halo shows this token. It works once, until {issued.host.enrollmentExpiresAt ? formatDateTime(issued.host.enrollmentExpiresAt) : "it expires"}.
          </SecretWarning>
        </div>
      ) : host ? null : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Hostnames" error={error} hint="Separated by commas or spaces, such as web1.example.com, without wildcards or ports. ssh trusts the certificate without a prompt only for names under a verified domain.">
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

  function submit() {
    if (!publicKey.trim()) {
      setError("Paste the host's public key, such as the contents of /etc/ssh/ssh_host_ed25519_key.pub.");
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
      title={issued ? `Certificate ${issued.serial} for ${hostLabel(host)}` : `Sign a host key for ${hostLabel(host)}`}
      description={
        issued
          ? `Valid until ${formatDateTime(issued.validBefore)}. Save it on the host next to the key as ssh_host_ed25519_key-cert.pub and name it in HostCertificate in sshd_config.`
          : "For hosts that can't reach Halo to enroll. The key becomes the host's key, so the host can renew with halo ssh-host renew later."
      }
      width="wide"
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "sign"} onClick={submit}>
              Sign key
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="animate-enter">
          <CopyField label="Host certificate" value={issued.certificate} />
        </div>
      ) : (
        <Field label="Host public key" error={error}>
          {(props) => (
            <Textarea {...props} mono autoFocus rows={4} spellCheck={false} value={publicKey} onChange={(event) => setPublicKey(event.target.value)} placeholder="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…" />
          )}
        </Field>
      )}
    </Dialog>
  );
}

"use client";

import { KeyRound, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { RelativeTime } from "@/components/ui/relative-time";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDate, formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Credential } from "@/lib/types";
import { ExpiryText, SecretWarning } from "./shared";

export function RotateSecretDialog({
  open,
  onOpenChange,
  appId,
  appName,
  secret = "client",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  appId: string;
  appName: string;
  secret?: "client" | "shared" | "bind";
}) {
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const c = t.credentials;
  const label = c.kinds[secret].label;
  const bind = secret === "bind";
  const { pending, run, errorFor, clearError } = useMutation();
  const [rotated, setRotated] = useState<{ secret: string; graceEnds: string } | null>(null);

  function close() {
    onOpenChange(false);
    setRotated(null);
    clearError();
  }

  function rotate() {
    void run(
      "rotate",
      () => api<{ credential: Credential; clientSecret: string }>(`/applications/${appId}/secrets`, { body: {} }),
      ({ clientSecret }) => setRotated({ secret: clientSecret, graceEnds: formatDateTime(new Date(Date.now() + 86_400_000).toISOString(), locale) }),
      "inline",
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      error={errorFor("rotate")}
      title={rotated ? c.kinds[secret].fresh : c.kinds[secret].rotate}
      description={
        bind
          ? rotated
            ? c.bindRotated(appName)
            : c.bindConfirm(appName)
          : rotated
            ? c.rotated(appName, rotated.graceEnds)
            : c.confirm(label.toLowerCase(), appName)
      }
      footer={
        rotated ? (
          <Button variant="primary" onClick={close}>
            {t.common.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {t.common.cancel}
            </Button>
            <Button variant="primary" loading={pending === "rotate"} onClick={rotate}>
              {bind ? c.rotatePassword : c.rotateSecret}
            </Button>
          </>
        )
      }
    >
      {rotated ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={label} value={rotated.secret} secret />
          <SecretWarning>{c.onlyOnce}</SecretWarning>
        </div>
      ) : null}
    </Dialog>
  );
}

export function CredentialList({ appId, appName, credentials, empty, secret = "client" }: { appId: string; appName: string; credentials: Credential[]; empty: string; secret?: "client" | "shared" }) {
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const c = t.credentials;
  const [rotating, setRotating] = useState(false);
  const { run, toast } = useMutation();

  if (credentials.length === 0) return <EmptyState icon={KeyRound} title={c.empty} description={empty} />;

  function revoke(credential: Credential) {
    return run(
      `revoke-${credential.id}`,
      () => api(`/applications/${appId}/secrets/${credential.id}`, { method: "DELETE" }),
      () => toast({ title: c.revoked(credential.label), description: c.revokedHint(appName) }),
    );
  }

  return (
    <>
      <SimpleTable
        caption={c.caption}
        head={[c.credential, t.common.created, c.expires, c.lastUsed, t.common.actions]}
        rows={credentials.map((credential) => [
          <span key="n" className="flex flex-col">
            <span className="font-medium text-fg">{credential.label}</span>
            <span className="text-caption text-fg-3">
              {credential.kind === "secret" ? c.kinds[secret].label : c.certificate} · <span className="font-mono text-code-sm">{credential.hint}</span>
            </span>
          </span>,
          <span key="c" className="tnum whitespace-nowrap">
            {formatDate(credential.createdAt, locale)}
          </span>,
          <ExpiryText key="e" iso={credential.expiresAt} />,
          <span key="u" className="whitespace-nowrap">
            {credential.lastUsedAt ? <RelativeTime iso={credential.lastUsedAt} /> : t.common.never}
          </span>,
          <span key="a" className="flex justify-end gap-2">
            {credential.kind === "secret" ? (
              <Button size="sm" variant="secondary" onClick={() => setRotating(true)}>
                <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                {c.rotateSecret}
              </Button>
            ) : null}
            <HoldToConfirm size="sm" doneLabel={c.revokedDone} onConfirm={() => revoke(credential)}>
              {c.holdToRevoke}
            </HoldToConfirm>
          </span>,
        ])}
      />
      <RotateSecretDialog open={rotating} onOpenChange={setRotating} appId={appId} appName={appName} secret={secret} />
    </>
  );
}

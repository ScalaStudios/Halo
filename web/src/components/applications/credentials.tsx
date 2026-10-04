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
import type { Credential } from "@/lib/types";
import { ExpiryText, SecretWarning } from "./shared";

export function RotateSecretDialog({
  open,
  onOpenChange,
  appId,
  appName,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  appId: string;
  appName: string;
}) {
  const { pending, run } = useMutation();
  const [rotated, setRotated] = useState<{ secret: string; graceEnds: string } | null>(null);

  function close() {
    onOpenChange(false);
    setRotated(null);
  }

  function rotate() {
    void run(
      "rotate",
      () => api<{ credential: Credential; clientSecret: string }>(`/applications/${appId}/secrets`, { body: {} }),
      ({ clientSecret }) => setRotated({ secret: clientSecret, graceEnds: formatDateTime(new Date(Date.now() + 86_400_000).toISOString()) }),
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title={rotated ? "New client secret" : "Rotate client secret?"}
      description={
        rotated
          ? `Update ${appName} with this secret. The previous secrets keep working for 24 hours, until ${rotated.graceEnds}.`
          : `Halo creates a new client secret for ${appName}. The current secrets keep working for 24 hours, so you can deploy the new one without downtime.`
      }
      footer={
        rotated ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "rotate"} onClick={rotate}>
              Rotate secret
            </Button>
          </>
        )
      }
    >
      {rotated ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Client secret" value={rotated.secret} secret />
          <SecretWarning>This is the only time Halo shows this secret. Store it in your secret manager before closing this dialog.</SecretWarning>
        </div>
      ) : null}
    </Dialog>
  );
}

export function CredentialList({ appId, appName, credentials, empty }: { appId: string; appName: string; credentials: Credential[]; empty: string }) {
  const [rotating, setRotating] = useState(false);
  const { run, toast } = useMutation();

  if (credentials.length === 0) return <EmptyState icon={KeyRound} title="No credentials" description={empty} />;

  function revoke(credential: Credential) {
    void run(
      `revoke-${credential.id}`,
      () => api(`/applications/${appId}/secrets/${credential.id}`, { method: "DELETE" }),
      () => toast({ title: `“${credential.label}” revoked`, description: `${appName} can no longer use it. Recorded in the audit log.` }),
    );
  }

  return (
    <>
      <SimpleTable
        caption="Credentials"
        head={["Credential", "Created", "Expires", "Last used", "Actions"]}
        rows={credentials.map((credential) => [
          <span key="n" className="flex flex-col">
            <span className="font-medium text-fg">{credential.label}</span>
            <span className="text-caption text-fg-3">
              {credential.kind === "secret" ? "Client secret" : "Signing certificate"} · <span className="font-mono text-code-sm">{credential.hint}</span>
            </span>
          </span>,
          <span key="c" className="tnum whitespace-nowrap">
            {formatDate(credential.createdAt)}
          </span>,
          <ExpiryText key="e" iso={credential.expiresAt} />,
          <span key="u" className="whitespace-nowrap">
            {credential.lastUsedAt ? <RelativeTime iso={credential.lastUsedAt} /> : "Never"}
          </span>,
          <span key="a" className="flex justify-end gap-2">
            {credential.kind === "secret" ? (
              <Button size="sm" variant="secondary" onClick={() => setRotating(true)}>
                <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                Rotate secret
              </Button>
            ) : null}
            <HoldToConfirm size="sm" doneLabel="Revoked" onConfirm={() => revoke(credential)}>
              Hold to revoke
            </HoldToConfirm>
          </span>,
        ])}
      />
      <RotateSecretDialog open={rotating} onOpenChange={setRotating} appId={appId} appName={appName} />
    </>
  );
}

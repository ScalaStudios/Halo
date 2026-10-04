"use client";

import { PlugZap, RefreshCw, Settings2 } from "lucide-react";
import { useState } from "react";
import { PROTOCOL_LABEL } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { AppProvisioning } from "@/lib/provisioning-types";
import type { Protocol } from "@/lib/types";
import { StatusLabel } from "./shared";

export type OutboundApp = { id: string; name: string; protocol: Protocol; config: AppProvisioning | null };

function status(config: AppProvisioning | null) {
  if (!config) return { label: "Not set up", tone: "neutral" as const };
  if (!config.enabled) return { label: "Paused", tone: "neutral" as const };
  if (config.lastError) return { label: "Failing", tone: "danger" as const };
  return { label: "On", tone: "success" as const };
}

export function OutboundTable({ apps }: { apps: OutboundApp[] }) {
  const { pending, run, toast } = useMutation();
  const [configuring, setConfiguring] = useState<OutboundApp | null>(null);

  if (apps.length === 0) {
    return <EmptyState icon={RefreshCw} title="No applications to provision" description="Register an OpenID Connect or SAML application, then set up provisioning here." />;
  }

  function test(app: OutboundApp) {
    void run(
      `test-${app.id}`,
      () => api(`/provisioning/applications/${app.id}/test`, { method: "POST" }),
      () => toast({ title: `${app.name} accepted the connection`, description: "Halo reached the SCIM endpoint with the saved bearer token." }),
    );
  }

  function sync(app: OutboundApp) {
    void run(
      `sync-${app.id}`,
      () => api<AppProvisioning>(`/provisioning/applications/${app.id}/sync`, { method: "POST" }),
      (result) =>
        toast(
          result.lastError
            ? { title: `Some changes to ${app.name} failed`, description: result.lastError, tone: "danger" }
            : { title: `${app.name} is up to date`, description: `${result.created} created, ${result.updated} updated, ${result.deactivated} deactivated.` },
        ),
    );
  }

  return (
    <>
      <SimpleTable
        caption="Outbound provisioning"
        head={["Application", "Status", "People", "Last sync", "Actions"]}
        rows={apps.map((app) => {
          const config = app.config;
          return [
            <span key="n" className="flex flex-col">
              <span className="font-medium text-fg">{app.name}</span>
              <span className="text-caption text-fg-3">{PROTOCOL_LABEL[app.protocol]}</span>
            </span>,
            <span key="s" className="flex max-w-80 flex-col gap-1">
              <StatusLabel {...status(config)} />
              {config?.lastError ? (
                <span className="line-clamp-2 text-caption text-danger" title={config.lastError}>
                  {config.lastError}
                </span>
              ) : null}
            </span>,
            <span key="p" className="tnum">
              {config ? config.provisioned : "—"}
            </span>,
            config?.lastSyncAt ? (
              <span key="l" className="flex flex-col whitespace-nowrap" title={formatDateTime(config.lastSyncAt)}>
                <span><RelativeTime iso={config.lastSyncAt} /></span>
                <span className="text-caption text-fg-3">
                  {config.created} created · {config.updated} updated · {config.deactivated} deactivated
                </span>
              </span>
            ) : (
              <span key="l" className="text-fg-3">
                Never
              </span>
            ),
            <span key="a" className="flex justify-end gap-2">
              {config ? (
                <>
                  <Button size="sm" variant="quiet" loading={pending === `test-${app.id}`} onClick={() => test(app)}>
                    <PlugZap aria-hidden="true" size={16} strokeWidth={1.75} />
                    Test
                  </Button>
                  <Button size="sm" variant="quiet" loading={pending === `sync-${app.id}`} onClick={() => sync(app)}>
                    <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                    Sync now
                  </Button>
                </>
              ) : null}
              <Button size="sm" variant="secondary" onClick={() => setConfiguring(app)}>
                <Settings2 aria-hidden="true" size={16} strokeWidth={1.75} />
                {config ? "Edit" : "Set up"}
              </Button>
            </span>,
          ];
        })}
      />
      <ConfigureDialog key={configuring?.id ?? "closed"} app={configuring} onClose={() => setConfiguring(null)} />
    </>
  );
}

function ConfigureDialog({ app, onClose }: { app: OutboundApp | null; onClose: () => void }) {
  const configured = Boolean(app?.config);
  const [baseUrl, setBaseUrl] = useState(app?.config?.baseUrl ?? "");
  const [token, setToken] = useState("");
  const [enabled, setEnabled] = useState(app?.config?.enabled ?? true);
  const [errors, setErrors] = useState<{ baseUrl?: string; token?: string }>({});
  const { pending, run, toast } = useMutation();

  function submit() {
    if (!app) return;
    const next = {
      baseUrl: /^https?:\/\/\S+$/.test(baseUrl.trim()) ? undefined : "Enter the full SCIM base URL, starting with https://.",
      token: configured || token.trim() ? undefined : `Enter the bearer token ${app.name} issued for provisioning.`,
    };
    setErrors(next);
    if (next.baseUrl || next.token) return;
    void run(
      "save",
      () => api<AppProvisioning>(`/provisioning/applications/${app.id}`, { method: "PUT", body: { baseUrl: baseUrl.trim(), token: token.trim(), enabled } }),
      () => {
        onClose();
        toast({
          title: `Provisioning saved for ${app.name}`,
          description: enabled ? "Halo pushes assigned people every 2 minutes. Use Test to check the connection." : "Provisioning is paused. Nothing is pushed until you turn it on.",
        });
      },
    );
  }

  return (
    <Dialog
      open={app !== null}
      onOpenChange={(next) => (next ? undefined : onClose())}
      title={configured ? `Edit provisioning for ${app?.name}` : `Set up provisioning for ${app?.name}`}
      description="Halo creates accounts for people assigned to the application, updates them when their profile changes and deactivates them when they lose access."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            Save provisioning
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <Field label="SCIM base URL" error={errors.baseUrl} hint="From the application's SCIM or user provisioning settings.">
          {(props) => (
            <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://app.example.com/scim/v2" />
          )}
        </Field>
        <Field
          label="Bearer token"
          error={errors.token}
          hint={configured ? "Stored encrypted. Leave empty to keep the saved token." : "Stored encrypted. Halo never shows it again."}
        >
          {(props) => <Input {...props} mono type="password" autoComplete="off" value={token} onChange={(event) => setToken(event.target.value)} />}
        </Field>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Push changes every 2 minutes</span>
            <span className="text-caption text-fg-3">Turn off to pause provisioning without losing the settings.</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

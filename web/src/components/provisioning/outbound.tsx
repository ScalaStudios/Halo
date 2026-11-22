"use client";

import { PlugZap, RefreshCw, Settings2 } from "lucide-react";
import { useState } from "react";
import { Checklist } from "@/components/applications/collections-table";
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
import { useLocale, useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { Locale } from "@/lib/i18n/locale";
import type { AppProvisioning } from "@/lib/provisioning-types";
import type { Group, Protocol } from "@/lib/types";
import { StatusLabel } from "./shared";

export type OutboundApp = { id: string; name: string; protocol: Protocol; config: AppProvisioning | null };

function status(config: AppProvisioning | null, t: (typeof provisioningMessages)[Locale]["outbound"]) {
  if (!config) return { label: t.notSetUp, tone: "neutral" as const };
  if (!config.enabled) return { label: t.paused, tone: "neutral" as const };
  if (config.lastError) return { label: t.failing, tone: "danger" as const };
  return { label: t.on, tone: "success" as const };
}

function pushesGroups(config: AppProvisioning) {
  return config.groupIds.length > 0 || config.groupsDeleted > 0;
}

export function OutboundTable({ apps, groups }: { apps: OutboundApp[]; groups: Group[] }) {
  const { pending, run, toast } = useMutation();
  const [configuring, setConfiguring] = useState<OutboundApp | null>(null);
  const locale = useLocale();
  const common = provisioningMessages[locale];
  const t = common.outbound;

  if (apps.length === 0) {
    return <EmptyState icon={RefreshCw} title={t.empty} description={t.emptyHint} />;
  }

  function test(app: OutboundApp) {
    void run(
      `test-${app.id}`,
      () => api(`/provisioning/applications/${app.id}/test`, { method: "POST" }),
      () => toast({ title: t.accepted(app.name), description: t.acceptedHint }),
    );
  }

  function sync(app: OutboundApp) {
    void run(
      `sync-${app.id}`,
      () => api<AppProvisioning>(`/provisioning/applications/${app.id}/sync`, { method: "POST" }),
      (result) =>
        toast(
          result.lastError
            ? { title: t.syncFailed(app.name), description: result.lastError, tone: "danger" }
            : {
                title: t.upToDate(app.name),
                description: `${t.syncCounts(result.created, result.updated, result.deactivated)}${pushesGroups(result) ? t.groupSyncCounts(result.groupsCreated, result.groupsUpdated, result.groupsDeleted) : ""}`,
              },
        ),
    );
  }

  return (
    <>
      <SimpleTable
        caption={t.caption}
        head={t.head}
        rows={apps.map((app) => {
          const config = app.config;
          return [
            <span key="n" className="flex flex-col">
              <span className="font-medium text-fg">{app.name}</span>
              <span className="text-caption text-fg-3">{PROTOCOL_LABEL[app.protocol]}</span>
            </span>,
            <span key="s" className="flex max-w-80 flex-col gap-1">
              <StatusLabel {...status(config, t)} />
              {config?.lastError ? (
                <span lang="en" className="line-clamp-2 text-caption text-danger" title={config.lastError}>
                  {config.lastError}
                </span>
              ) : null}
            </span>,
            <span key="p" className="flex flex-col">
              <span className="tnum">{config ? config.provisioned : "—"}</span>
              {config && config.groupIds.length > 0 ? <span className="whitespace-nowrap text-caption text-fg-3">{common.groups(config.groupIds.length)}</span> : null}
            </span>,
            config?.lastSyncAt ? (
              <span key="l" className="flex flex-col whitespace-nowrap" title={formatDateTime(config.lastSyncAt, locale)}>
                <span><RelativeTime iso={config.lastSyncAt} /></span>
                <span className="text-caption text-fg-3">
                  {t.counts(config.created, config.updated, config.deactivated)}
                </span>
                {pushesGroups(config) ? (
                  <span className="text-caption text-fg-3">
                    {t.groupCounts(config.groupsCreated, config.groupsUpdated, config.groupsDeleted)}
                  </span>
                ) : null}
              </span>
            ) : (
              <span key="l" className="text-fg-3">
                {common.never}
              </span>
            ),
            <span key="a" className="flex justify-end gap-2">
              {config ? (
                <>
                  <Button size="sm" variant="quiet" loading={pending === `test-${app.id}`} onClick={() => test(app)}>
                    <PlugZap aria-hidden="true" size={16} strokeWidth={1.75} />
                    {t.test}
                  </Button>
                  <Button size="sm" variant="quiet" loading={pending === `sync-${app.id}`} onClick={() => sync(app)}>
                    <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                    {t.syncNow}
                  </Button>
                </>
              ) : null}
              <Button size="sm" variant="secondary" onClick={() => setConfiguring(app)}>
                <Settings2 aria-hidden="true" size={16} strokeWidth={1.75} />
                {config ? t.edit : t.setUp}
              </Button>
            </span>,
          ];
        })}
      />
      <ConfigureDialog key={configuring?.id ?? "closed"} app={configuring} groups={groups} onClose={() => setConfiguring(null)} />
    </>
  );
}

function ConfigureDialog({ app, groups, onClose }: { app: OutboundApp | null; groups: Group[]; onClose: () => void }) {
  const configured = Boolean(app?.config);
  const [baseUrl, setBaseUrl] = useState(app?.config?.baseUrl ?? "");
  const [token, setToken] = useState("");
  const [enabled, setEnabled] = useState(app?.config?.enabled ?? true);
  const [groupIds, setGroupIds] = useState<string[]>(app?.config?.groupIds ?? []);
  const [errors, setErrors] = useState<{ baseUrl?: string; token?: string }>({});
  const { pending, run, errorFor, toast } = useMutation();
  const common = useMessages(provisioningMessages);
  const t = common.outbound;

  function submit() {
    if (!app) return;
    const next = {
      baseUrl: /^https?:\/\/\S+$/.test(baseUrl.trim()) ? undefined : t.baseUrlError,
      token: configured || token.trim() ? undefined : t.tokenError(app.name),
    };
    setErrors(next);
    if (next.baseUrl || next.token) return;
    void run(
      "save",
      () => api<AppProvisioning>(`/provisioning/applications/${app.id}`, { method: "PUT", body: { baseUrl: baseUrl.trim(), token: token.trim(), enabled, groupIds } }),
      () => {
        onClose();
        toast({
          title: t.saved(app.name),
          description: enabled ? t.savedHint(groupIds.length) : t.pausedHint,
        });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open={app !== null}
      onOpenChange={(next) => (next ? undefined : onClose())}
      error={errorFor("save")}
      title={configured ? t.editTitle(app?.name) : t.setUpTitle(app?.name)}
      description={t.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {t.save}
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
        <Field label={common.page.scimBaseUrl} error={errors.baseUrl} hint={t.baseUrlHint}>
          {(props) => (
            <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://app.example.com/scim/v2" />
          )}
        </Field>
        <Field
          label={t.token}
          error={errors.token}
          hint={configured ? t.keepToken : t.tokenHint}
        >
          {(props) => <Input {...props} mono type="password" autoComplete="off" value={token} onChange={(event) => setToken(event.target.value)} />}
        </Field>
        {groups.length > 0 ? (
          <Checklist
            legend={t.groupsToPush}
            hint={t.groupsToPushHint(app?.name)}
            items={groups.map((g) => ({ id: g.id, label: g.name, detail: t.groupKind[g.kind] }))}
            value={groupIds}
            onChange={setGroupIds}
          />
        ) : null}
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{t.push}</span>
            <span className="text-caption text-fg-3">{t.pushHint}</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

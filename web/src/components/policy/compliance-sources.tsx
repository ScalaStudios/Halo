"use client";

import { MonitorCheck, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { RelativeTime } from "@/components/ui/relative-time";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { ComplianceSource } from "@/lib/policy-types";
import type { PolicyCopy } from "./shared";

type Saved = { source: ComplianceSource; secret?: string };

function maxAge(minutes: number, t: PolicyCopy["compliance"]) {
  return t.ages[minutes] ?? t.minutes(minutes);
}

export function ComplianceSources({ sources, issuer }: { sources: ComplianceSource[]; issuer: string }) {
  const copy = useMessages(policyMessages);
  const t = copy.compliance;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<ComplianceSource | "new" | null>(null);
  const [deleting, setDeleting] = useState<ComplianceSource | null>(null);

  function sync(source: ComplianceSource) {
    void run(`sync:${source.id}`, () => api<Saved>(`/compliance-sources/${source.id}/sync`, { method: "POST" }), ({ source: synced }) =>
      toast(
        synced.syncError
          ? { title: t.syncFailed(synced.name), description: synced.syncError, tone: "danger" }
          : { title: t.synced(synced.name), description: t.syncedDescription(synced.records) },
      ),
    );
  }

  function remove() {
    const source = deleting;
    if (!source) return;
    void run("delete", () => api(`/compliance-sources/${source.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: copy.deleted(source.name), description: copy.recorded });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.title}
        description={t.description}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.add}
          </Button>
        }
      />
      {sources.length ? (
        <SimpleTable
          caption={t.title}
          head={[copy.name, t.devices, t.validFor, t.lastReport, copy.actions]}
          rows={sources.map((source) => [
            <span key="name" className="flex flex-col">
              <span className="font-medium whitespace-nowrap text-fg">{source.name}</span>
              <span className="text-caption whitespace-nowrap text-fg-3">{t.kind[source.kind].label}</span>
            </span>,
            <span key="devices" className="flex flex-col">
              <span className="tnum whitespace-nowrap">{t.reported(source.records)}</span>
              <span className="tnum text-caption whitespace-nowrap text-fg-3">{t.linked(source.linked)}</span>
            </span>,
            <span key="age" className="whitespace-nowrap">
              {maxAge(source.maxAgeMinutes, t)}
            </span>,
            <span key="last" className="flex max-w-80 flex-col">
              <span className="whitespace-nowrap text-fg-3">{source.reportedAt ? <RelativeTime iso={source.reportedAt} /> : t.never}</span>
              {source.syncError ? <span lang="en" className="text-caption text-danger">{source.syncError}</span> : null}
            </span>,
            <span key="actions" className="flex gap-1">
              {source.kind === "intune" ? (
                <Button size="icon-sm" variant="quiet" aria-label={t.sync(source.name)} loading={pending === `sync:${source.id}`} onClick={() => sync(source)}>
                  <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              ) : null}
              <Button size="icon-sm" variant="quiet" aria-label={copy.edit(source.name)} onClick={() => setEditing(source)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={copy.delete(source.name)} onClick={() => setDeleting(source)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={MonitorCheck}
            title={t.empty}
            description={t.emptyDescription}
          />
        </Card>
      )}
      {editing ? (
        <SourceDialog key={editing === "new" ? "new" : editing.id} source={editing === "new" ? null : editing} issuer={issuer} onClose={() => setEditing(null)} />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={copy.confirmDelete(deleting?.name ?? t.deleteFallback)}
        description={t.deleteDescription(deleting?.records ?? 0, deleting?.linked ?? 0)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.delete}
            </Button>
          </>
        }
      />
    </div>
  );
}

function SourceDialog({ source, issuer, onClose }: { source: ComplianceSource | null; issuer: string; onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.compliance;
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState(source?.name ?? "");
  const [kind, setKind] = useState<ComplianceSource["kind"]>(source?.kind ?? "intune");
  const [tenantId, setTenantId] = useState(source?.tenantId ?? "");
  const [clientId, setClientId] = useState(source?.clientId ?? "");
  const [clientSecret, setClientSecret] = useState("");
  const [maxAgeMinutes, setMaxAgeMinutes] = useState(source?.maxAgeMinutes ?? 1440);
  const [rotateSecret, setRotateSecret] = useState(false);
  const [errors, setErrors] = useState<{ name?: string; tenantId?: string; clientId?: string; clientSecret?: string }>({});
  const [secret, setSecret] = useState<Saved | null>(null);

  function save() {
    const intune = kind === "intune";
    const next = {
      name: name.trim() ? undefined : copy.enterName(t.nameExample),
      tenantId: !intune || tenantId.trim() ? undefined : t.tenantRequired,
      clientId: !intune || clientId.trim() ? undefined : t.clientRequired,
      clientSecret: !intune || source || clientSecret.trim() ? undefined : t.secretRequired,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = { name: name.trim(), kind, tenantId: tenantId.trim(), clientId: clientId.trim(), clientSecret: clientSecret.trim(), maxAgeMinutes, rotateSecret };
    void run(
      "save",
      () => (source ? api<Saved>(`/compliance-sources/${source.id}`, { method: "PUT", body }) : api<Saved>("/compliance-sources", { body })),
      (saved) => {
        if (saved.secret) {
          setSecret(saved);
          return;
        }
        const s = saved.source;
        toast(
          s.syncError
            ? { title: t.savedNoSync(s.name), description: s.syncError, tone: "danger" }
            : { title: source ? copy.saved(s.name) : copy.added(s.name), description: s.kind === "intune" ? t.reportedByIntune(s.records) : copy.recorded },
        );
        onClose();
      },
      "inline",
    );
  }

  if (secret) {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
        title={source ? t.secretReplaced : copy.added(secret.source.name)}
        description={t.secretDescription}
        footer={
          <Button variant="primary" onClick={onClose}>
            {t.done}
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.reportUrl} value={`${issuer}/api/v1/compliance-sources/${secret.source.id}/reports`} />
          <CopyField label={t.signingSecret} value={secret.secret ?? ""} secret />
          <SecretWarning>{t.secretWarning}</SecretWarning>
        </div>
      </Dialog>
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={source ? copy.edit(source.name) : t.addTitle}
      description={t.kind[kind].description}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {source ? t.save : t.add}
          </Button>
        </>
      }
    >
      <form
        noValidate
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <Field label={copy.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" maxLength={100} value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
        </Field>
        {source ? null : (
          <Field label={t.kindLabel}>
            {(props) => (
              <Select {...props} value={kind} onChange={(event) => setKind(event.target.value as ComplianceSource["kind"])}>
                <option value="intune">{t.kind.intune.label}</option>
                <option value="agent">{t.kind.agent.label}</option>
              </Select>
            )}
          </Field>
        )}
        {kind === "intune" ? (
          <>
            <Field label={t.tenant} error={errors.tenantId} hint={t.tenantHint}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={tenantId} onChange={(event) => setTenantId(event.target.value)} placeholder="contoso.onmicrosoft.com" />}
            </Field>
            <Field label={t.client} error={errors.clientId}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={clientId} onChange={(event) => setClientId(event.target.value)} placeholder="00000000-0000-0000-0000-000000000000" />}
            </Field>
            <Field
              label={t.secret}
              error={errors.clientSecret}
              hint={source ? t.secretKeep : t.secretHint}
            >
              {(props) => <PasswordInput {...props} autoComplete="off" value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />}
            </Field>
          </>
        ) : null}
        <Field label={t.validForLabel} hint={t.validForHint}>
          {(props) => (
            <Select {...props} value={maxAgeMinutes} onChange={(event) => setMaxAgeMinutes(Number(event.target.value))}>
              {Object.entries({ ...t.ages, [maxAgeMinutes]: maxAge(maxAgeMinutes, t) }).map(([minutes, label]) => (
                <option key={minutes} value={minutes}>
                  {label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {source?.kind === "agent" ? (
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={rotateSecret} onChange={(event) => setRotateSecret(event.target.checked)} />
            <span className="flex flex-col gap-1">
              <span className="text-body-sm text-fg">{t.rotate}</span>
              <span className="text-caption text-fg-3">{t.rotateHint}</span>
            </span>
          </label>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

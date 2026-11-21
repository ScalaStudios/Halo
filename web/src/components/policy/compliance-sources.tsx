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
import { pluralize } from "@/lib/format";
import type { ComplianceSource } from "@/lib/policy-types";

type Saved = { source: ComplianceSource; secret?: string };

const KIND: Record<ComplianceSource["kind"], { label: string; description: string }> = {
  intune: { label: "Microsoft Intune", description: "Halo reads each managed device's compliance state from Microsoft Graph every 30 minutes." },
  agent: { label: "Device agent", description: "An agent or your own integration sends signed reports with each device's state, OS version, disk encryption and patch level." },
};

const MAX_AGE: Record<number, string> = { 60: "1 hour", 240: "4 hours", 720: "12 hours", 1440: "24 hours", 4320: "3 days", 10080: "7 days", 43200: "30 days" };

function maxAge(minutes: number) {
  return MAX_AGE[minutes] ?? pluralize(minutes, "minute");
}

export function ComplianceSources({ sources, issuer }: { sources: ComplianceSource[]; issuer: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<ComplianceSource | "new" | null>(null);
  const [deleting, setDeleting] = useState<ComplianceSource | null>(null);

  function sync(source: ComplianceSource) {
    void run(`sync:${source.id}`, () => api<Saved>(`/compliance-sources/${source.id}/sync`, { method: "POST" }), ({ source: synced }) =>
      toast(
        synced.syncError
          ? { title: `${synced.name} didn't sync`, description: synced.syncError, tone: "danger" }
          : { title: `${synced.name} synced`, description: `${pluralize(synced.records, "device")} reported. Linked devices use the new states at their next sign-in.` },
      ),
    );
  }

  function remove() {
    const source = deleting;
    if (!source) return;
    void run("delete", () => api(`/compliance-sources/${source.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${source.name} deleted`, description: "Recorded in the audit log." });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Compliance sources"
        description="Device management that tells Halo whether a device is compliant. Link a device to its record on the Devices tab, then require compliant devices in a policy."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add source
          </Button>
        }
      />
      {sources.length ? (
        <SimpleTable
          caption="Compliance sources"
          head={["Name", "Devices", "Reports valid for", "Last report", "Actions"]}
          rows={sources.map((source) => [
            <span key="name" className="flex flex-col">
              <span className="font-medium whitespace-nowrap text-fg">{source.name}</span>
              <span className="text-caption whitespace-nowrap text-fg-3">{KIND[source.kind].label}</span>
            </span>,
            <span key="devices" className="flex flex-col">
              <span className="tnum whitespace-nowrap">{pluralize(source.records, "device")} reported</span>
              <span className="tnum text-caption whitespace-nowrap text-fg-3">{source.linked} linked</span>
            </span>,
            <span key="age" className="whitespace-nowrap">
              {maxAge(source.maxAgeMinutes)}
            </span>,
            <span key="last" className="flex max-w-80 flex-col">
              <span className="whitespace-nowrap text-fg-3">{source.reportedAt ? <RelativeTime iso={source.reportedAt} /> : "Never"}</span>
              {source.syncError ? <span className="text-caption text-danger">{source.syncError}</span> : null}
            </span>,
            <span key="actions" className="flex gap-1">
              {source.kind === "intune" ? (
                <Button size="icon-sm" variant="quiet" aria-label={`Sync ${source.name} now`} loading={pending === `sync:${source.id}`} onClick={() => sync(source)}>
                  <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              ) : null}
              <Button size="icon-sm" variant="quiet" aria-label={`Edit ${source.name}`} onClick={() => setEditing(source)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Delete ${source.name}`} onClick={() => setDeleting(source)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={MonitorCheck}
            title="No compliance sources yet"
            description="Connect Microsoft Intune, or let a device agent report, so policies can require devices that are managed, encrypted and patched."
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
        title={`Delete ${deleting?.name ?? "source"}?`}
        description={`Halo removes its ${pluralize(deleting?.records ?? 0, "device record")} and unlinks ${pluralize(deleting?.linked ?? 0, "device")}, whose compliance becomes unknown. Policies that need compliant devices then refuse them.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete source
            </Button>
          </>
        }
      />
    </div>
  );
}

function SourceDialog({ source, issuer, onClose }: { source: ComplianceSource | null; issuer: string; onClose: () => void }) {
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
      name: name.trim() ? undefined : "Enter a name, for example “Intune”.",
      tenantId: !intune || tenantId.trim() ? undefined : "Enter the directory (tenant) ID.",
      clientId: !intune || clientId.trim() ? undefined : "Enter the application (client) ID.",
      clientSecret: !intune || source || clientSecret.trim() ? undefined : "Enter the client secret.",
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
            ? { title: `${s.name} saved, but didn't sync`, description: s.syncError, tone: "danger" }
            : { title: `${s.name} ${source ? "saved" : "added"}`, description: s.kind === "intune" ? `${pluralize(s.records, "device")} reported by Intune.` : "Recorded in the audit log." },
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
        title={source ? "Signing secret replaced" : `${secret.source.name} added`}
        description="Agents POST their reports to this URL and sign each one with this secret in the Halo-Signature header, the same way Halo signs webhooks."
        footer={
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Report URL" value={`${issuer}/api/v1/compliance-sources/${secret.source.id}/reports`} />
          <CopyField label="Signing secret" value={secret.secret ?? ""} secret />
          <SecretWarning>This is the only time Halo shows this secret. Store it with your agent before closing this dialog.</SecretWarning>
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
      title={source ? `Edit ${source.name}` : "Add compliance source"}
      description={KIND[kind].description}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {source ? "Save source" : "Add source"}
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
        <Field label="Name" error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" maxLength={100} value={name} onChange={(event) => setName(event.target.value)} placeholder="Intune" />}
        </Field>
        {source ? null : (
          <Field label="Kind">
            {(props) => (
              <Select {...props} value={kind} onChange={(event) => setKind(event.target.value as ComplianceSource["kind"])}>
                <option value="intune">{KIND.intune.label}</option>
                <option value="agent">{KIND.agent.label}</option>
              </Select>
            )}
          </Field>
        )}
        {kind === "intune" ? (
          <>
            <Field label="Directory (tenant) ID" error={errors.tenantId} hint="From the app registration's overview in Microsoft Entra, or your primary domain.">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={tenantId} onChange={(event) => setTenantId(event.target.value)} placeholder="contoso.onmicrosoft.com" />}
            </Field>
            <Field label="Application (client) ID" error={errors.clientId}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={clientId} onChange={(event) => setClientId(event.target.value)} placeholder="00000000-0000-0000-0000-000000000000" />}
            </Field>
            <Field
              label="Client secret"
              error={errors.clientSecret}
              hint={
                source
                  ? "Leave empty to keep the current secret. Halo stores it sealed."
                  : "Give the app the DeviceManagementManagedDevices.Read.All application permission with admin consent. Halo stores the secret sealed."
              }
            >
              {(props) => <PasswordInput {...props} autoComplete="off" value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />}
            </Field>
          </>
        ) : null}
        <Field label="Reports stay valid for" hint="A device whose last report is older than this counts as unknown, so it no longer meets a compliant-device condition.">
          {(props) => (
            <Select {...props} value={maxAgeMinutes} onChange={(event) => setMaxAgeMinutes(Number(event.target.value))}>
              {Object.entries({ ...MAX_AGE, [maxAgeMinutes]: maxAge(maxAgeMinutes) }).map(([minutes, label]) => (
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
              <span className="text-body-sm text-fg">Replace the signing secret</span>
              <span className="text-caption text-fg-3">Halo shows the new secret once. Reports signed with the old one are refused from then on.</span>
            </span>
          </label>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

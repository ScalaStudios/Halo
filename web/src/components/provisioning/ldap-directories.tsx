"use client";

import { Eye, FolderTree, History, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { RelativeTime } from "@/components/ui/relative-time";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { LDAPCounts, LDAPDirectory, LDAPMapping, LDAPSecurity, LDAPSync, LDAPSyncReport } from "@/lib/provisioning-types";
import type { Attribute } from "@/lib/types";
import { StatusLabel } from "./shared";

const SECURITY: Record<LDAPSecurity, string> = {
  ldaps: "LDAPS (ldaps://)",
  starttls: "StartTLS (ldap://)",
  none: "None: sends the bind password unencrypted",
};

const INTERVAL: Record<number, string> = { 15: "15 minutes", 30: "30 minutes", 60: "hour", 240: "4 hours", 720: "12 hours", 1440: "day" };

const DEFAULT_MAPPING: LDAPMapping = { id: "objectGUID", email: "mail", name: "displayName", title: "title", department: "department", location: "l", manager: "manager", groupName: "cn", member: "member", attributes: {} };

const PERSON_FIELDS: { key: "id" | "email" | "name" | "title" | "department" | "location" | "manager"; label: string; hint?: string }[] = [
  { key: "id", label: "Entry ID", hint: "objectGUID in Active Directory, entryUUID in OpenLDAP." },
  { key: "email", label: "Email" },
  { key: "name", label: "Name" },
  { key: "title", label: "Title" },
  { key: "department", label: "Department" },
  { key: "location", label: "Location" },
  { key: "manager", label: "Manager DN" },
];

function interval(minutes: number) {
  return INTERVAL[minutes] ?? pluralize(minutes, "minute");
}

function status(d: LDAPDirectory) {
  if (d.syncError) return { label: "Failing", tone: "danger" as const };
  if (!d.enabled) return { label: "Manual", tone: "neutral" as const };
  return { label: `Every ${interval(d.intervalMinutes)}`, tone: "success" as const };
}

function summary(c: LDAPCounts) {
  const parts = [
    [c.created, "created"],
    [c.updated, "updated"],
    [c.suspended, "suspended"],
    [c.restored, "restored"],
    [c.groupsCreated, c.groupsCreated === 1 ? "group created" : "groups created"],
    [c.groupsRenamed, c.groupsRenamed === 1 ? "group renamed" : "groups renamed"],
    [c.membersAdded, c.membersAdded === 1 ? "member added" : "members added"],
    [c.membersRemoved, c.membersRemoved === 1 ? "member removed" : "members removed"],
  ].filter(([n]) => n);
  return parts.length ? parts.map(([n, label]) => `${n} ${label}`).join(" · ") : "No changes";
}

export function LDAPDirectories({ directories, attributes }: { directories: LDAPDirectory[]; attributes: Attribute[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<LDAPDirectory | "new" | null>(null);
  const [deleting, setDeleting] = useState<LDAPDirectory | null>(null);
  const [preview, setPreview] = useState<{ directory: LDAPDirectory; report: LDAPSyncReport } | null>(null);
  const [history, setHistory] = useState<{ directory: LDAPDirectory; syncs: LDAPSync[] } | null>(null);

  function showPreview(directory: LDAPDirectory) {
    void run(`preview:${directory.id}`, () => api<LDAPSyncReport>(`/ldap-directories/${directory.id}/sync?dryRun=true`, { method: "POST" }), (report) => setPreview({ directory, report }));
  }

  function showHistory(directory: LDAPDirectory) {
    void run(`history:${directory.id}`, () => api<LDAPSync[]>(`/ldap-directories/${directory.id}/syncs`), (syncs) => setHistory({ directory, syncs }));
  }

  function sync() {
    const directory = preview?.directory;
    if (!directory) return;
    void run(
      "sync",
      () => api<LDAPSyncReport>(`/ldap-directories/${directory.id}/sync`, { method: "POST" }),
      (report) => {
        setPreview(null);
        toast(
          report.result === "failed"
            ? { title: `${directory.name} didn't sync`, description: report.error, tone: "danger" }
            : { title: `${directory.name} synced`, description: `${summary(report.counts)}. Every change is in the audit log.` },
        );
      },
      "inline",
    );
  }

  function remove() {
    const directory = deleting;
    if (!directory) return;
    void run(
      "delete",
      () => api(`/ldap-directories/${directory.id}`, { method: "DELETE" }),
      () => {
        setDeleting(null);
        toast({ title: `${directory.name} deleted`, description: "The people and groups it synced stay in Halo and can be edited there." });
      },
      "inline",
    );
  }

  return (
    <section className="flex flex-col gap-6">
      <SectionTitle
        title="LDAP directories"
        description="Halo reads people and groups from Active Directory or OpenLDAP, keeps their profiles and group members in step, and suspends people who leave or are disabled. It never writes to the directory."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add directory
          </Button>
        }
      />
      {directories.length ? (
        <SimpleTable
          caption="LDAP directories"
          head={["Directory", "Schedule", "Synced", "Last sync", "Actions"]}
          rows={directories.map((d) => [
            <span key="name" className="flex flex-col">
              <span className="font-medium whitespace-nowrap text-fg">{d.name}</span>
              <span className="font-mono text-caption whitespace-nowrap text-fg-3">{d.url}</span>
            </span>,
            <StatusLabel key="status" {...status(d)} />,
            <span key="synced" className="flex flex-col whitespace-nowrap">
              <span className="tnum">{pluralize(d.people, "person", "people")}</span>
              <span className="tnum text-caption text-fg-3">{pluralize(d.groups, "group")}</span>
            </span>,
            <span key="last" className="flex max-w-80 flex-col">
              <span className="whitespace-nowrap text-fg-3">{d.syncedAt ? <RelativeTime iso={d.syncedAt} /> : "Never"}</span>
              {d.syncError ? (
                <span className="line-clamp-2 text-caption text-danger" title={d.syncError}>
                  {d.syncError}
                </span>
              ) : null}
            </span>,
            <span key="actions" className="flex justify-end gap-1">
              <Button size="sm" variant="quiet" loading={pending === `preview:${d.id}`} onClick={() => showPreview(d)}>
                <Eye aria-hidden="true" size={16} strokeWidth={1.75} />
                Preview sync
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Sync history of ${d.name}`} loading={pending === `history:${d.id}`} onClick={() => showHistory(d)}>
                <History aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Edit ${d.name}`} onClick={() => setEditing(d)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Delete ${d.name}`} onClick={() => setDeleting(d)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={FolderTree}
            title="No LDAP directories yet"
            description="Add Active Directory or OpenLDAP with a read-only bind account, preview the first sync, then turn on scheduled syncs."
          />
        </Card>
      )}
      {editing ? <DirectoryDialog key={editing === "new" ? "new" : editing.id} directory={editing === "new" ? null : editing} attributes={attributes} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={preview !== null}
        onOpenChange={(open) => {
          if (open) return;
          setPreview(null);
          clearError();
        }}
        width="wide"
        error={errorFor("sync")}
        title={`Preview sync of ${preview?.directory.name ?? "directory"}`}
        description="What a sync would change now. Nothing has changed yet."
        footer={
          <>
            <Button variant="secondary" onClick={() => setPreview(null)}>
              Cancel
            </Button>
            {preview?.report.result === "succeeded" ? (
              <Button variant="primary" loading={pending === "sync"} onClick={sync}>
                Sync now
              </Button>
            ) : null}
          </>
        }
      >
        {preview ? <SyncResult report={preview.report} /> : null}
      </Dialog>
      <Dialog
        open={history !== null}
        onOpenChange={(open) => (open ? undefined : setHistory(null))}
        width="wide"
        title={`Sync history of ${history?.directory.name ?? "directory"}`}
        description="The 50 most recent syncs from the last 90 days. Previews aren't recorded."
        footer={
          <Button variant="secondary" onClick={() => setHistory(null)}>
            Close
          </Button>
        }
      >
        {history?.syncs.length ? (
          <ul className="flex flex-col rounded-md border border-border">
            {history.syncs.map((s) => (
              <li key={s.id} className="flex flex-col gap-1 border-t border-border px-4 py-3 first:border-t-0">
                <span className="flex flex-wrap items-center justify-between gap-2">
                  <StatusLabel label={s.result === "failed" ? "Failed" : "Succeeded"} tone={s.result === "failed" ? "danger" : "success"} />
                  <span className="text-caption text-fg-3" title={formatDateTime(s.startedAt)}>
                    <RelativeTime iso={s.startedAt} />
                  </span>
                </span>
                <span className="text-body-sm text-fg-2">{s.error || summary(s.counts)}</span>
                {s.skipped.length ? <span className="text-caption text-fg-3">{pluralize(s.skipped.length, "entry", "entries")} left alone</span> : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-body-sm text-fg-3">No syncs yet. Preview a sync to run the first one.</p>
        )}
      </Dialog>
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={`Delete ${deleting?.name ?? "directory"}?`}
        description={`Halo stops syncing and deletes its sync history. The ${pluralize(deleting?.people ?? 0, "person", "people")} and ${pluralize(deleting?.groups ?? 0, "group")} it synced stay in Halo with their status and members, and can be edited in Halo again.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete directory
            </Button>
          </>
        }
      />
    </section>
  );
}

function SyncResult({ report }: { report: LDAPSyncReport }) {
  if (report.result === "failed") return <ActionError message={report.error} />;
  const c = report.counts;
  return (
    <div className="flex animate-enter flex-col gap-6">
      <DescriptionList
        columns={2}
        items={[
          { label: "People", value: `${c.created} created · ${c.updated} updated · ${c.suspended} suspended · ${c.restored} restored` },
          { label: "Groups", value: `${c.groupsCreated} created · ${c.groupsRenamed} renamed · ${c.membersAdded} members added · ${c.membersRemoved} removed` },
        ]}
      />
      {report.changes.length ? (
        <ul aria-label="Changes" className="flex max-h-72 flex-col overflow-y-auto rounded-md border border-border">
          {report.changes.map((change, i) => (
            <li key={i} className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-2 first:border-t-0">
              <Tag>{change.action}</Tag>
              <span className="font-medium text-fg">{change.target}</span>
              <span className="text-body-sm text-fg-3">{change.detail}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-body-sm text-fg-3">Halo is already in step with the directory.</p>
      )}
      {report.skipped.length ? (
        <div className="flex flex-col gap-2">
          <h3 className="text-label text-fg">Left alone</h3>
          <ul className="flex max-h-56 flex-col gap-2 overflow-y-auto rounded-md border border-border p-4 text-body-sm text-warning">
            {report.skipped.map((s, i) => (
              <li key={i}>{s}</li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

function DirectoryDialog({ directory, attributes, onClose }: { directory: LDAPDirectory | null; attributes: Attribute[]; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [form, setForm] = useState({
    name: directory?.name ?? "",
    url: directory?.url ?? "",
    security: directory?.security ?? ("ldaps" as LDAPSecurity),
    caCertificate: directory?.caCertificate ?? "",
    bindDn: directory?.bindDn ?? "",
    password: "",
    userBaseDn: directory?.userBaseDn ?? "",
    userFilter: directory?.userFilter ?? "(&(objectClass=person)(mail=*))",
    groupBaseDn: directory?.groupBaseDn ?? "",
    groupFilter: directory?.groupFilter ?? "",
    enabled: directory?.enabled ?? false,
    intervalMinutes: directory?.intervalMinutes ?? 60,
  });
  const [mapping, setMapping] = useState<LDAPMapping>(directory?.mapping ?? DEFAULT_MAPPING);
  const [errors, setErrors] = useState<Partial<Record<"name" | "url" | "bindDn" | "password" | "userBaseDn", string>>>({});
  const mappable = attributes.filter((a) => !a.selfService);
  const text = (key: "name" | "url" | "caCertificate" | "bindDn" | "password" | "userBaseDn" | "userFilter" | "groupBaseDn" | "groupFilter") => ({
    value: form[key],
    onChange: (event: { target: { value: string } }) => setForm({ ...form, [key]: event.target.value }),
  });

  function save() {
    const next = {
      name: form.name.trim() ? undefined : "Enter a name, for example “Active Directory”.",
      url: /^ldaps?:\/\/\S+$/.test(form.url.trim()) ? undefined : "Enter the server as ldaps://host or ldap://host.",
      bindDn: form.bindDn.trim() ? undefined : "Enter the DN of a read-only account.",
      password: directory || form.password ? undefined : "Enter the bind account's password.",
      userBaseDn: form.userBaseDn.trim() ? undefined : "Enter where to search for people.",
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = { ...form, mapping: { ...mapping, attributes: Object.fromEntries(Object.entries(mapping.attributes).filter(([name, source]) => source && mappable.some((a) => a.name === name))) } };
    void run(
      "save",
      () => (directory ? api<LDAPDirectory>(`/ldap-directories/${directory.id}`, { method: "PUT", body }) : api<LDAPDirectory>("/ldap-directories", { body })),
      (saved) => {
        toast({
          title: `${saved.name} ${directory ? "saved" : "added"}`,
          description: saved.enabled ? `Halo syncs it every ${interval(saved.intervalMinutes)}.` : "Preview a sync to check the mapping before the first one.",
        });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      width="wide"
      error={errorFor("save")}
      title={directory ? `Edit ${directory.name}` : "Add LDAP directory"}
      description="Halo binds with a read-only account and searches with paged results. People synced from the directory can only be changed there."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {directory ? "Save directory" : "Add directory"}
          </Button>
        </>
      }
    >
      <form
        noValidate
        className="flex flex-col gap-8"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <fieldset className="flex flex-col gap-6">
          <legend className="mb-4 text-label text-fg">Connection</legend>
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" maxLength={100} placeholder="Active Directory" {...text("name")} />}
          </Field>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label="Server" error={errors.url}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="ldaps://dc1.example.com" {...text("url")} />}
            </Field>
            <Field label="Connection security">
              {(props) => (
                <Select {...props} value={form.security} onChange={(event) => setForm({ ...form, security: event.target.value as LDAPSecurity })}>
                  {Object.entries(SECURITY).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Certificate authority" hint="Optional. PEM certificates to trust for this server instead of the system's.">
            {(props) => <Textarea {...props} mono rows={3} spellCheck={false} placeholder="-----BEGIN CERTIFICATE-----" {...text("caCertificate")} />}
          </Field>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label="Bind DN" error={errors.bindDn}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="CN=halo-sync,OU=Service Accounts,DC=example,DC=com" {...text("bindDn")} />}
            </Field>
            <Field label="Bind password" error={errors.password} hint={directory ? "Leave empty to keep the current password. Stored sealed." : "Stored sealed and never shown again."}>
              {(props) => <PasswordInput {...props} autoComplete="new-password" {...text("password")} />}
            </Field>
          </div>
        </fieldset>
        <fieldset className="flex flex-col gap-6">
          <legend className="mb-4 text-label text-fg">What to sync</legend>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label="People base DN" error={errors.userBaseDn}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="OU=Staff,DC=example,DC=com" {...text("userBaseDn")} />}
            </Field>
            <Field label="People filter" hint="People outside the filter are suspended.">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} {...text("userFilter")} />}
            </Field>
            <Field label="Group base DN" hint="Optional. Leave empty to sync people only.">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="OU=Groups,DC=example,DC=com" {...text("groupBaseDn")} />}
            </Field>
            <Field label="Group filter">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="(|(objectClass=group)(objectClass=groupOfNames))" {...text("groupFilter")} />}
            </Field>
          </div>
        </fieldset>
        <fieldset className="flex flex-col gap-6">
          <legend className="mb-4 text-label text-fg">Attribute mapping</legend>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            {PERSON_FIELDS.map((f) => (
              <Field key={f.key} label={f.label} hint={f.hint}>
                {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping[f.key]} onChange={(event) => setMapping({ ...mapping, [f.key]: event.target.value.trim() })} />}
              </Field>
            ))}
            <Field label="Group name">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping.groupName} onChange={(event) => setMapping({ ...mapping, groupName: event.target.value.trim() })} />}
            </Field>
            <Field label="Group members">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping.member} onChange={(event) => setMapping({ ...mapping, member: event.target.value.trim() })} />}
            </Field>
            {mappable.map((a) => (
              <Field key={a.id} label={a.label} hint="Optional. A custom user attribute.">
                {(props) => (
                  <Input
                    {...props}
                    mono
                    autoComplete="off"
                    spellCheck={false}
                    value={mapping.attributes[a.name] ?? ""}
                    onChange={(event) => setMapping({ ...mapping, attributes: { ...mapping.attributes, [a.name]: event.target.value.trim() } })}
                  />
                )}
              </Field>
            ))}
          </div>
        </fieldset>
        <div className="flex flex-col gap-4">
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Sync on a schedule</span>
              <span className="text-caption text-fg-3">Leave off until a preview shows the changes you expect.</span>
            </span>
          </label>
          {form.enabled ? (
            <Field label="Sync every">
              {(props) => (
                <Select {...props} value={form.intervalMinutes} onChange={(event) => setForm({ ...form, intervalMinutes: Number(event.target.value) })}>
                  {Object.entries({ ...INTERVAL, [form.intervalMinutes]: interval(form.intervalMinutes) }).map(([minutes, label]) => (
                    <option key={minutes} value={minutes}>
                      {label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          ) : null}
        </div>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

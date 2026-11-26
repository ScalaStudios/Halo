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
import { formatDateTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { Locale } from "@/lib/i18n/locale";
import type { LDAPCounts, LDAPDirectory, LDAPMapping, LDAPSecurity, LDAPSync, LDAPSyncReport } from "@/lib/provisioning-types";
import type { Attribute } from "@/lib/types";
import { StatusLabel } from "./shared";

type Messages = (typeof provisioningMessages)[Locale];

const DEFAULT_MAPPING: LDAPMapping = { id: "objectGUID", email: "mail", name: "displayName", title: "title", department: "department", location: "l", manager: "manager", groupName: "cn", member: "member", attributes: {} };

const PERSON_FIELDS = (t: Messages): { key: "id" | "email" | "name" | "title" | "department" | "location" | "manager"; label: string; hint?: string }[] => [
  { key: "id", label: t.ldap.entryId, hint: t.ldap.entryIdHint },
  { key: "email", label: t.ldap.email },
  { key: "name", label: t.name },
  { key: "title", label: t.ldap.title },
  { key: "department", label: t.ldap.department },
  { key: "location", label: t.ldap.location },
  { key: "manager", label: t.ldap.manager },
];

function interval(minutes: number, t: Messages) {
  return t.ldap.interval[minutes] ?? t.ldap.minutes(minutes);
}

function status(d: LDAPDirectory, t: Messages) {
  if (d.syncError) return { label: t.ldap.failing, tone: "danger" as const };
  if (!d.enabled) return { label: t.ldap.manual, tone: "neutral" as const };
  return { label: t.ldap.every(interval(d.intervalMinutes, t)), tone: "success" as const };
}

function summary(c: LDAPCounts, t: Messages) {
  const parts = (Object.keys(t.ldap.counts) as (keyof Messages["ldap"]["counts"])[]).filter((key) => c[key]).map((key) => t.ldap.counts[key](c[key]));
  return parts.length ? parts.join(" · ") : t.ldap.noChanges;
}

export function LDAPDirectories({ directories, attributes }: { directories: LDAPDirectory[]; attributes: Attribute[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<LDAPDirectory | "new" | null>(null);
  const [deleting, setDeleting] = useState<LDAPDirectory | null>(null);
  const [preview, setPreview] = useState<{ directory: LDAPDirectory; report: LDAPSyncReport } | null>(null);
  const [history, setHistory] = useState<{ directory: LDAPDirectory; syncs: LDAPSync[] } | null>(null);
  const locale = useLocale();
  const t = provisioningMessages[locale];

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
            ? { title: t.ldap.syncFailed(directory.name), description: report.error, tone: "danger" }
            : { title: t.ldap.synced(directory.name), description: t.ldap.syncedHint(summary(report.counts, t)) },
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
        toast({ title: t.deleted(directory.name), description: t.ldap.deletedHint });
      },
      "inline",
    );
  }

  return (
    <section className="flex flex-col gap-6">
      <SectionTitle
        title={t.ldap.heading}
        description={t.ldap.headingHint}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.ldap.add}
          </Button>
        }
      />
      {directories.length ? (
        <SimpleTable
          caption={t.ldap.heading}
          head={t.ldap.head}
          rows={directories.map((d) => [
            <span key="name" className="flex flex-col">
              <span className="font-medium whitespace-nowrap text-fg">{d.name}</span>
              <span className="font-mono text-caption whitespace-nowrap text-fg-3">{d.url}</span>
            </span>,
            <StatusLabel key="status" {...status(d, t)} />,
            <span key="synced" className="flex flex-col whitespace-nowrap">
              <span className="tnum">{t.people(d.people)}</span>
              <span className="tnum text-caption text-fg-3">{t.groups(d.groups)}</span>
            </span>,
            <span key="last" className="flex max-w-80 flex-col">
              <span className="whitespace-nowrap text-fg-3">{d.syncedAt ? <RelativeTime iso={d.syncedAt} /> : t.never}</span>
              {d.syncError ? (
                <span lang="en" className="line-clamp-2 text-caption text-danger" title={d.syncError}>
                  {d.syncError}
                </span>
              ) : null}
            </span>,
            <span key="actions" className="flex justify-end gap-1">
              <Button size="sm" variant="quiet" loading={pending === `preview:${d.id}`} onClick={() => showPreview(d)}>
                <Eye aria-hidden="true" size={16} strokeWidth={1.75} />
                {t.ldap.preview}
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={t.ldap.history(d.name)} loading={pending === `history:${d.id}`} onClick={() => showHistory(d)}>
                <History aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={t.edit(d.name)} onClick={() => setEditing(d)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={t.delete(d.name)} onClick={() => setDeleting(d)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={FolderTree}
            title={t.ldap.empty}
            description={t.ldap.emptyHint}
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
        title={t.ldap.previewTitle(preview?.directory.name ?? null)}
        description={t.ldap.previewHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPreview(null)}>
              {t.cancel}
            </Button>
            {preview?.report.result === "succeeded" ? (
              <Button variant="primary" loading={pending === "sync"} onClick={sync}>
                {t.ldap.syncNow}
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
        title={t.ldap.historyTitle(history?.directory.name ?? null)}
        description={t.ldap.historyHint}
        footer={
          <Button variant="secondary" onClick={() => setHistory(null)}>
            {t.ldap.close}
          </Button>
        }
      >
        {history?.syncs.length ? (
          <ul className="flex flex-col rounded-md border border-border">
            {history.syncs.map((s) => (
              <li key={s.id} className="flex flex-col gap-1 border-t border-border px-4 py-3 first:border-t-0">
                <span className="flex flex-wrap items-center justify-between gap-2">
                  <StatusLabel label={s.result === "failed" ? t.ldap.failed : t.ldap.succeeded} tone={s.result === "failed" ? "danger" : "success"} />
                  <span className="text-caption text-fg-3" title={formatDateTime(s.startedAt, locale)}>
                    <RelativeTime iso={s.startedAt} />
                  </span>
                </span>
                <span className="text-body-sm text-fg-2">{s.error ? <span lang="en">{s.error}</span> : summary(s.counts, t)}</span>
                {s.skipped.length ? <span className="text-caption text-fg-3">{t.ldap.leftAlone(s.skipped.length)}</span> : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-body-sm text-fg-3">{t.ldap.noSyncs}</p>
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
        title={t.ldap.deleteTitle(deleting?.name ?? null)}
        description={t.ldap.deleteHint(deleting?.people ?? 0, deleting?.groups ?? 0)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.ldap.deleteConfirm}
            </Button>
          </>
        }
      />
    </section>
  );
}

function SyncResult({ report }: { report: LDAPSyncReport }) {
  const t = useMessages(provisioningMessages);
  if (report.result === "failed") return <ActionError message={report.error} />;
  const c = report.counts;
  return (
    <div className="flex animate-enter flex-col gap-6">
      <DescriptionList
        columns={2}
        items={[
          { label: t.ldap.people, value: t.ldap.peopleCounts(c) },
          { label: t.ldap.groupsLabel, value: t.ldap.groupCounts(c) },
        ]}
      />
      {report.changes.length ? (
        <ul aria-label={t.ldap.changes} className="flex max-h-72 flex-col overflow-y-auto rounded-md border border-border">
          {report.changes.map((change, i) => (
            <li key={i} className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-2 first:border-t-0">
              <Tag>{change.action}</Tag>
              <span className="font-medium text-fg">{change.target}</span>
              <span lang="en" className="text-body-sm text-fg-3">{change.detail}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-body-sm text-fg-3">{t.ldap.inStep}</p>
      )}
      {report.skipped.length ? (
        <div className="flex flex-col gap-2">
          <h3 className="text-label text-fg">{t.ldap.leftAloneTitle}</h3>
          <ul lang="en" className="flex max-h-56 flex-col gap-2 overflow-y-auto rounded-md border border-border p-4 text-body-sm text-warning">
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
  const t = useMessages(provisioningMessages);
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

  const keepsPassword = directory && form.url.trim() === directory.url && form.security === directory.security && form.caCertificate.trim() === directory.caCertificate;

  function save() {
    const next = {
      name: form.name.trim() ? undefined : t.ldap.nameError,
      url: /^ldaps?:\/\/\S+$/.test(form.url.trim()) ? undefined : t.ldap.urlError,
      bindDn: form.bindDn.trim() ? undefined : t.ldap.bindDnError,
      password: keepsPassword || form.password ? undefined : t.ldap.passwordError,
      userBaseDn: form.userBaseDn.trim() ? undefined : t.ldap.userBaseDnError,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = { ...form, mapping: { ...mapping, attributes: Object.fromEntries(Object.entries(mapping.attributes).filter(([name, source]) => source && mappable.some((a) => a.name === name))) } };
    void run(
      "save",
      () => (directory ? api<LDAPDirectory>(`/ldap-directories/${directory.id}`, { method: "PUT", body }) : api<LDAPDirectory>("/ldap-directories", { body })),
      (saved) => {
        toast({
          title: directory ? t.ldap.saved(saved.name) : t.ldap.added(saved.name),
          description: saved.enabled ? t.ldap.syncsEvery(interval(saved.intervalMinutes, t)) : t.ldap.previewFirst,
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
      title={directory ? t.edit(directory.name) : t.ldap.addTitle}
      description={t.ldap.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {directory ? t.ldap.save : t.ldap.add}
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
          <legend className="mb-4 text-label text-fg">{t.ldap.connection}</legend>
          <Field label={t.name} error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" maxLength={100} placeholder="Active Directory" {...text("name")} />}
          </Field>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label={t.ldap.server} error={errors.url}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="ldaps://dc1.example.com" {...text("url")} />}
            </Field>
            <Field label={t.ldap.connectionSecurity}>
              {(props) => (
                <Select {...props} value={form.security} onChange={(event) => setForm({ ...form, security: event.target.value as LDAPSecurity })}>
                  {Object.entries(t.ldap.security).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label={t.ldap.certificateAuthority} hint={t.ldap.certificateAuthorityHint}>
            {(props) => <Textarea {...props} mono rows={3} spellCheck={false} placeholder="-----BEGIN CERTIFICATE-----" {...text("caCertificate")} />}
          </Field>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label={t.ldap.bindDn} error={errors.bindDn}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="CN=halo-sync,OU=Service Accounts,DC=example,DC=com" {...text("bindDn")} />}
            </Field>
            <Field label={t.ldap.bindPassword} error={errors.password} hint={keepsPassword ? t.ldap.keepPassword : t.ldap.passwordHint}>
              {(props) => <PasswordInput {...props} autoComplete="new-password" {...text("password")} />}
            </Field>
          </div>
        </fieldset>
        <fieldset className="flex flex-col gap-6">
          <legend className="mb-4 text-label text-fg">{t.ldap.whatToSync}</legend>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label={t.ldap.peopleBaseDn} error={errors.userBaseDn}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="OU=Staff,DC=example,DC=com" {...text("userBaseDn")} />}
            </Field>
            <Field label={t.ldap.peopleFilter} hint={t.ldap.peopleFilterHint}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} {...text("userFilter")} />}
            </Field>
            <Field label={t.ldap.groupBaseDn} hint={t.ldap.groupBaseDnHint}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="OU=Groups,DC=example,DC=com" {...text("groupBaseDn")} />}
            </Field>
            <Field label={t.ldap.groupFilter}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} placeholder="(|(objectClass=group)(objectClass=groupOfNames))" {...text("groupFilter")} />}
            </Field>
          </div>
        </fieldset>
        <fieldset className="flex flex-col gap-6">
          <legend className="mb-4 text-label text-fg">{t.ldap.mapping}</legend>
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            {PERSON_FIELDS(t).map((f) => (
              <Field key={f.key} label={f.label} hint={f.hint}>
                {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping[f.key]} onChange={(event) => setMapping({ ...mapping, [f.key]: event.target.value.trim() })} />}
              </Field>
            ))}
            <Field label={t.ldap.groupName}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping.groupName} onChange={(event) => setMapping({ ...mapping, groupName: event.target.value.trim() })} />}
            </Field>
            <Field label={t.ldap.groupMembers}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={mapping.member} onChange={(event) => setMapping({ ...mapping, member: event.target.value.trim() })} />}
            </Field>
            {mappable.map((a) => (
              <Field key={a.id} label={a.label} hint={t.ldap.customAttributeHint}>
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
              <span className="text-body-sm text-fg">{t.ldap.schedule}</span>
              <span className="text-caption text-fg-3">{t.ldap.scheduleHint}</span>
            </span>
          </label>
          {form.enabled ? (
            <Field label={t.ldap.syncEvery}>
              {(props) => (
                <Select {...props} value={form.intervalMinutes} onChange={(event) => setForm({ ...form, intervalMinutes: Number(event.target.value) })}>
                  {Object.entries({ ...t.ldap.interval, [form.intervalMinutes]: interval(form.intervalMinutes, t) }).map(([minutes, label]) => (
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

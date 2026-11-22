"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { BadgeCheck, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { joinNames, type Person } from "@/components/governance/shared";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
import { GROUP_KIND } from "@/lib/labels";
import type { AppRole, Group } from "@/lib/types";
import { Checklist } from "./collections-table";

type Row = AppRole & { holders: { name: string }[] };

const columns = (t: (typeof applicationsMessages)[Locale]): ColumnDef<Row, any>[] => [
  {
    id: "name",
    accessorFn: (r) => r.name,
    header: t.roles.role,
    meta: { label: t.roles.role, hideable: false },
    cell: ({ row }) => (
      <span className="flex flex-col py-2">
        <span className="font-medium whitespace-nowrap text-fg">{row.original.name}</span>
        <span className="font-mono text-code-sm text-fg-3">{row.original.key}</span>
      </span>
    ),
  },
  {
    id: "description",
    accessorFn: (r) => r.description,
    header: t.common.description,
    meta: { label: t.common.description },
    cell: ({ getValue }) => <span className="block max-w-96 min-w-48 py-2">{getValue<string>() || "—"}</span>,
  },
  {
    id: "holders",
    accessorFn: (r) => joinNames(r.holders),
    header: t.roles.heldBy,
    meta: { label: t.roles.heldBy },
    cell: ({ row }) =>
      row.original.holders.length ? (
        <span className="block max-w-96 min-w-48 py-2">{joinNames(row.original.holders)}</span>
      ) : (
        <span className="whitespace-nowrap text-fg-3">{t.common.nobody}</span>
      ),
  },
];

export function RolesTable({ appId, appName, roles, groups, people }: { appId: string; appName: string; roles: AppRole[]; groups: Group[]; people: Person[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const r = t.roles;
  const [editing, setEditing] = useState<AppRole | "new" | null>(null);
  const [deleting, setDeleting] = useState<AppRole | null>(null);
  const rows = roles.map((r) => ({
    ...r,
    holders: [...groups.filter((g) => r.groupIds.includes(g.id)), ...people.filter((p) => r.userIds.includes(p.id))],
  }));

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {r.add}
    </Button>
  );

  return (
    <>
      {rows.length === 0 ? (
        <Card>
          <EmptyState
            icon={BadgeCheck}
            title={r.empty}
            description={r.emptyHint(appName)}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns(t)}
          getRowId={(r) => r.id}
          label={r.label}
          noun={r.noun}
          storageKey="application-roles"
          searchText={(r) => `${r.name} ${r.key} ${joinNames(r.holders)}`}
          searchPlaceholder={r.search}
          onRowOpen={setEditing}
          rowActions={[
            { label: r.edit, icon: Pencil, onSelect: setEditing },
            { label: r.delete, icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
          ]}
          toolbarEnd={create}
        />
      )}
      {editing ? (
        <RoleDialog appId={appId} appName={appName} role={editing === "new" ? null : editing} groups={groups} people={people} onClose={() => setEditing(null)} />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.common.deleteConfirm(deleting?.name ?? "")}
        description={r.deleteHint(appName)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.common.cancel}
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                deleting &&
                run(
                  "delete",
                  () => api(`/applications/${appId}/roles/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: t.common.deletedToast(deleting.name), description: t.common.audit });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              {r.delete}
            </Button>
          </>
        }
      />
    </>
  );
}

function RoleDialog({ appId, appName, role, groups, people, onClose }: { appId: string; appName: string; role: AppRole | null; groups: Group[]; people: Person[]; onClose: () => void }) {
  const [name, setName] = useState(role?.name ?? "");
  const [key, setKey] = useState(role?.key ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [groupIds, setGroupIds] = useState<string[]>(role?.groupIds ?? []);
  const [userIds, setUserIds] = useState<string[]>(role?.userIds ?? []);
  const [errors, setErrors] = useState<{ name?: string; key?: string }>({});
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const r = t.roles;

  function submit() {
    const next = {
      name: name.trim() ? undefined : r.nameRequired,
      key: role || /^[A-Za-z][A-Za-z0-9_.:-]{0,63}$/.test(key.trim()) ? undefined : r.keyInvalid,
    };
    setErrors(next);
    if (next.name || next.key) return;
    const body = { name: name.trim(), description: description.trim(), groupIds, userIds };
    void run(
      "save",
      () =>
        api<AppRole>(role ? `/applications/${appId}/roles/${role.id}` : `/applications/${appId}/roles`, {
          method: role ? "PATCH" : "POST",
          body: role ? body : { ...body, key: key.trim() },
        }),
      (saved) => {
        onClose();
        toast({ title: role ? t.common.savedToast(saved.name) : t.common.addedToast(saved.name), description: r.savedHint(appName) });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={role ? t.common.edit(role.name) : r.add}
      description={r.dialogHint(appName)}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {role ? r.save : r.add}
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
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label={t.common.name} error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={r.placeholder} />}
          </Field>
          <Field label={r.key} error={errors.key} hint={role ? r.keyLocked : r.keyHint(appName)}>
            {(props) => <Input {...props} mono autoComplete="off" disabled={!!role} value={key} onChange={(event) => setKey(event.target.value)} placeholder="editor" />}
          </Field>
        </div>
        <Field label={t.common.description} hint={r.descriptionHint}>
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Checklist
          legend={t.common.groups}
          hint={r.groupsHint}
          items={groups.map((g) => ({ id: g.id, label: g.name, detail: GROUP_KIND[g.kind] }))}
          value={groupIds}
          onChange={setGroupIds}
        />
        <Field label={t.common.people} hint={r.peopleHint}>
          {(props) => <PeoplePicker {...props} people={people} value={userIds} onChange={setUserIds} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

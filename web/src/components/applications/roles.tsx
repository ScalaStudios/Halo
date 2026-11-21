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
import { GROUP_KIND } from "@/lib/labels";
import type { AppRole, Group } from "@/lib/types";
import { Checklist } from "./collections-table";

type Row = AppRole & { holders: { name: string }[] };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "name",
    accessorFn: (r) => r.name,
    header: "Role",
    meta: { label: "Role", hideable: false },
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
    header: "Description",
    meta: { label: "Description" },
    cell: ({ getValue }) => <span className="block max-w-96 min-w-48 py-2">{getValue<string>() || "—"}</span>,
  },
  {
    id: "holders",
    accessorFn: (r) => joinNames(r.holders),
    header: "Held by",
    meta: { label: "Held by" },
    cell: ({ row }) =>
      row.original.holders.length ? (
        <span className="block max-w-96 min-w-48 py-2">{joinNames(row.original.holders)}</span>
      ) : (
        <span className="whitespace-nowrap text-fg-3">Nobody</span>
      ),
  },
];

export function RolesTable({ appId, appName, roles, groups, people }: { appId: string; appName: string; roles: AppRole[]; groups: Group[]; people: Person[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<AppRole | "new" | null>(null);
  const [deleting, setDeleting] = useState<AppRole | null>(null);
  const rows = roles.map((r) => ({
    ...r,
    holders: [...groups.filter((g) => r.groupIds.includes(g.id)), ...people.filter((p) => r.userIds.includes(p.id))],
  }));

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add role
    </Button>
  );

  return (
    <>
      {rows.length === 0 ? (
        <Card>
          <EmptyState
            icon={BadgeCheck}
            title="No roles yet"
            description={`Add roles such as Viewer, Editor or Billing, then give them to groups and people. ${appName} reads them from one roles claim instead of group names.`}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns}
          getRowId={(r) => r.id}
          label="Roles"
          noun={["role", "roles"]}
          storageKey="application-roles"
          searchText={(r) => `${r.name} ${r.key} ${joinNames(r.holders)}`}
          searchPlaceholder="Search roles, groups or people"
          onRowOpen={setEditing}
          rowActions={[
            { label: "Edit role", icon: Pencil, onSelect: setEditing },
            { label: "Delete role", icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
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
        title={`Delete ${deleting?.name ?? ""}?`}
        description={`Everyone who holds it loses it the next time they sign in to ${appName}. It can't be undone.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
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
                    toast({ title: `${deleting.name} deleted`, description: "Recorded in the audit log." });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              Delete role
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

  function submit() {
    const next = {
      name: name.trim() ? undefined : "Enter a role name, like Editor.",
      key: role || /^[A-Za-z][A-Za-z0-9_.:-]{0,63}$/.test(key.trim()) ? undefined : "Start with a letter and use only letters, digits, dots, colons, dashes and underscores, like billing.admin.",
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
        toast({ title: `${saved.name} ${role ? "saved" : "added"}`, description: `${appName} receives it on the next sign-in. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={role ? `Edit ${role.name}` : "Add role"}
      description={`Roles refine what people can do in ${appName}. They don't let anyone sign in: that still takes a group on the Users & groups tab.`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {role ? "Save role" : "Add role"}
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
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Editor" />}
          </Field>
          <Field label="Key" error={errors.key} hint={role ? "Can't be changed." : `What ${appName} receives. Can't be changed later.`}>
            {(props) => <Input {...props} mono autoComplete="off" disabled={!!role} value={key} onChange={(event) => setKey(event.target.value)} placeholder="editor" />}
          </Field>
        </div>
        <Field label="Description" hint="Shown to administrators, and never sent to the application.">
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Checklist
          legend="Groups"
          hint="Members of these groups hold the role."
          items={groups.map((g) => ({ id: g.id, label: g.name, detail: GROUP_KIND[g.kind] }))}
          value={groupIds}
          onChange={setGroupIds}
        />
        <Field label="People" hint="Give the role to someone directly, without a group.">
          {(props) => <PeoplePicker {...props} people={people} value={userIds} onChange={setUserIds} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

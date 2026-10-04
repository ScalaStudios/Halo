"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Bot, Copy, Plus } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import type { RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import { ACCOUNT_STATUS, ROLES, StatusLabel, activeKeys, lastUsed } from "./shared";

export type Person = { id: string; name: string; email: string };
type Row = ServiceAccount & { ownerName: string; activeKeyCount: number; lastUsedAt: string | null };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "name",
    accessorFn: (a) => a.name,
    header: "Name",
    meta: { label: "Name", hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-80 min-w-56 items-center gap-3 py-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
          <Bot aria-hidden="true" size={16} strokeWidth={1.75} />
        </span>
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-medium text-fg">{row.original.name}</span>
          <span className="truncate text-caption text-fg-3">{row.original.description || row.original.email}</span>
        </span>
      </span>
    ),
  },
  {
    id: "status",
    accessorFn: (a) => a.status,
    header: "Status",
    filterFn: facetFilter,
    meta: { label: "Status" },
    cell: ({ row }) => <StatusLabel {...ACCOUNT_STATUS[row.original.status]} />,
  },
  {
    id: "roles",
    accessorFn: (a) => a.roles,
    getUniqueValues: (a) => a.roles,
    header: "Roles",
    filterFn: facetFilter,
    enableSorting: false,
    meta: { label: "Roles" },
    cell: ({ row }) => {
      const [first, ...rest] = row.original.roles;
      return first ? (
        <span className="whitespace-nowrap text-fg">
          {ROLES[first].label}
          {rest.length ? <span className="text-fg-3"> and {rest.length} more</span> : null}
        </span>
      ) : (
        <span className="text-fg-3">No roles</span>
      );
    },
  },
  {
    id: "owner",
    accessorFn: (a) => a.ownerName,
    header: "Owner",
    meta: { label: "Owner" },
    cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>() || "—"}</span>,
  },
  {
    id: "keys",
    accessorFn: (a) => a.activeKeyCount,
    header: "Active keys",
    meta: { label: "Active keys", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "lastUsed",
    accessorFn: (a) => (a.lastUsedAt ? new Date(a.lastUsedAt).getTime() : 0),
    header: "Last used",
    meta: { label: "Last used", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastUsedAt ? <RelativeTime iso={row.original.lastUsedAt} /> : "Never"}</span>,
  },
  {
    id: "created",
    accessorFn: (a) => new Date(a.createdAt).getTime(),
    header: "Created",
    meta: { label: "Created", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt)}</span>,
  },
];

export function ServiceAccountsTable({ accounts, people }: { accounts: ServiceAccount[]; people: Person[] }) {
  const { toast, router } = useMutation();
  const [creating, setCreating] = useState(false);
  const names = Object.fromEntries(people.map((p) => [p.id, p.name]));
  const rows: Row[] = accounts.map((a) => ({ ...a, ownerName: (a.ownerId && names[a.ownerId]) || "", activeKeyCount: activeKeys(a.keys).length, lastUsedAt: lastUsed(a.keys) }));
  const create = (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Create service account
    </Button>
  );

  return (
    <>
      {accounts.length === 0 ? (
        <Card>
          <EmptyState
            icon={Bot}
            title="No service accounts yet"
            description="Create one for each system that automates Halo, such as Terraform or your HR system. Give it only the roles it needs, then create an API key."
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns}
          getRowId={(a) => a.id}
          label="Service accounts"
          noun={["service account", "service accounts"]}
          storageKey="service-accounts"
          searchText={(a) => `${a.name} ${a.description} ${a.ownerName}`}
          searchPlaceholder="Search name, description or owner"
          facets={[
            { column: "status", label: "Status", options: Object.entries(ACCOUNT_STATUS).map(([value, s]) => ({ value, label: s.label })) },
            { column: "roles", label: "Role", options: Object.entries(ROLES).map(([value, r]) => ({ value, label: r.label })) },
          ]}
          onRowOpen={(a) => router.push(`/admin/service-accounts/${a.id}`)}
          rowActions={[
            { label: "Open service account", icon: Bot, onSelect: (a) => router.push(`/admin/service-accounts/${a.id}`) },
            {
              label: "Copy service account ID",
              icon: Copy,
              onSelect: (a) => {
                void navigator.clipboard?.writeText(a.id);
                toast({ title: "Service account ID copied", description: a.id });
              },
            },
          ]}
          toolbarEnd={create}
        />
      )}
      <CreateServiceAccountDialog open={creating} onOpenChange={setCreating} people={people} />
    </>
  );
}

export function RoleCheckboxes({ selected, onChange }: { selected: RoleKey[]; onChange: (roles: RoleKey[]) => void }) {
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-3 text-label text-fg">Roles</legend>
      {(Object.keys(ROLES) as RoleKey[]).map((role) => (
        <label key={role} className="flex cursor-pointer items-start gap-3">
          <Checkbox
            className="mt-0.5"
            checked={selected.includes(role)}
            onChange={(event) => onChange(event.target.checked ? [...selected, role] : selected.filter((r) => r !== role))}
          />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{ROLES[role].label}</span>
            <span className="text-caption text-fg-3">{ROLES[role].description}</span>
          </span>
        </label>
      ))}
    </fieldset>
  );
}

function CreateServiceAccountDialog({ open, onOpenChange, people }: { open: boolean; onOpenChange: (open: boolean) => void; people: Person[] }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [ownerId, setOwnerId] = useState("");
  const [roles, setRoles] = useState<RoleKey[]>([]);
  const [error, setError] = useState<string>();
  const { pending, run, toast, router } = useMutation();

  function close() {
    onOpenChange(false);
    setName("");
    setDescription("");
    setOwnerId("");
    setRoles([]);
    setError(undefined);
  }

  function submit() {
    if (!name.trim()) {
      setError("Enter a name that says what the service account automates.");
      return;
    }
    void run(
      "create",
      () => api<ServiceAccount>("/service-accounts", { body: { name: name.trim(), description: description.trim(), ownerId: ownerId || null, roles } }),
      (account) => {
        close();
        toast({ title: `${account.name} created`, description: "Create an API key so it can authenticate." });
        router.push(`/admin/service-accounts/${account.id}`);
      },
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title="Create service account"
      description="A non-human identity for automation. It can't sign in to the console; it authenticates with API keys and acts with the roles you give it here."
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "create"} onClick={submit}>
            Create service account
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
        <Field label="Name" error={error}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Terraform" />}
        </Field>
        <Field label="Description" hint="Optional. Say what it automates and where it runs.">
          {(props) => <Input {...props} autoComplete="off" value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Field label="Owner" hint="The person to contact about this service account.">
          {(props) => (
            <Select {...props} value={ownerId} onChange={(event) => setOwnerId(event.target.value)}>
              <option value="">No owner</option>
              {people.map((person) => (
                <option key={person.id} value={person.id}>
                  {person.name} · {person.email}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <RoleCheckboxes selected={roles} onChange={setRoles} />
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

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
import { useLocale, useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { Locale } from "@/lib/i18n/locale";
import type { RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import { AccountStatus } from "./account-status";
import { activeKeys, lastUsed } from "./shared";

export type Person = { id: string; name: string; email: string };
type Row = ServiceAccount & { ownerName: string; activeKeyCount: number; lastUsedAt: string | null };

const columns = (t: (typeof provisioningMessages)[Locale], locale: Locale): ColumnDef<Row, any>[] => [
  {
    id: "name",
    accessorFn: (a) => a.name,
    header: t.name,
    meta: { label: t.name, hideable: false },
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
    header: t.status,
    filterFn: facetFilter,
    meta: { label: t.status },
    cell: ({ row }) => <AccountStatus status={row.original.status} />,
  },
  {
    id: "roles",
    accessorFn: (a) => a.roles,
    getUniqueValues: (a) => a.roles,
    header: t.roles,
    filterFn: facetFilter,
    enableSorting: false,
    meta: { label: t.roles },
    cell: ({ row }) => {
      const [first, ...rest] = row.original.roles;
      return first ? (
        <span className="whitespace-nowrap text-fg">
          {t.role[first].label}
          {rest.length ? <span className="text-fg-3">{t.serviceAccounts.andMore(rest.length)}</span> : null}
        </span>
      ) : (
        <span className="text-fg-3">{t.serviceAccounts.noRoles}</span>
      );
    },
  },
  {
    id: "owner",
    accessorFn: (a) => a.ownerName,
    header: t.serviceAccounts.owner,
    meta: { label: t.serviceAccounts.owner },
    cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>() || "—"}</span>,
  },
  {
    id: "keys",
    accessorFn: (a) => a.activeKeyCount,
    header: t.serviceAccounts.activeKeys,
    meta: { label: t.serviceAccounts.activeKeys, align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "lastUsed",
    accessorFn: (a) => (a.lastUsedAt ? new Date(a.lastUsedAt).getTime() : 0),
    header: t.lastUsed,
    meta: { label: t.lastUsed, align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastUsedAt ? <RelativeTime iso={row.original.lastUsedAt} /> : t.never}</span>,
  },
  {
    id: "created",
    accessorFn: (a) => new Date(a.createdAt).getTime(),
    header: t.created,
    meta: { label: t.created, align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt, locale)}</span>,
  },
];

export function ServiceAccountsTable({ accounts, people }: { accounts: ServiceAccount[]; people: Person[] }) {
  const { toast, router } = useMutation();
  const [creating, setCreating] = useState(false);
  const locale = useLocale();
  const t = provisioningMessages[locale];
  const names = Object.fromEntries(people.map((p) => [p.id, p.name]));
  const rows: Row[] = accounts.map((a) => ({ ...a, ownerName: (a.ownerId && names[a.ownerId]) || "", activeKeyCount: activeKeys(a.keys).length, lastUsedAt: lastUsed([...a.keys, ...a.federatedCredentials]) }));
  const create = (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.serviceAccounts.create}
    </Button>
  );

  return (
    <>
      {accounts.length === 0 ? (
        <Card>
          <EmptyState
            icon={Bot}
            title={t.serviceAccounts.empty}
            description={t.serviceAccounts.emptyHint}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns(t, locale)}
          getRowId={(a) => a.id}
          label={t.serviceAccounts.label}
          noun={t.serviceAccounts.noun}
          storageKey="service-accounts"
          searchText={(a) => `${a.name} ${a.description} ${a.ownerName}`}
          searchPlaceholder={t.serviceAccounts.search}
          facets={[
            { column: "status", label: t.status, options: Object.entries(t.accountStatus).map(([value, label]) => ({ value, label })) },
            { column: "roles", label: t.serviceAccounts.roleFacet, options: Object.entries(t.role).map(([value, r]) => ({ value, label: r.label })) },
          ]}
          onRowOpen={(a) => router.push(`/admin/service-accounts/${a.id}`)}
          rowActions={[
            { label: t.serviceAccounts.open, icon: Bot, onSelect: (a) => router.push(`/admin/service-accounts/${a.id}`) },
            {
              label: t.serviceAccounts.copyId,
              icon: Copy,
              onSelect: (a) => {
                void navigator.clipboard?.writeText(a.id);
                toast({ title: t.serviceAccounts.idCopied, description: a.id });
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
  const t = useMessages(provisioningMessages);
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-3 text-label text-fg">{t.roles}</legend>
      {(Object.keys(t.role) as RoleKey[]).map((role) => (
        <label key={role} className="flex cursor-pointer items-start gap-3">
          <Checkbox
            className="mt-0.5"
            checked={selected.includes(role)}
            onChange={(event) => onChange(event.target.checked ? [...selected, role] : selected.filter((r) => r !== role))}
          />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{t.role[role].label}</span>
            <span className="text-caption text-fg-3">{t.role[role].description}</span>
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
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const t = useMessages(provisioningMessages);

  function close() {
    onOpenChange(false);
    clearError();
    setName("");
    setDescription("");
    setOwnerId("");
    setRoles([]);
    setError(undefined);
  }

  function submit() {
    if (!name.trim()) {
      setError(t.serviceAccounts.nameError);
      return;
    }
    void run(
      "create",
      () => api<ServiceAccount>("/service-accounts", { body: { name: name.trim(), description: description.trim(), ownerId: ownerId || null, roles } }),
      (account) => {
        close();
        toast({ title: t.serviceAccounts.created(account.name), description: t.serviceAccounts.createdHint });
        router.push(`/admin/service-accounts/${account.id}`);
      },
      "inline",
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      error={errorFor("create")}
      title={t.serviceAccounts.create}
      description={t.serviceAccounts.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "create"} onClick={submit}>
            {t.serviceAccounts.create}
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
        <Field label={t.name} error={error}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.serviceAccounts.namePlaceholder} />}
        </Field>
        <Field label={t.description} hint={t.serviceAccounts.descriptionHint}>
          {(props) => <Input {...props} autoComplete="off" value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Field label={t.serviceAccounts.owner} hint={t.serviceAccounts.ownerHint}>
          {(props) => (
            <Select {...props} value={ownerId} onChange={(event) => setOwnerId(event.target.value)}>
              <option value="">{t.serviceAccounts.noOwner}</option>
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

"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Layers, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { joinNames } from "@/components/governance/shared";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Locale } from "@/lib/i18n/locale";
import type { Application, Collection, Group } from "@/lib/types";

type Row = Collection & { applications: Application[]; groups: Group[] };

const columns = (t: (typeof applicationsMessages)[Locale], locale: Locale): ColumnDef<Row, any>[] => [
  {
    id: "name",
    accessorFn: (c) => c.name,
    header: t.collections.collection,
    meta: { label: t.collections.collection, hideable: false },
    cell: ({ getValue }) => <span className="font-medium whitespace-nowrap text-fg">{getValue<string>()}</span>,
  },
  {
    id: "applications",
    accessorFn: (c) => joinNames(c.applications, locale),
    header: t.common.applications,
    meta: { label: t.common.applications },
    cell: ({ row }) => <Names names={row.original.applications} empty={t.collections.noApplications} />,
  },
  {
    id: "groups",
    accessorFn: (c) => joinNames(c.groups, locale),
    header: t.collections.shownTo,
    meta: { label: t.collections.shownTo },
    cell: ({ row }) => <Names names={row.original.groups} empty={t.common.nobody} />,
  },
];

function Names({ names, empty }: { names: { name: string }[]; empty: string }) {
  const locale = useLocale();
  return names.length ? <span className="block max-w-96 min-w-48 py-2">{joinNames(names, locale)}</span> : <span className="whitespace-nowrap text-fg-3">{empty}</span>;
}

export function CollectionsTable({ collections, applications, groups }: { collections: Collection[]; applications: Application[]; groups: Group[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const c = t.collections;
  const [editing, setEditing] = useState<Row | "new" | null>(null);
  const [deleting, setDeleting] = useState<Row | null>(null);
  const rows = collections.map((c) => ({
    ...c,
    applications: applications.filter((a) => c.applicationIds.includes(a.id)),
    groups: groups.filter((g) => c.groupIds.includes(g.id)),
  }));

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {c.create}
    </Button>
  );

  return (
    <>
      {rows.length === 0 ? (
        <Card>
          <EmptyState
            icon={Layers}
            title={c.empty}
            description={c.emptyHint}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns(t, locale)}
          getRowId={(c) => c.id}
          label={c.label}
          noun={c.noun}
          storageKey="collections"
          searchText={(c) => `${c.name} ${joinNames(c.applications, locale)} ${joinNames(c.groups, locale)}`}
          searchPlaceholder={c.search}
          onRowOpen={setEditing}
          rowActions={[
            { label: c.edit, icon: Pencil, onSelect: setEditing },
            { label: c.delete, icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
          ]}
          toolbarEnd={create}
        />
      )}
      {editing ? (
        <CollectionDialog
          collection={editing === "new" ? null : editing}
          applications={applications.filter((a) => a.type !== "service")}
          groups={groups}
          onClose={() => setEditing(null)}
        />
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
        description={c.deleteHint}
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
                  () => api(`/collections/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: t.common.deletedToast(deleting.name), description: t.common.audit });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              {c.delete}
            </Button>
          </>
        }
      />
    </>
  );
}

function CollectionDialog({ collection, applications, groups, onClose }: { collection: Row | null; applications: Application[]; groups: Group[]; onClose: () => void }) {
  const [name, setName] = useState(collection?.name ?? "");
  const [applicationIds, setApplicationIds] = useState<string[]>(collection?.applicationIds ?? []);
  const [groupIds, setGroupIds] = useState<string[]>(collection?.groupIds ?? []);
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const kinds = useMessages(directoryMessages).groupKind;
  const c = t.collections;

  function submit() {
    setError(name.trim() ? undefined : c.nameRequired);
    if (!name.trim()) return;
    void run(
      "save",
      () =>
        api<Collection>(collection ? `/collections/${collection.id}` : "/collections", {
          method: collection ? "PUT" : "POST",
          body: { name: name.trim(), applicationIds, groupIds },
        }),
      (saved) => {
        onClose();
        toast({ title: collection ? t.common.savedToast(saved.name) : t.common.createdToast(saved.name), description: c.savedHint });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={collection ? t.common.edit(collection.name) : c.create}
      description={c.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {collection ? c.save : c.create}
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
        <Field label={t.common.name} error={error}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={c.placeholder} />}
        </Field>
        <Checklist
          legend={t.common.applications}
          hint={c.applicationsHint}
          items={applications.map((a) => ({ id: a.id, label: a.name, detail: t.protocols[a.protocol] }))}
          value={applicationIds}
          onChange={setApplicationIds}
        />
        <Checklist
          legend={c.shownTo}
          hint={c.shownToHint}
          items={groups.map((g) => ({ id: g.id, label: g.name, detail: kinds[g.kind] }))}
          value={groupIds}
          onChange={setGroupIds}
        />
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

export function Checklist({
  legend,
  hint,
  items,
  value,
  onChange,
}: {
  legend: string;
  hint: string;
  items: { id: string; label: string; detail: string }[];
  value: string[];
  onChange: (value: string[]) => void;
}) {
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-3 text-label text-fg">{legend}</legend>
      <div className="flex max-h-56 flex-col gap-3 overflow-y-auto rounded-md border border-border p-4">
        {items.map((item) => (
          <label key={item.id} className="flex cursor-pointer items-start gap-3">
            <Checkbox
              className="mt-0.5"
              checked={value.includes(item.id)}
              onChange={(event) => onChange(event.target.checked ? [...value, item.id] : value.filter((id) => id !== item.id))}
            />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{item.label}</span>
              <span className="text-caption text-fg-3">{item.detail}</span>
            </span>
          </label>
        ))}
      </div>
      <p className="text-caption text-fg-3">{hint}</p>
    </fieldset>
  );
}

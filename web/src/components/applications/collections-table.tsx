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
import { GROUP_KIND } from "@/lib/labels";
import type { Application, Collection, Group } from "@/lib/types";
import { PROTOCOL_LABEL } from "./shared";

type Row = Collection & { applications: Application[]; groups: Group[] };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "name",
    accessorFn: (c) => c.name,
    header: "Collection",
    meta: { label: "Collection", hideable: false },
    cell: ({ getValue }) => <span className="font-medium whitespace-nowrap text-fg">{getValue<string>()}</span>,
  },
  {
    id: "applications",
    accessorFn: (c) => joinNames(c.applications),
    header: "Applications",
    meta: { label: "Applications" },
    cell: ({ row }) => <Names names={row.original.applications} empty="No applications" />,
  },
  {
    id: "groups",
    accessorFn: (c) => joinNames(c.groups),
    header: "Shown to",
    meta: { label: "Shown to" },
    cell: ({ row }) => <Names names={row.original.groups} empty="Nobody" />,
  },
];

function Names({ names, empty }: { names: { name: string }[]; empty: string }) {
  return names.length ? <span className="block max-w-96 min-w-48 py-2">{joinNames(names)}</span> : <span className="whitespace-nowrap text-fg-3">{empty}</span>;
}

export function CollectionsTable({ collections, applications, groups }: { collections: Collection[]; applications: Application[]; groups: Group[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
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
      Create collection
    </Button>
  );

  return (
    <>
      {rows.length === 0 ? (
        <Card>
          <EmptyState
            icon={Layers}
            title="No collections yet"
            description="Create a collection to give the account portal a named section, like Engineering or HR, for the groups you choose."
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rows}
          columns={columns}
          getRowId={(c) => c.id}
          label="Collections"
          noun={["collection", "collections"]}
          storageKey="collections"
          searchText={(c) => `${c.name} ${joinNames(c.applications)} ${joinNames(c.groups)}`}
          searchPlaceholder="Search collections, applications or groups"
          onRowOpen={setEditing}
          rowActions={[
            { label: "Edit collection", icon: Pencil, onSelect: setEditing },
            { label: "Delete collection", icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
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
        title={`Delete ${deleting?.name ?? ""}?`}
        description="People stop seeing this section in their account portal. The applications and their group assignments don't change."
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
                  () => api(`/collections/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: `${deleting.name} deleted`, description: "Recorded in the audit log." });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              Delete collection
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

  function submit() {
    setError(name.trim() ? undefined : "Enter a collection name, like Engineering.");
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
        toast({ title: `${saved.name} ${collection ? "saved" : "created"}`, description: "Members of its groups see it in their account portal." });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={collection ? `Edit ${collection.name}` : "Create collection"}
      description="A named section of the account portal. People see the applications in it that they already have access to."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {collection ? "Save collection" : "Create collection"}
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
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Engineering" />}
        </Field>
        <Checklist
          legend="Applications"
          hint="Adding an application here doesn't give anyone access to it."
          items={applications.map((a) => ({ id: a.id, label: a.name, detail: PROTOCOL_LABEL[a.protocol] }))}
          value={applicationIds}
          onChange={setApplicationIds}
        />
        <Checklist
          legend="Shown to"
          hint="Members of these groups see the collection."
          items={groups.map((g) => ({ id: g.id, label: g.name, detail: GROUP_KIND[g.kind] }))}
          value={groupIds}
          onChange={setGroupIds}
        />
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function Checklist({
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

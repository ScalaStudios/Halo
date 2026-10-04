"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Archive, Package, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { AccessPackage } from "@/lib/governance-types";
import type { Group } from "@/lib/types";
import { PeoplePicker } from "./people-picker";
import { DURATIONS, durationLabel, joinNames, type Person } from "./shared";

const columns: ColumnDef<AccessPackage, any>[] = [
  {
    id: "name",
    accessorFn: (p) => p.name,
    header: "Package",
    meta: { label: "Package", hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-80 min-w-56 flex-col py-2">
        <span className="font-medium text-fg">{row.original.name}</span>
        <span className="text-caption text-fg-3">{row.original.description}</span>
      </span>
    ),
  },
  {
    id: "groups",
    accessorFn: (p) => joinNames(p.groups),
    header: "Grants",
    meta: { label: "Grants" },
    cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>()}</span>,
  },
  {
    id: "approvers",
    accessorFn: (p) => joinNames(p.approvers),
    header: "Approvers",
    meta: { label: "Approvers" },
    cell: ({ row }) =>
      row.original.approvers.length ? (
        <span className="whitespace-nowrap">{joinNames(row.original.approvers)}</span>
      ) : (
        <span className="whitespace-nowrap text-fg-3">Global administrators</span>
      ),
  },
  {
    id: "duration",
    accessorFn: (p) => p.maxDays ?? Number.MAX_SAFE_INTEGER,
    header: "Maximum",
    meta: { label: "Maximum duration", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap">{durationLabel(row.original.maxDays)}</span>,
  },
  {
    id: "pending",
    accessorFn: (p) => p.pendingRequests,
    header: "Pending",
    meta: { label: "Pending requests", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "active",
    accessorFn: (p) => p.activeGrants,
    header: "Active",
    meta: { label: "Active grants", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "status",
    accessorFn: (p) => (p.archived ? "archived" : "requestable"),
    header: "Status",
    filterFn: facetFilter,
    meta: { label: "Status" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={row.original.archived ? "neutral" : "success"} />
        {row.original.archived ? "Archived" : "Requestable"}
      </span>
    ),
  },
];

const views: SavedView[] = [
  { id: "requestable", label: "Requestable", filters: [{ id: "status", value: ["requestable"] }] },
  { id: "archived", label: "Archived", filters: [{ id: "status", value: ["archived"] }] },
  { id: "all", label: "All packages", filters: [] },
];

function bodyFor(p: AccessPackage) {
  return {
    name: p.name,
    description: p.description,
    ownerId: p.owner?.id ?? null,
    groupIds: p.groups.map((g) => g.id),
    approverIds: p.approvers.map((a) => a.id),
    maxDays: p.maxDays,
    requireJustification: p.requireJustification,
    archived: p.archived,
  };
}

export function PackagesTable({ packages, groups, people }: { packages: AccessPackage[]; groups: Group[]; people: Person[] }) {
  const { pending, run, toast } = useMutation();
  const [editing, setEditing] = useState<AccessPackage | "new" | null>(null);
  const [deleting, setDeleting] = useState<AccessPackage | null>(null);

  function setArchived(p: AccessPackage, archived: boolean) {
    void run(
      "archive",
      () => api(`/access-packages/${p.id}`, { method: "PUT", body: { ...bodyFor(p), archived } }),
      () =>
        toast(
          archived
            ? { title: `${p.name} archived`, description: "People can no longer request it. Access already granted lasts until it expires." }
            : { title: `${p.name} restored`, description: "People can request it again from their account." },
        ),
    );
  }

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Create package
    </Button>
  );

  return (
    <>
      {packages.length === 0 ? (
        <Card>
          <EmptyState
            icon={Package}
            title="No access packages yet"
            description="Create a package so people can request time-bound membership of groups, decided by approvers you choose."
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={packages}
          columns={columns}
          getRowId={(p) => p.id}
          label="Access packages"
          noun={["package", "packages"]}
          storageKey="access-packages"
          searchText={(p) => `${p.name} ${p.description} ${joinNames(p.groups)} ${joinNames(p.approvers)}`}
          searchPlaceholder="Search packages, groups or approvers"
          views={views}
          facets={[
            {
              column: "status",
              label: "Status",
              options: [
                { value: "requestable", label: "Requestable" },
                { value: "archived", label: "Archived" },
              ],
            },
          ]}
          onRowOpen={setEditing}
          rowActions={[
            { label: "Edit package", icon: Pencil, onSelect: setEditing },
            { label: "Delete package", icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
          ]}
          toolbarEnd={create}
        />
      )}
      {editing ? <PackageDialog pkg={editing === "new" ? null : editing} groups={groups} people={people} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => (open ? undefined : setDeleting(null))}
        title={`Delete ${deleting?.name ?? ""}?`}
        description="Only packages nobody has requested can be deleted. To stop new requests and keep the history, archive the package instead."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            {deleting?.archived === false ? (
              <Button
                variant="secondary"
                loading={pending === "archive"}
                onClick={() => {
                  setArchived(deleting, true);
                  setDeleting(null);
                }}
              >
                <Archive aria-hidden="true" size={16} strokeWidth={1.75} />
                Archive instead
              </Button>
            ) : null}
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                deleting &&
                run(
                  "delete",
                  () => api(`/access-packages/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: `${deleting.name} deleted`, description: "Recorded in the audit log." });
                    setDeleting(null);
                  },
                )
              }
            >
              Delete package
            </Button>
          </>
        }
      />
    </>
  );
}

function PackageDialog({ pkg, groups, people, onClose }: { pkg: AccessPackage | null; groups: Group[]; people: Person[]; onClose: () => void }) {
  const [name, setName] = useState(pkg?.name ?? "");
  const [description, setDescription] = useState(pkg?.description ?? "");
  const [groupIds, setGroupIds] = useState<string[]>(pkg?.groups.map((g) => g.id) ?? []);
  const [approverIds, setApproverIds] = useState<string[]>(pkg?.approvers.map((a) => a.id) ?? []);
  const [ownerId, setOwnerId] = useState(pkg?.owner?.id ?? "");
  const [maxDays, setMaxDays] = useState(pkg ? (pkg.maxDays === null ? "none" : String(pkg.maxDays)) : "7");
  const [requireJustification, setRequireJustification] = useState(pkg?.requireJustification ?? true);
  const [archived, setArchived] = useState(pkg?.archived ?? false);
  const [errors, setErrors] = useState<{ name?: string; groups?: string }>({});
  const { pending, run, toast } = useMutation();
  const durations = [...new Set([...DURATIONS, ...(pkg?.maxDays ? [pkg.maxDays] : [])])].sort((a, b) => a - b);

  function submit() {
    const next = { name: name.trim() ? undefined : "Enter a package name.", groups: groupIds.length ? undefined : "Choose at least one group to grant." };
    setErrors(next);
    if (next.name || next.groups) return;
    const body = {
      name: name.trim(),
      description: description.trim(),
      ownerId: ownerId || null,
      groupIds,
      approverIds,
      maxDays: maxDays === "none" ? null : Number(maxDays),
      requireJustification,
      archived,
    };
    void run(
      "save",
      () => api<AccessPackage>(pkg ? `/access-packages/${pkg.id}` : "/access-packages", { method: pkg ? "PUT" : "POST", body }),
      (saved) => {
        onClose();
        toast(
          pkg
            ? { title: `${saved.name} saved`, description: "Changes apply to new requests. Access already granted keeps its end date." }
            : { title: `${saved.name} created`, description: "People can request it from the Access page in their account." },
        );
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title={pkg ? `Edit ${pkg.name}` : "Create access package"}
      description="A package bundles groups people can request. Approved access is removed automatically when it expires."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {pkg ? "Save package" : "Create package"}
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
        <Field label="Name" error={errors.name}>
          {(props) => (
            <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Production access" />
          )}
        </Field>
        <Field label="Description" hint="Shown to people choosing what to request.">
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">Groups</legend>
          <div className="flex max-h-56 flex-col gap-3 overflow-y-auto rounded-md border border-border p-4">
            {groups.map((group) => (
              <label key={group.id} className="flex cursor-pointer items-start gap-3">
                <Checkbox
                  className="mt-0.5"
                  checked={groupIds.includes(group.id)}
                  onChange={(event) => setGroupIds((current) => (event.target.checked ? [...current, group.id] : current.filter((id) => id !== group.id)))}
                />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{group.name}</span>
                  <span className="text-caption text-fg-3">{group.description}</span>
                </span>
              </label>
            ))}
          </div>
          {errors.groups ? (
            <p className="text-caption text-danger">{errors.groups}</p>
          ) : (
            <p className="text-caption text-fg-3">
              Only assigned groups are listed. Rule-based groups follow people&apos;s profiles, so they can&apos;t be granted.
            </p>
          )}
        </fieldset>
        <Field label="Approvers" hint="Any one of them can approve. Global administrators can always approve. Nobody can approve their own request.">
          {(props) => <PeoplePicker {...props} people={people} value={approverIds} onChange={setApproverIds} />}
        </Field>
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label="Owner" hint="Who answers questions about this package.">
            {(props) => (
              <Select {...props} value={ownerId} onChange={(event) => setOwnerId(event.target.value)}>
                <option value="">No owner</option>
                {people.map((person) => (
                  <option key={person.id} value={person.id}>
                    {person.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Maximum duration" hint="Requesters choose up to this.">
            {(props) => (
              <Select {...props} value={maxDays} onChange={(event) => setMaxDays(event.target.value)}>
                {durations.map((days) => (
                  <option key={days} value={String(days)}>
                    {durationLabel(days)}
                  </option>
                ))}
                <option value="none">Until revoked</option>
              </Select>
            )}
          </Field>
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={requireJustification} onChange={(event) => setRequireJustification(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Require a justification</span>
            <span className="text-caption text-fg-3">Requesters explain why they need access. Approvers read it before deciding.</span>
          </span>
        </label>
        {pkg ? (
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={archived} onChange={(event) => setArchived(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Archived</span>
              <span className="text-caption text-fg-3">People can&apos;t request an archived package. Access already granted lasts until it expires.</span>
            </span>
          </label>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

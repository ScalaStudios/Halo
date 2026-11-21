"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Tags, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import { ATTRIBUTE_TYPE } from "@/lib/labels";
import type { Attribute, AttributeType } from "@/lib/types";

const columns: ColumnDef<Attribute, any>[] = [
  {
    id: "label",
    accessorFn: (a) => a.label,
    header: "Attribute",
    meta: { label: "Attribute", hideable: false },
    cell: ({ row }) => (
      <span className="flex flex-col py-2">
        <span className="font-medium whitespace-nowrap text-fg">{row.original.label}</span>
        <span className="font-mono text-code-sm text-fg-3">{row.original.name}</span>
      </span>
    ),
  },
  { id: "type", accessorFn: (a) => ATTRIBUTE_TYPE[a.type], header: "Type", meta: { label: "Type" } },
  {
    id: "editable",
    accessorFn: (a) => (a.selfService ? "People and administrators" : "Administrators"),
    header: "Who can change it",
    meta: { label: "Who can change it" },
    cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>()}</span>,
  },
  {
    id: "people",
    accessorFn: (a) => a.people,
    header: "Set for",
    meta: { label: "Set for" },
    cell: ({ getValue }) => <span className="tnum whitespace-nowrap">{pluralize(getValue<number>(), "person", "people")}</span>,
  },
];

export function AttributesTable({ attributes, canEdit }: { attributes: Attribute[]; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<Attribute | "new" | null>(null);
  const [deleting, setDeleting] = useState<Attribute | null>(null);

  const create = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add attribute
    </Button>
  ) : undefined;

  return (
    <>
      {attributes.length === 0 ? (
        <Card>
          <EmptyState
            icon={Tags}
            title="No custom attributes yet"
            description="Add an attribute, such as an employee number or a cost centre, to store it on each person and send it to applications."
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={attributes}
          columns={columns}
          getRowId={(a) => a.id}
          label="User attributes"
          noun={["attribute", "attributes"]}
          storageKey="attributes"
          searchText={(a) => `${a.label} ${a.name}`}
          searchPlaceholder="Search attributes"
          onRowOpen={canEdit ? setEditing : undefined}
          rowActions={
            canEdit
              ? [
                  { label: "Edit attribute", icon: Pencil, onSelect: setEditing },
                  { label: "Delete attribute", icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
                ]
              : undefined
          }
          toolbarEnd={create}
        />
      )}
      {editing ? <AttributeDialog attribute={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={`Delete ${deleting?.label ?? ""}?`}
        description={`This removes the value from ${pluralize(deleting?.people ?? 0, "person", "people")} and from outbound SCIM. It can't be undone.`}
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
                  () => api(`/attributes/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: `${deleting.label} deleted`, description: "Recorded in the audit log." });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              Delete attribute
            </Button>
          </>
        }
      />
    </>
  );
}

function AttributeDialog({ attribute, onClose }: { attribute: Attribute | null; onClose: () => void }) {
  const [label, setLabel] = useState(attribute?.label ?? "");
  const [name, setName] = useState(attribute?.name ?? "");
  const [type, setType] = useState<AttributeType>(attribute?.type ?? "string");
  const [selfService, setSelfService] = useState(attribute?.selfService ?? false);
  const [errors, setErrors] = useState<{ label?: string; name?: string }>({});
  const { pending, run, errorFor, toast } = useMutation();

  function submit() {
    const next = {
      label: label.trim() ? undefined : "Enter a display name, like Employee number.",
      name: attribute || /^[A-Za-z][A-Za-z0-9_]{0,63}$/.test(name.trim()) ? undefined : "Start with a letter and use only letters, digits and underscores, like employeeNumber.",
    };
    setErrors(next);
    if (next.label || next.name) return;
    void run(
      "save",
      () =>
        api<Attribute>(attribute ? `/attributes/${attribute.id}` : "/attributes", {
          method: attribute ? "PATCH" : "POST",
          body: attribute ? { label: label.trim(), selfService } : { label: label.trim(), name: name.trim(), type, selfService },
        }),
      (saved) => {
        onClose();
        toast({ title: `${saved.label} ${attribute ? "saved" : "added"}`, description: "Recorded in the audit log." });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={attribute ? `Edit ${attribute.label}` : "Add attribute"}
      description="Administrators set the value on each person's profile. Map it to a claim on an application to send it at sign-in."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {attribute ? "Save attribute" : "Add attribute"}
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
        <Field label="Display name" error={errors.label}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Employee number" />}
        </Field>
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label="Name" error={errors.name} hint={attribute ? "Can't be changed." : "Used in claims, SCIM and the API. Can't be changed later."}>
            {(props) => (
              <Input {...props} mono autoComplete="off" disabled={!!attribute} value={name} onChange={(event) => setName(event.target.value)} placeholder="employeeNumber" />
            )}
          </Field>
          <Field label="Type" hint={attribute ? "Can't be changed." : undefined}>
            {(props) => (
              <Select {...props} disabled={!!attribute} value={type} onChange={(event) => setType(event.target.value as AttributeType)}>
                {(Object.keys(ATTRIBUTE_TYPE) as AttributeType[]).map((key) => (
                  <option key={key} value={key}>
                    {ATTRIBUTE_TYPE[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={selfService} onChange={(event) => setSelfService(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">People can change their own value</span>
            <span className="text-caption text-fg-3">They edit it on their account page. Leave this off for values applications rely on, such as a licence type.</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

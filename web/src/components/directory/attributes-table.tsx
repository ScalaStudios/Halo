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
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Attribute, AttributeType } from "@/lib/types";

function attributeColumns(t: (typeof directoryMessages)["en"]): ColumnDef<Attribute, any>[] {
  return [
    {
      id: "label",
      accessorFn: (a) => a.label,
      header: t.attributes.attribute,
      meta: { label: t.attributes.attribute, hideable: false },
      cell: ({ row }) => (
        <span className="flex flex-col py-2">
          <span className="font-medium whitespace-nowrap text-fg">{row.original.label}</span>
          <span className="font-mono text-code-sm text-fg-3">{row.original.name}</span>
        </span>
      ),
    },
    { id: "type", accessorFn: (a) => t.attributeType[a.type], header: t.fields.type, meta: { label: t.fields.type } },
    {
      id: "editable",
      accessorFn: (a) => (a.selfService ? t.attributes.peopleAndAdmins : t.attributes.admins),
      header: t.attributes.whoCanChange,
      meta: { label: t.attributes.whoCanChange },
      cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>()}</span>,
    },
    {
      id: "people",
      accessorFn: (a) => a.people,
      header: t.attributes.setFor,
      meta: { label: t.attributes.setFor },
      cell: ({ getValue }) => <span className="tnum whitespace-nowrap">{t.people(getValue<number>())}</span>,
    },
  ];
}

export function AttributesTable({ attributes, canEdit }: { attributes: Attribute[]; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<Attribute | "new" | null>(null);
  const [deleting, setDeleting] = useState<Attribute | null>(null);
  const t = useMessages(directoryMessages);

  const create = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.attributes.add}
    </Button>
  ) : undefined;

  return (
    <>
      {attributes.length === 0 ? (
        <Card>
          <EmptyState
            icon={Tags}
            title={t.attributes.emptyTitle}
            description={t.attributes.emptyDescription}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={attributes}
          columns={attributeColumns(t)}
          getRowId={(a) => a.id}
          label={t.attributes.label}
          noun={t.attributes.noun}
          storageKey="attributes"
          searchText={(a) => `${a.label} ${a.name}`}
          searchPlaceholder={t.attributes.search}
          onRowOpen={canEdit ? setEditing : undefined}
          rowActions={
            canEdit
              ? [
                  { label: t.attributes.editAttribute, icon: Pencil, onSelect: setEditing },
                  { label: t.attributes.deleteAttribute, icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
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
        title={t.attributes.deleteTitle(deleting?.label ?? "")}
        description={t.attributes.deleteDescription(deleting?.people ?? 0)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.cancel}
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
                    toast({ title: t.attributes.deleted(deleting.label), description: t.recorded });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              {t.attributes.deleteAttribute}
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
  const t = useMessages(directoryMessages);

  function submit() {
    const next = {
      label: label.trim() ? undefined : t.attributes.labelRequired,
      name: attribute || /^[A-Za-z][A-Za-z0-9_]{0,63}$/.test(name.trim()) ? undefined : t.attributes.nameInvalid,
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
        toast({ title: t.attributes.saved(saved.label, Boolean(attribute)), description: t.recorded });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={attribute ? t.attributes.editTitle(attribute.label) : t.attributes.add}
      description={t.attributes.description}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {attribute ? t.attributes.saveAttribute : t.attributes.add}
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
        <Field label={t.attributes.displayName} error={errors.label}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={label} onChange={(event) => setLabel(event.target.value)} placeholder={t.attributes.displayNamePlaceholder} />}
        </Field>
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label={t.fields.name} error={errors.name} hint={attribute ? t.attributes.fixed : t.attributes.nameHint}>
            {(props) => (
              <Input {...props} mono autoComplete="off" disabled={!!attribute} value={name} onChange={(event) => setName(event.target.value)} placeholder="employeeNumber" />
            )}
          </Field>
          <Field label={t.fields.type} hint={attribute ? t.attributes.fixed : undefined}>
            {(props) => (
              <Select {...props} disabled={!!attribute} value={type} onChange={(event) => setType(event.target.value as AttributeType)}>
                {(Object.keys(t.attributeType) as AttributeType[]).map((key) => (
                  <option key={key} value={key}>
                    {t.attributeType[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={selfService} onChange={(event) => setSelfService(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{t.attributes.selfService}</span>
            <span className="text-caption text-fg-3">{t.attributes.selfServiceHint}</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

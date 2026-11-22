"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { inlineRuleError } from "@/components/directory/group-detail";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Group } from "@/lib/types";

function groupColumns(t: (typeof directoryMessages)["en"]): ColumnDef<Group, any>[] {
  return [
    {
      id: "name",
      accessorFn: (g) => g.name,
      header: t.fields.name,
      meta: { label: t.fields.name, hideable: false },
      cell: ({ row }) => (
        <span className="flex min-w-64 flex-col py-2">
          <span className="font-medium text-fg">{row.original.name}</span>
          <span className="text-caption text-fg-3">{row.original.description}</span>
        </span>
      ),
    },
    {
      id: "kind",
      accessorFn: (g) => g.kind,
      header: t.fields.membership,
      filterFn: facetFilter,
      meta: { label: t.fields.membership },
      cell: ({ row }) => (row.original.rule ? <Tag>{row.original.rule}</Tag> : <span className="whitespace-nowrap">{t.groupKind.assigned}</span>),
    },
    {
      id: "source",
      accessorFn: (g) => g.source,
      header: t.fields.source,
      filterFn: facetFilter,
      meta: { label: t.fields.source },
      cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.source}</span>,
    },
    {
      id: "members",
      accessorFn: (g) => g.memberCount,
      header: t.fields.members,
      meta: { label: t.fields.members, align: "right" },
      cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
    },
  ];
}

export function GroupsTable({ groups }: { groups: Group[] }) {
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const t = useMessages(directoryMessages);
  const sources = [...new Set(groups.map((g) => g.source))];

  return (
    <>
      <DataTable
        data={groups}
        columns={groupColumns(t)}
        getRowId={(g) => g.id}
        label={t.fields.groups}
        noun={t.groups.noun}
        storageKey="groups"
        searchText={(g) => `${g.name} ${g.description} ${g.rule ?? ""}`}
        searchPlaceholder={t.groups.search}
        onRowOpen={(g) => router.push(`/admin/groups/${g.id}`)}
        facets={[
          { column: "kind", label: t.fields.membership, options: Object.entries(t.groupKind).map(([value, label]) => ({ value, label })) },
          { column: "source", label: t.fields.source, options: sources.map((s) => ({ value: s, label: s })) },
        ]}
        toolbarEnd={
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.groups.create}
          </Button>
        }
      />
      <CreateGroupDialog open={creating} onOpenChange={setCreating} />
    </>
  );
}

function CreateGroupDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [membership, setMembership] = useState<Group["kind"] | "all">("assigned");
  const [rule, setRule] = useState("");
  const [errors, setErrors] = useState<{ name?: string; rule?: string }>({});
  const { pending, run, toast, router } = useMutation();
  const t = useMessages(directoryMessages);

  function reset() {
    setName("");
    setDescription("");
    setMembership("assigned");
    setRule("");
    setErrors({});
  }

  function submit() {
    const next = {
      name: name.trim() ? undefined : t.validation.groupName,
      rule: membership === "dynamic" && !rule.trim() ? t.groups.ruleOrAssigned : undefined,
    };
    setErrors(next);
    if (next.name || next.rule) return;
    void run(
      "create",
      () =>
        api<Group>("/groups", {
          body: {
            name: name.trim(),
            description: description.trim(),
            kind: membership === "assigned" ? "assigned" : "dynamic",
            rule: membership === "assigned" ? null : membership === "all" ? "true" : rule.trim(),
          },
        }),
      (group) => {
        onOpenChange(false);
        toast({
          title: t.groups.created(group.name),
          description: membership === "assigned" ? t.groups.createdAssigned : t.groups.createdDynamic,
        });
        reset();
        router.push(`/admin/groups/${group.id}`);
      },
      inlineRuleError((message) => setErrors({ rule: message })),
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) reset();
      }}
      title={t.groups.create}
      description={t.groups.createDescription}
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "create"} onClick={submit}>
            {t.groups.create}
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
        <Field label={t.fields.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.groups.namePlaceholder} />}
        </Field>
        <Field label={t.fields.description} hint={t.descriptionHint}>
          {(props) => <Input {...props} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Field label={t.fields.membership}>
          {(props) => (
            <Select {...props} value={membership} onChange={(event) => setMembership(event.target.value as Group["kind"] | "all")}>
              <option value="assigned">{t.groups.membershipAssigned}</option>
              <option value="dynamic">{t.groups.membershipDynamic}</option>
              <option value="all">{t.groups.membershipAll}</option>
            </Select>
          )}
        </Field>
        {membership === "dynamic" ? (
          <Field
            label={t.fields.rule}
            error={errors.rule}
            hint={
              <>
                {t.ruleHint}{" "}
                <code className="text-code-sm text-fg-2">user.email endsWith &quot;@example.com&quot; and user.status == &quot;active&quot;</code>
              </>
            }
          >
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={rule} onChange={(event) => setRule(event.target.value)} placeholder='user.department == "Engineering"' />}
          </Field>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

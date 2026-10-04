"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { Group } from "@/lib/types";

const KIND = { assigned: "Assigned", dynamic: "Rule-based" } as const;

const columns: ColumnDef<Group, any>[] = [
  {
    id: "name",
    accessorFn: (g) => g.name,
    header: "Name",
    meta: { label: "Name", hideable: false },
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
    header: "Membership",
    filterFn: facetFilter,
    meta: { label: "Membership" },
    cell: ({ row }) => (row.original.rule ? <Tag>{row.original.rule}</Tag> : <span className="whitespace-nowrap">Assigned</span>),
  },
  {
    id: "source",
    accessorFn: (g) => g.source,
    header: "Source",
    filterFn: facetFilter,
    meta: { label: "Source" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.source}</span>,
  },
  {
    id: "members",
    accessorFn: (g) => g.memberCount,
    header: "Members",
    meta: { label: "Members", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
];

export function GroupsTable({ groups }: { groups: Group[] }) {
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const sources = [...new Set(groups.map((g) => g.source))];

  return (
    <>
      <DataTable
        data={groups}
        columns={columns}
        getRowId={(g) => g.id}
        label="Groups"
        noun={["group", "groups"]}
        storageKey="groups"
        searchText={(g) => `${g.name} ${g.description} ${g.rule ?? ""}`}
        searchPlaceholder="Search name, description or rule"
        onRowOpen={(g) => router.push(`/admin/groups/${g.id}`)}
        facets={[
          { column: "kind", label: "Membership", options: Object.entries(KIND).map(([value, label]) => ({ value, label })) },
          { column: "source", label: "Source", options: sources.map((s) => ({ value: s, label: s })) },
        ]}
        toolbarEnd={
          <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Create group
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
  const [kind, setKind] = useState<Group["kind"]>("assigned");
  const [rule, setRule] = useState("");
  const [errors, setErrors] = useState<{ name?: string; rule?: string }>({});
  const { pending, run, toast, router } = useMutation();

  function reset() {
    setName("");
    setDescription("");
    setKind("assigned");
    setRule("");
    setErrors({});
  }

  function submit() {
    const next = {
      name: name.trim() ? undefined : "Enter a group name.",
      rule: kind === "dynamic" && !rule.trim() ? "Enter a rule, or switch to assigned membership." : undefined,
    };
    setErrors(next);
    if (next.name || next.rule) return;
    void run(
      "create",
      () => api<Group>("/groups", { body: { name: name.trim(), description: description.trim(), kind, rule: kind === "dynamic" ? rule.trim() : null } }),
      (group) => {
        onOpenChange(false);
        toast({
          title: `${group.name} created`,
          description: kind === "dynamic" ? "People who match the rule are added automatically." : "Add its members here.",
        });
        reset();
        router.push(`/admin/groups/${group.id}`);
      },
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) reset();
      }}
      title="Create group"
      description="Grant access to applications and policies through groups, not individual people."
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "create"} onClick={submit}>
            Create group
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
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Platform on-call" />}
        </Field>
        <Field label="Description" hint="Optional. Helps other administrators pick the right group.">
          {(props) => <Input {...props} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <Field label="Membership">
          {(props) => (
            <Select {...props} value={kind} onChange={(event) => setKind(event.target.value as Group["kind"])}>
              <option value="assigned">Assigned — add people by hand</option>
              <option value="dynamic">Rule-based — members follow a rule</option>
            </Select>
          )}
        </Field>
        {kind === "dynamic" ? (
          <Field
            label="Rule"
            error={errors.rule}
            hint={
              <>
                Compare user attributes, for example <code className="text-code-sm text-fg-2">user.department == &quot;Engineering&quot;</code>
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

"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2, Workflow, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { LifecycleAction, LifecycleActionType, LifecycleRule, LifecycleRun, LifecycleTrigger } from "@/lib/governance-types";
import type { Group } from "@/lib/types";
import { ACTION, TRIGGER } from "./shared";

const groupAction = (type: LifecycleActionType) => type === "add-to-group" || type === "remove-from-group";

function bodyFor(rule: LifecycleRule, enabled = rule.enabled) {
  return { name: rule.name, trigger: rule.trigger, condition: rule.condition, actions: rule.actions, enabled };
}

function RuleToggle({ rule }: { rule: LifecycleRule }) {
  const { pending, run, toast } = useMutation();
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 whitespace-nowrap" onClick={(event) => event.stopPropagation()}>
      <Checkbox
        checked={rule.enabled}
        disabled={pending === "toggle"}
        aria-label={`${rule.name} enabled`}
        onChange={(event) =>
          run(
            "toggle",
            () => api(`/lifecycle/rules/${rule.id}`, { method: "PUT", body: bodyFor(rule, event.target.checked) }),
            () =>
              toast(
                rule.enabled
                  ? { title: `${rule.name} disabled`, description: "Changes that happen while it's disabled won't run it later." }
                  : { title: `${rule.name} enabled`, description: "It runs on the next change Halo detects, within a minute." },
              ),
          )
        }
      />
      {rule.enabled ? "Enabled" : "Disabled"}
    </label>
  );
}

export function RulesTable({ rules, groups }: { rules: LifecycleRule[]; groups: Group[] }) {
  const [editing, setEditing] = useState<LifecycleRule | "new" | null>(null);
  const [deleting, setDeleting] = useState<LifecycleRule | null>(null);
  const { pending, run, toast } = useMutation();
  const describe = useMemo(() => {
    const name = (id?: string) => groups.find((g) => g.id === id)?.name ?? "a deleted group";
    return (a: LifecycleAction) =>
      a.type === "add-to-group" ? `Add to ${name(a.groupId)}` : a.type === "remove-from-group" ? `Remove from ${name(a.groupId)}` : ACTION[a.type];
  }, [groups]);

  const columns = useMemo<ColumnDef<LifecycleRule, any>[]>(
    () => [
      {
        id: "name",
        accessorFn: (r) => r.name,
        header: "Rule",
        meta: { label: "Rule", hideable: false },
        cell: ({ row }) => <span className="font-medium whitespace-nowrap text-fg">{row.original.name}</span>,
      },
      {
        id: "trigger",
        accessorFn: (r) => r.trigger,
        header: "Trigger",
        filterFn: facetFilter,
        meta: { label: "Trigger" },
        cell: ({ row }) => <span className="whitespace-nowrap">{TRIGGER[row.original.trigger].label}</span>,
      },
      {
        id: "condition",
        accessorFn: (r) => r.condition ?? "",
        header: "Condition",
        meta: { label: "Condition" },
        cell: ({ row }) => (row.original.condition ? <Tag>{row.original.condition}</Tag> : <span className="text-fg-3">Everyone</span>),
      },
      {
        id: "actions",
        accessorFn: (r) => r.actions.map(describe).join("; "),
        header: "Actions",
        meta: { label: "Actions" },
        cell: ({ getValue }) => <span className="block max-w-80 min-w-48 py-2">{getValue<string>()}</span>,
      },
      {
        id: "status",
        accessorFn: (r) => (r.enabled ? "enabled" : "disabled"),
        header: "Status",
        filterFn: facetFilter,
        meta: { label: "Status" },
        cell: ({ row }) => <RuleToggle rule={row.original} />,
      },
      {
        id: "runs",
        accessorFn: (r) => r.runs,
        header: "Runs",
        meta: { label: "Runs", align: "right" },
        cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
      },
      {
        id: "lastRun",
        accessorFn: (r) => (r.lastRunAt ? new Date(r.lastRunAt).getTime() : 0),
        header: "Last run",
        meta: { label: "Last run", align: "right" },
        cell: ({ row }) => (
          <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastRunAt ? <RelativeTime iso={row.original.lastRunAt} /> : "Never"}</span>
        ),
      },
    ],
    [describe],
  );

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Create rule
    </Button>
  );

  return (
    <>
      {rules.length === 0 ? (
        <Card>
          <EmptyState
            icon={Workflow}
            title="No lifecycle rules yet"
            description="Create a rule to add or remove groups, end sessions or suspend accounts when people join, move or leave."
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rules}
          columns={columns}
          getRowId={(r) => r.id}
          label="Lifecycle rules"
          noun={["rule", "rules"]}
          storageKey="lifecycle-rules"
          searchText={(r) => `${r.name} ${r.condition ?? ""} ${r.actions.map(describe).join(" ")}`}
          searchPlaceholder="Search rules, conditions or actions"
          facets={[
            {
              column: "trigger",
              label: "Trigger",
              options: (Object.keys(TRIGGER) as LifecycleTrigger[]).map((value) => ({ value, label: TRIGGER[value].label })),
            },
            {
              column: "status",
              label: "Status",
              options: [
                { value: "enabled", label: "Enabled" },
                { value: "disabled", label: "Disabled" },
              ],
            },
          ]}
          onRowOpen={setEditing}
          rowActions={[
            { label: "Edit rule", icon: Pencil, onSelect: setEditing },
            { label: "Delete rule", icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
          ]}
          toolbarEnd={create}
        />
      )}
      {editing ? <RuleDialog rule={editing === "new" ? null : editing} groups={groups} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => (open ? undefined : setDeleting(null))}
        title={`Delete ${deleting?.name ?? ""}?`}
        description={
          deleting?.runs
            ? `Halo stops running it and removes its ${pluralize(deleting.runs, "run")} from the run history; the audit log keeps a record of each run. To keep it for later, disable it instead.`
            : "Halo stops running it. To keep it for later, disable it instead."
        }
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
                  () => api(`/lifecycle/rules/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: `${deleting.name} deleted`, description: "Recorded in the audit log." });
                    setDeleting(null);
                  },
                )
              }
            >
              Delete rule
            </Button>
          </>
        }
      />
    </>
  );
}

function RuleDialog({ rule, groups, onClose }: { rule: LifecycleRule | null; groups: Group[]; onClose: () => void }) {
  const [name, setName] = useState(rule?.name ?? "");
  const [trigger, setTrigger] = useState<LifecycleTrigger>(rule?.trigger ?? "joiner");
  const [condition, setCondition] = useState(rule?.condition ?? "");
  const [actions, setActions] = useState<LifecycleAction[]>(rule?.actions ?? [{ type: "add-to-group" }]);
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [errors, setErrors] = useState<{ name?: string; actions?: string }>({});
  const { pending, run, toast } = useMutation();

  function update(index: number, next: LifecycleAction) {
    setActions((current) => current.map((a, i) => (i === index ? next : a)));
  }

  function submit() {
    const next = {
      name: name.trim() ? undefined : "Enter a rule name.",
      actions: actions.some((a) => groupAction(a.type) && !a.groupId) ? "Choose a group for every add or remove action." : undefined,
    };
    setErrors(next);
    if (next.name || next.actions) return;
    const body = {
      name: name.trim(),
      trigger,
      condition: condition.trim() || null,
      actions: actions.map((a) => (groupAction(a.type) ? a : { type: a.type })),
      enabled,
    };
    void run(
      "save",
      () => api<LifecycleRule>(rule ? `/lifecycle/rules/${rule.id}` : "/lifecycle/rules", { method: rule ? "PUT" : "POST", body }),
      (saved) => {
        onClose();
        toast({
          title: rule ? `${saved.name} saved` : `${saved.name} created`,
          description: "It applies to changes Halo detects from now on, checked every minute.",
        });
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title={rule ? `Edit ${rule.name}` : "Create lifecycle rule"}
      description="Halo compares every account with what it saw last time, every minute, and runs each matching rule once per change."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {rule ? "Save rule" : "Create rule"}
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
            <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Engineering joiners" />
          )}
        </Field>
        <Field label="Trigger" hint={TRIGGER[trigger].description}>
          {(props) => (
            <Select {...props} value={trigger} onChange={(event) => setTrigger(event.target.value as LifecycleTrigger)}>
              {(Object.keys(TRIGGER) as LifecycleTrigger[]).map((value) => (
                <option key={value} value={value}>
                  {TRIGGER[value].label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field
          label="Condition"
          hint={
            <>
              Optional. Only matching people are affected, for example{" "}
              <code className="text-code-sm text-fg-2">user.department == &quot;Engineering&quot;</code>. Leave empty to match everyone.
            </>
          }
        >
          {(props) => (
            <Input
              {...props}
              mono
              autoComplete="off"
              spellCheck={false}
              value={condition}
              onChange={(event) => setCondition(event.target.value)}
              placeholder='user.department == "Engineering"'
            />
          )}
        </Field>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">Actions</legend>
          {actions.map((action, index) => (
            <div key={index} className="flex items-center gap-2">
              <Select
                aria-label={`Action ${index + 1}`}
                value={action.type}
                onChange={(event) => update(index, { type: event.target.value as LifecycleActionType, groupId: action.groupId })}
              >
                {(Object.keys(ACTION) as LifecycleActionType[]).map((value) => (
                  <option key={value} value={value}>
                    {ACTION[value]}
                  </option>
                ))}
              </Select>
              {groupAction(action.type) ? (
                <Select
                  aria-label={`Group for action ${index + 1}`}
                  value={action.groupId ?? ""}
                  onChange={(event) => update(index, { type: action.type, groupId: event.target.value })}
                >
                  <option value="" disabled>
                    Choose a group
                  </option>
                  {groups.map((group) => (
                    <option key={group.id} value={group.id}>
                      {group.name}
                    </option>
                  ))}
                </Select>
              ) : null}
              <Button
                size="icon"
                variant="quiet"
                aria-label={`Remove action ${index + 1}`}
                disabled={actions.length === 1}
                onClick={() => setActions((current) => current.filter((_, i) => i !== index))}
              >
                <X aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </div>
          ))}
          <Button
            size="sm"
            variant="secondary"
            className="self-start"
            disabled={actions.length >= 10}
            onClick={() => setActions((current) => [...current, { type: "revoke-sessions" }])}
          >
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add action
          </Button>
          {errors.actions ? (
            <p className="text-caption text-danger">{errors.actions}</p>
          ) : (
            <p className="text-caption text-fg-3">Group actions work on assigned groups only. Suspending also ends every session.</p>
          )}
        </fieldset>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Enabled</span>
            <span className="text-caption text-fg-3">Disabled rules don&apos;t run, and changes made while a rule is disabled aren&apos;t replayed later.</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

const runColumns: ColumnDef<LifecycleRun, any>[] = [
  {
    id: "time",
    accessorFn: (r) => new Date(r.createdAt).getTime(),
    header: "Time",
    meta: { label: "Time" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.createdAt)}>
        <RelativeTime iso={row.original.createdAt} />
      </span>
    ),
  },
  {
    id: "user",
    accessorFn: (r) => r.user.name,
    header: "Person",
    meta: { label: "Person", hideable: false },
    cell: ({ row }) => (
      <Link href={`/admin/users/${row.original.user.id}`} className="font-medium whitespace-nowrap text-fg hover:underline">
        {row.original.user.name}
      </Link>
    ),
  },
  {
    id: "rule",
    accessorFn: (r) => r.rule.name,
    header: "Rule",
    filterFn: facetFilter,
    meta: { label: "Rule" },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.rule.name}</span>,
  },
  {
    id: "trigger",
    accessorFn: (r) => r.trigger,
    header: "Trigger",
    filterFn: facetFilter,
    meta: { label: "Trigger" },
    cell: ({ row }) => <span className="whitespace-nowrap">{TRIGGER[row.original.trigger].label}</span>,
  },
  {
    id: "change",
    accessorFn: (r) => r.change,
    header: "Change",
    meta: { label: "Change" },
    cell: ({ getValue }) => <span className="block max-w-72 min-w-48 py-2">{getValue<string>()}</span>,
  },
  {
    id: "steps",
    accessorFn: (r) => r.steps.join("; "),
    header: "What Halo did",
    meta: { label: "What Halo did" },
    cell: ({ getValue }) => <span className="block max-w-80 min-w-48 py-2">{getValue<string>()}</span>,
  },
  {
    id: "result",
    accessorFn: (r) => r.result,
    header: "Result",
    filterFn: facetFilter,
    meta: { label: "Result" },
    cell: ({ row }) => (
      <Badge tone={row.original.result === "succeeded" ? "success" : "danger"}>{row.original.result === "succeeded" ? "Succeeded" : "Failed"}</Badge>
    ),
  },
];

export function RunsTable({ runs }: { runs: LifecycleRun[] }) {
  const rules = [...new Set(runs.map((r) => r.rule.name))].sort();
  return (
    <DataTable
      data={runs}
      columns={runColumns}
      getRowId={(r) => r.id}
      label="Lifecycle run history"
      noun={["run", "runs"]}
      storageKey="lifecycle-runs"
      searchText={(r) => `${r.user.name} ${r.rule.name} ${r.change} ${r.steps.join(" ")}`}
      searchPlaceholder="Search people, rules or changes"
      facets={[
        { column: "rule", label: "Rule", options: rules.map((name) => ({ value: name, label: name })) },
        { column: "trigger", label: "Trigger", options: (Object.keys(TRIGGER) as LifecycleTrigger[]).map((value) => ({ value, label: TRIGGER[value].label })) },
        {
          column: "result",
          label: "Result",
          options: [
            { value: "succeeded", label: "Succeeded" },
            { value: "failed", label: "Failed" },
          ],
        },
      ]}
    />
  );
}

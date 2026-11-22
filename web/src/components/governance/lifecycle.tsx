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
import { Spinner } from "@/components/ui/spinner";
import { ApiError, api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { LifecycleAction, LifecycleActionType, LifecycleRule, LifecycleRun, LifecycleTrigger, NamedRef } from "@/lib/governance-types";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import type { Locale } from "@/lib/i18n/locale";
import type { Attribute, Group } from "@/lib/types";
import { ACTIONS, TRIGGERS } from "./shared";

const groupAction = (type: LifecycleActionType) => type === "add-to-group" || type === "remove-from-group";

const scheduled = (trigger: LifecycleTrigger) => trigger === "date" || trigger === "inactivity";

function bodyFor(rule: LifecycleRule, enabled = rule.enabled) {
  return { name: rule.name, trigger: rule.trigger, condition: rule.condition, attribute: rule.attribute, days: rule.days, actions: rule.actions, enabled };
}

function describeTrigger(rule: Pick<LifecycleRule, "trigger" | "attribute" | "days">, attributes: Attribute[], locale: Locale) {
  const t = governanceMessages[locale];
  const days = rule.days ?? 0;
  if (rule.trigger === "inactivity") return t.lifecycle.noSignInFor(days);
  if (rule.trigger !== "date") return t.triggers[rule.trigger].label;
  const label = attributes.find((a) => a.name === rule.attribute)?.label ?? t.lifecycle.deletedAttribute;
  return days === 0 ? t.lifecycle.onDate(label) : days < 0 ? t.lifecycle.daysBefore(-days, label) : t.lifecycle.daysAfter(days, label);
}

function RuleToggle({ rule }: { rule: LifecycleRule }) {
  const { pending, run, toast } = useMutation();
  const t = useMessages(governanceMessages).lifecycle;
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 whitespace-nowrap" onClick={(event) => event.stopPropagation()}>
      {pending === "toggle" ? (
        <Spinner />
      ) : (
        <Checkbox
          checked={rule.enabled}
          aria-label={t.enabledLabel(rule.name)}
          onChange={(event) =>
            run(
              "toggle",
              () => api(`/lifecycle/rules/${rule.id}`, { method: "PUT", body: bodyFor(rule, event.target.checked) }),
              () =>
                toast(
                  rule.enabled
                    ? { title: t.disabledToast(rule.name), description: scheduled(rule.trigger) ? t.disabledScheduled : t.disabledEvent }
                    : { title: t.enabledToast(rule.name), description: scheduled(rule.trigger) ? t.checksWithinHour : t.runsOnNextChange },
                ),
            )
          }
        />
      )}
      {rule.enabled ? t.enabled : t.disabled}
    </label>
  );
}

export function RulesTable({ rules, groups, attributes }: { rules: LifecycleRule[]; groups: Group[]; attributes: Attribute[] }) {
  const [editing, setEditing] = useState<LifecycleRule | "new" | null>(null);
  const [deleting, setDeleting] = useState<LifecycleRule | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.lifecycle;
  const describe = useMemo(() => {
    const name = (id?: string) => groups.find((g) => g.id === id)?.name ?? copy.deletedGroup;
    return (a: LifecycleAction) =>
      a.type === "add-to-group" ? copy.addTo(name(a.groupId)) : a.type === "remove-from-group" ? copy.removeFrom(name(a.groupId)) : t.actions[a.type];
  }, [groups, t, copy]);

  const columns = useMemo<ColumnDef<LifecycleRule, any>[]>(
    () => [
      {
        id: "name",
        accessorFn: (r) => r.name,
        header: t.rule,
        meta: { label: t.rule, hideable: false },
        cell: ({ row }) => <span className="font-medium whitespace-nowrap text-fg">{row.original.name}</span>,
      },
      {
        id: "trigger",
        accessorFn: (r) => r.trigger,
        header: t.trigger,
        filterFn: facetFilter,
        meta: { label: t.trigger },
        cell: ({ row }) => <span className="whitespace-nowrap">{describeTrigger(row.original, attributes, locale)}</span>,
      },
      {
        id: "condition",
        accessorFn: (r) => r.condition ?? "",
        header: copy.condition,
        meta: { label: copy.condition },
        cell: ({ row }) => (row.original.condition ? <Tag>{row.original.condition}</Tag> : <span className="text-fg-3">{t.everyone}</span>),
      },
      {
        id: "actions",
        accessorFn: (r) => r.actions.map(describe).join("; "),
        header: copy.actions,
        meta: { label: copy.actions },
        cell: ({ getValue }) => <span className="block max-w-80 min-w-48 py-2">{getValue<string>()}</span>,
      },
      {
        id: "status",
        accessorFn: (r) => (r.enabled ? "enabled" : "disabled"),
        header: t.status,
        filterFn: facetFilter,
        meta: { label: t.status },
        cell: ({ row }) => <RuleToggle rule={row.original} />,
      },
      {
        id: "runs",
        accessorFn: (r) => r.runs,
        header: copy.runs,
        meta: { label: copy.runs, align: "right" },
        cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
      },
      {
        id: "lastRun",
        accessorFn: (r) => (r.lastRunAt ? new Date(r.lastRunAt).getTime() : 0),
        header: copy.lastRun,
        meta: { label: copy.lastRun, align: "right" },
        cell: ({ row }) => (
          <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastRunAt ? <RelativeTime iso={row.original.lastRunAt} /> : copy.never}</span>
        ),
      },
    ],
    [describe, attributes, locale, t, copy],
  );

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {copy.create}
    </Button>
  );

  return (
    <>
      {rules.length === 0 ? (
        <Card>
          <EmptyState
            icon={Workflow}
            title={copy.emptyTitle}
            description={copy.emptyDescription}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={rules}
          columns={columns}
          getRowId={(r) => r.id}
          label={copy.label}
          noun={copy.noun}
          storageKey="lifecycle-rules"
          searchText={(r) => `${r.name} ${describeTrigger(r, attributes, locale)} ${r.condition ?? ""} ${r.actions.map(describe).join(" ")}`}
          searchPlaceholder={copy.search}
          facets={[
            {
              column: "trigger",
              label: t.trigger,
              options: TRIGGERS.map((value) => ({ value, label: t.triggers[value].label })),
            },
            {
              column: "status",
              label: t.status,
              options: [
                { value: "enabled", label: copy.enabled },
                { value: "disabled", label: copy.disabled },
              ],
            },
          ]}
          onRowOpen={setEditing}
          rowActions={[
            { label: copy.edit, icon: Pencil, onSelect: setEditing },
            { label: copy.delete, icon: Trash2, danger: true, separatorBefore: true, onSelect: setDeleting },
          ]}
          toolbarEnd={create}
        />
      )}
      {editing ? (
        <RuleDialog
          rule={editing === "new" ? null : editing}
          groups={groups}
          attributes={attributes.filter((a) => !a.selfService)}
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
        title={t.deleteTitle(deleting?.name ?? "")}
        description={deleting?.runs ? copy.deleteWithRuns(deleting.runs) : copy.deleteWithoutRuns}
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
                  () => api(`/lifecycle/rules/${deleting.id}`, { method: "DELETE" }),
                  () => {
                    toast({ title: t.deleted(deleting.name), description: t.recorded });
                    setDeleting(null);
                  },
                  "inline",
                )
              }
            >
              {copy.delete}
            </Button>
          </>
        }
      />
    </>
  );
}

function RuleDialog({ rule, groups, attributes, onClose }: { rule: LifecycleRule | null; groups: Group[]; attributes: Attribute[]; onClose: () => void }) {
  const [name, setName] = useState(rule?.name ?? "");
  const [trigger, setTrigger] = useState<LifecycleTrigger>(rule?.trigger ?? "joiner");
  const [condition, setCondition] = useState(rule?.condition ?? "");
  const [attribute, setAttribute] = useState(rule?.attribute ?? attributes[0]?.name ?? "");
  const [days, setDays] = useState(rule?.days != null ? String(Math.abs(rule.days)) : "");
  const [direction, setDirection] = useState<"before" | "after">((rule?.days ?? 0) < 0 ? "before" : "after");
  const [actions, setActions] = useState<LifecycleAction[]>(rule?.actions ?? [{ type: "add-to-group" }]);
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [errors, setErrors] = useState<{ name?: string; days?: string; actions?: string }>({});
  const [preview, setPreview] = useState<{ key: string; people: { user: NamedRef; change: string }[] } | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [previewError, setPreviewError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.lifecycle;
  const isScheduled = scheduled(trigger);
  const count = Number(days);
  const body = {
    name: name.trim(),
    trigger,
    condition: condition.trim() || null,
    attribute: trigger === "date" ? attribute : null,
    days: trigger === "date" ? (direction === "before" ? -count : count) : trigger === "inactivity" ? count : null,
    actions: actions.map((a) => (groupAction(a.type) ? a : { type: a.type })),
    enabled,
  };
  const key = JSON.stringify(body);
  const shown = preview?.key === key ? preview.people : null;

  function update(index: number, next: LifecycleAction) {
    setActions((current) => current.map((a, i) => (i === index ? next : a)));
  }

  function validate() {
    const next = {
      name: name.trim() ? undefined : copy.nameRequired,
      days: isScheduled && (days.trim() === "" || !Number.isInteger(count) || count < 0) ? copy.daysRequired : undefined,
      actions: actions.some((a) => groupAction(a.type) && !a.groupId) ? copy.groupRequired : undefined,
    };
    setErrors(next);
    return !next.name && !next.days && !next.actions;
  }

  async function previewRule() {
    if (!validate()) return;
    setPreviewing(true);
    setPreviewError(undefined);
    try {
      const people = await api<{ user: NamedRef; change: string }[]>(`/lifecycle/preview${rule ? `?rule=${rule.id}` : ""}`, { body });
      setPreview({ key, people });
    } catch (error) {
      setPreview(null);
      setPreviewError(error instanceof ApiError ? error.message : frameMessages[locale].mutation.unreachable);
    } finally {
      setPreviewing(false);
    }
  }

  function submit() {
    if (!validate()) return;
    void run(
      "save",
      () => api<LifecycleRule>(rule ? `/lifecycle/rules/${rule.id}` : "/lifecycle/rules", { method: rule ? "PUT" : "POST", body }),
      (saved) => {
        onClose();
        toast({
          title: rule ? t.saved(saved.name) : t.created(saved.name),
          description: isScheduled ? copy.checksWithinHour : copy.appliesFromNow,
        });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      title={rule ? t.editTitle(rule.name) : copy.createTitle}
      description={copy.dialogDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {rule ? copy.save : copy.create}
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
        <Field label={t.name} error={errors.name}>
          {(props) => (
            <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={copy.namePlaceholder} />
          )}
        </Field>
        <Field label={t.trigger} hint={t.triggers[trigger].description}>
          {(props) => (
            <Select {...props} value={trigger} onChange={(event) => setTrigger(event.target.value as LifecycleTrigger)}>
              {TRIGGERS.map((value) => (
                <option key={value} value={value}>
                  {t.triggers[value].label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {trigger === "date" ? (
          <>
            <Field
              label={copy.dateAttribute}
              hint={
                attributes.length ? (
                  copy.dateAttributeHint
                ) : (
                  <>
                    {copy.createAttribute}{" "}
                    <Link href="/admin/settings/attributes" className="text-fg-2 underline">
                      {frameMessages[locale].nav.items.attributes.label}
                    </Link>
                    .
                  </>
                )
              }
            >
              {(props) => (
                <Select {...props} value={attribute} disabled={!attributes.length} onChange={(event) => setAttribute(event.target.value)}>
                  {attributes.length ? null : <option value="">{copy.noDateAttributes}</option>}
                  {attributes.map((a) => (
                    <option key={a.name} value={a.name}>
                      {a.label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label={copy.when} hint={copy.whenHint} error={errors.days}>
              {(props) => (
                <div className="flex gap-2">
                  <Input {...props} type="number" min={0} max={3650} step={1} inputMode="numeric" className="tnum" value={days} onChange={(event) => setDays(event.target.value)} placeholder="7" />
                  <div className="w-40 shrink-0">
                    <Select aria-label={copy.beforeOrAfter} value={direction} onChange={(event) => setDirection(event.target.value as "before" | "after")}>
                      <option value="before">{copy.before}</option>
                      <option value="after">{copy.after}</option>
                    </Select>
                  </div>
                </div>
              )}
            </Field>
          </>
        ) : null}
        {trigger === "inactivity" ? (
          <Field
            label={copy.inactiveDays}
            hint={copy.inactiveDaysHint}
            error={errors.days}
          >
            {(props) => (
              <Input {...props} type="number" min={1} max={3650} step={1} inputMode="numeric" className="tnum" value={days} onChange={(event) => setDays(event.target.value)} placeholder="90" />
            )}
          </Field>
        ) : null}
        <Field
          label={copy.condition}
          hint={
            <>
              {copy.conditionHint[0]}{" "}
              <code className="text-code-sm text-fg-2">user.department == &quot;Engineering&quot;</code>
              {copy.conditionHint[1]}
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
          <legend className="mb-3 text-label text-fg">{copy.actions}</legend>
          {actions.map((action, index) => (
            <div key={index} className="flex items-center gap-2">
              <Select
                aria-label={copy.action(index + 1)}
                value={action.type}
                onChange={(event) => update(index, { type: event.target.value as LifecycleActionType, groupId: action.groupId })}
              >
                {ACTIONS.map((value) => (
                  <option key={value} value={value}>
                    {t.actions[value]}
                  </option>
                ))}
              </Select>
              {groupAction(action.type) ? (
                <Select
                  aria-label={copy.groupForAction(index + 1)}
                  value={action.groupId ?? ""}
                  onChange={(event) => update(index, { type: action.type, groupId: event.target.value })}
                >
                  <option value="" disabled>
                    {t.chooseGroup}
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
                aria-label={copy.removeAction(index + 1)}
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
            {copy.addAction}
          </Button>
          {errors.actions ? (
            <p className="text-caption text-danger">{errors.actions}</p>
          ) : (
            <p className="text-caption text-fg-3">{copy.actionsHint}</p>
          )}
        </fieldset>
        {isScheduled ? (
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-3">
              <Button size="sm" variant="secondary" loading={previewing} onClick={previewRule}>
                {copy.preview}
              </Button>
              {shown ? (
                <p className="animate-enter text-body-sm text-fg-2" aria-live="polite">
                  {shown.length === 0
                    ? copy.nobody
                    : copy.previewPeople(
                        shown.length,
                        shown
                          .slice(0, 5)
                          .map((p) => p.user.name)
                          .join(", "),
                        shown.length - 5,
                      )}
                </p>
              ) : null}
            </div>
            {previewError ? <p className="text-caption text-danger">{previewError}</p> : null}
            <p className="text-caption text-fg-3">{copy.previewHint}</p>
          </div>
        ) : null}
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{copy.enabled}</span>
            <span className="text-caption text-fg-3">{isScheduled ? copy.enabledScheduledHint : copy.enabledEventHint}</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function runColumns(locale: Locale): ColumnDef<LifecycleRun, any>[] {
  const t = governanceMessages[locale];
  const copy = t.lifecycle;
  return [
    {
      id: "time",
      accessorFn: (r) => new Date(r.createdAt).getTime(),
      header: copy.time,
      meta: { label: copy.time },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.createdAt, locale)}>
          <RelativeTime iso={row.original.createdAt} />
        </span>
      ),
    },
    {
      id: "user",
      accessorFn: (r) => r.user.name,
      header: copy.person,
      meta: { label: copy.person, hideable: false },
      cell: ({ row }) => (
        <Link href={`/admin/users/${row.original.user.id}`} className="font-medium whitespace-nowrap text-fg hover:underline">
          {row.original.user.name}
        </Link>
      ),
    },
    {
      id: "rule",
      accessorFn: (r) => r.rule.name,
      header: t.rule,
      filterFn: facetFilter,
      meta: { label: t.rule },
      cell: ({ row }) => <span className="whitespace-nowrap">{row.original.rule.name}</span>,
    },
    {
      id: "trigger",
      accessorFn: (r) => r.trigger,
      header: t.trigger,
      filterFn: facetFilter,
      meta: { label: t.trigger },
      cell: ({ row }) => <span className="whitespace-nowrap">{t.triggers[row.original.trigger].label}</span>,
    },
    {
      id: "change",
      accessorFn: (r) => r.change,
      header: copy.change,
      meta: { label: copy.change },
      cell: ({ getValue }) => <span className="block max-w-72 min-w-48 py-2">{getValue<string>()}</span>,
    },
    {
      id: "steps",
      accessorFn: (r) => r.steps.join("; "),
      header: copy.steps,
      meta: { label: copy.steps },
      cell: ({ getValue }) => <span className="block max-w-80 min-w-48 py-2">{getValue<string>()}</span>,
    },
    {
      id: "result",
      accessorFn: (r) => r.result,
      header: t.result,
      filterFn: facetFilter,
      meta: { label: t.result },
      cell: ({ row }) => (
        <Badge tone={row.original.result === "succeeded" ? "success" : "danger"}>{row.original.result === "succeeded" ? copy.succeeded : copy.failed}</Badge>
      ),
    },
  ];
}

export function RunsTable({ runs }: { runs: LifecycleRun[] }) {
  const locale = useLocale();
  const t = governanceMessages[locale];
  const columns = useMemo(() => runColumns(locale), [locale]);
  const rules = [...new Set(runs.map((r) => r.rule.name))].sort();
  return (
    <DataTable
      data={runs}
      columns={columns}
      getRowId={(r) => r.id}
      label={t.lifecycle.history}
      noun={t.lifecycle.runNoun}
      storageKey="lifecycle-runs"
      searchText={(r) => `${r.user.name} ${r.rule.name} ${r.change} ${r.steps.join(" ")}`}
      searchPlaceholder={t.lifecycle.runSearch}
      facets={[
        { column: "rule", label: t.rule, options: rules.map((name) => ({ value: name, label: name })) },
        { column: "trigger", label: t.trigger, options: TRIGGERS.map((value) => ({ value, label: t.triggers[value].label })) },
        {
          column: "result",
          label: t.result,
          options: [
            { value: "succeeded", label: t.lifecycle.succeeded },
            { value: "failed", label: t.lifecycle.failed },
          ],
        },
      ]}
    />
  );
}

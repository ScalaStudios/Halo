"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Archive, Package, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { AccessPackage, AccessQuestion, StageApprover } from "@/lib/governance-types";
import { useLocale } from "@/lib/i18n/client";
import { governanceMessages } from "@/lib/i18n/console/governance";
import type { Locale } from "@/lib/i18n/locale";
import type { Group } from "@/lib/types";
import { PeoplePicker } from "./people-picker";
import { DURATIONS, durationLabel, joinNames, stagesLabel, type Person } from "./shared";

const DEADLINES = [1, 2, 3, 7, 14, 30];

type StageDraft = { key: number; approver: StageApprover; approverIds: string[]; groupId: string; fallbackIds: string[]; timeout: string };
type QuestionDraft = { key: number; id: string; text: string; kind: AccessQuestion["kind"]; choices: string; required: boolean };

let draftKey = 0;

function approvalLabel(p: AccessPackage, locale: Locale): string {
  const label = stagesLabel(p.stages, locale);
  return label.charAt(0).toUpperCase() + label.slice(1);
}

const packageColumns = (t: typeof governanceMessages.en, locale: Locale): ColumnDef<AccessPackage, any>[] => [
  {
    id: "name",
    accessorFn: (p) => p.name,
    header: t.package,
    meta: { label: t.package, hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-80 min-w-56 flex-col py-2">
        <span className="font-medium text-fg">{row.original.name}</span>
        <span className="text-caption text-fg-3">{row.original.description}</span>
      </span>
    ),
  },
  {
    id: "groups",
    accessorFn: (p) => joinNames(p.groups, locale),
    header: t.packages.grants,
    meta: { label: t.packages.grants },
    cell: ({ getValue }) => <span className="whitespace-nowrap">{getValue<string>()}</span>,
  },
  {
    id: "approvers",
    accessorFn: (p) => approvalLabel(p, locale),
    header: t.packages.approval,
    meta: { label: t.packages.approval },
    cell: ({ getValue }) => <span className="block max-w-72 min-w-40">{getValue<string>()}</span>,
  },
  {
    id: "duration",
    accessorFn: (p) => p.maxDays ?? Number.MAX_SAFE_INTEGER,
    header: t.packages.maximum,
    meta: { label: t.packages.maximumDuration, align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap">{durationLabel(row.original.maxDays, locale)}</span>,
  },
  {
    id: "pending",
    accessorFn: (p) => p.pendingRequests,
    header: t.pending,
    meta: { label: t.packages.pendingRequests, align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "active",
    accessorFn: (p) => p.activeGrants,
    header: t.packages.active,
    meta: { label: t.packages.activeGrants, align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "status",
    accessorFn: (p) => (p.archived ? "archived" : "requestable"),
    header: t.status,
    filterFn: facetFilter,
    meta: { label: t.status },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={row.original.archived ? "neutral" : "success"} />
        {row.original.archived ? t.packages.archived : t.packages.requestable}
      </span>
    ),
  },
];

function bodyFor(p: AccessPackage) {
  return {
    name: p.name,
    description: p.description,
    ownerId: p.owner?.id ?? null,
    groupIds: p.groups.map((g) => g.id),
    maxDays: p.maxDays,
    requireJustification: p.requireJustification,
    archived: p.archived,
  };
}

export function PackagesTable({ packages, groups, people }: { packages: AccessPackage[]; groups: Group[]; people: Person[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<AccessPackage | "new" | null>(null);
  const [deleting, setDeleting] = useState<AccessPackage | null>(null);
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.packages;
  const columns = useMemo(() => packageColumns(t, locale), [t, locale]);
  const views: SavedView[] = [
    { id: "requestable", label: copy.requestable, filters: [{ id: "status", value: ["requestable"] }] },
    { id: "archived", label: copy.archived, filters: [{ id: "status", value: ["archived"] }] },
    { id: "all", label: copy.all, filters: [] },
  ];

  function setArchived(p: AccessPackage, archived: boolean) {
    void run(
      "archive",
      () => api(`/access-packages/${p.id}`, { method: "PUT", body: { ...bodyFor(p), archived } }),
      () =>
        toast(
          archived
            ? { title: copy.archivedToast(p.name), description: copy.archivedHint }
            : { title: copy.restored(p.name), description: copy.restoredHint },
        ),
    );
  }

  const create = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {copy.create}
    </Button>
  );

  return (
    <>
      {packages.length === 0 ? (
        <Card>
          <EmptyState
            icon={Package}
            title={copy.emptyTitle}
            description={copy.emptyDescription}
            action={create}
          />
        </Card>
      ) : (
        <DataTable
          data={packages}
          columns={columns}
          getRowId={(p) => p.id}
          label={copy.label}
          noun={copy.noun}
          storageKey="access-packages"
          searchText={(p) => `${p.name} ${p.description} ${joinNames(p.groups, locale)} ${approvalLabel(p, locale)}`}
          searchPlaceholder={copy.search}
          views={views}
          facets={[
            {
              column: "status",
              label: t.status,
              options: [
                { value: "requestable", label: copy.requestable },
                { value: "archived", label: copy.archived },
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
      {editing ? <PackageDialog pkg={editing === "new" ? null : editing} groups={groups} people={people} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.deleteTitle(deleting?.name ?? "")}
        description={copy.deleteDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.cancel}
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
                {copy.archiveInstead}
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

function PackageDialog({ pkg, groups, people, onClose }: { pkg: AccessPackage | null; groups: Group[]; people: Person[]; onClose: () => void }) {
  const [name, setName] = useState(pkg?.name ?? "");
  const [description, setDescription] = useState(pkg?.description ?? "");
  const [groupIds, setGroupIds] = useState<string[]>(pkg?.groups.map((g) => g.id) ?? []);
  const [stages, setStages] = useState<StageDraft[]>(() =>
    (pkg?.stages ?? [{ approver: "people" as const, approvers: [], group: null, fallback: [], timeoutDays: null }]).map((stage) => ({
      key: ++draftKey,
      approver: stage.approver,
      approverIds: stage.approvers.map((a) => a.id),
      groupId: stage.group?.id ?? "",
      fallbackIds: stage.fallback.map((f) => f.id),
      timeout: stage.timeoutDays === null ? "none" : String(stage.timeoutDays),
    })),
  );
  const [questions, setQuestions] = useState<QuestionDraft[]>(() =>
    (pkg?.questions ?? []).map((q) => ({ key: ++draftKey, id: q.id, text: q.text, kind: q.kind, choices: q.choices.join("\n"), required: q.required })),
  );
  const [ownerId, setOwnerId] = useState(pkg?.owner?.id ?? "");
  const [maxDays, setMaxDays] = useState(pkg ? (pkg.maxDays === null ? "none" : String(pkg.maxDays)) : "7");
  const [requireJustification, setRequireJustification] = useState(pkg?.requireJustification ?? true);
  const [archived, setArchived] = useState(pkg?.archived ?? false);
  const [errors, setErrors] = useState<{ name?: string; groups?: string }>({});
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.packages;
  const durations = [...new Set([...DURATIONS, ...(pkg?.maxDays ? [pkg.maxDays] : [])])].sort((a, b) => a - b);

  function updateStage(key: number, change: Partial<StageDraft>) {
    setStages((current) => current.map((stage) => (stage.key === key ? { ...stage, ...change } : stage)));
  }

  function updateQuestion(key: number, change: Partial<QuestionDraft>) {
    setQuestions((current) => current.map((q) => (q.key === key ? { ...q, ...change } : q)));
  }

  function submit() {
    const next = { name: name.trim() ? undefined : copy.nameRequired, groups: groupIds.length ? undefined : copy.groupsRequired };
    setErrors(next);
    if (next.name || next.groups) return;
    const body = {
      name: name.trim(),
      description: description.trim(),
      ownerId: ownerId || null,
      groupIds,
      stages: stages.map((stage) => ({
        approver: stage.approver,
        approverIds: stage.approver === "people" ? stage.approverIds : [],
        groupId: stage.approver === "group-owners" ? stage.groupId || null : null,
        fallbackIds: stage.fallbackIds,
        timeoutDays: stage.timeout === "none" ? null : Number(stage.timeout),
      })),
      questions: questions.map((q) => ({
        id: q.id,
        text: q.text.trim(),
        kind: q.kind,
        choices: q.kind === "choice" ? q.choices.split("\n").map((c) => c.trim()).filter(Boolean) : [],
        required: q.required,
      })),
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
            ? { title: t.saved(saved.name), description: copy.savedHint }
            : { title: t.created(saved.name), description: copy.createdHint },
        );
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("save")}
      width="wide"
      title={pkg ? t.editTitle(pkg.name) : copy.createTitle}
      description={copy.dialogDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {pkg ? copy.save : copy.create}
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
        <Field label={copy.description} hint={copy.descriptionHint}>
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">{copy.groups}</legend>
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
            <p className="text-caption text-fg-3">{copy.groupsHint}</p>
          )}
        </fieldset>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">{copy.approval}</legend>
          <ol className="flex flex-col gap-3">
            {stages.map((stage, index) => (
              <li key={stage.key} className="flex flex-col gap-4 rounded-md border border-border p-4">
                <div className="flex min-h-8 items-center justify-between gap-3">
                  <span className="text-body-sm font-semibold text-fg">{copy.stage(index + 1)}</span>
                  {stages.length > 1 ? (
                    <Button size="sm" variant="quiet" aria-label={copy.removeStage(index + 1)} onClick={() => setStages((current) => current.filter((s) => s.key !== stage.key))}>
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                      {t.remove}
                    </Button>
                  ) : null}
                </div>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field label={copy.decidedBy}>
                    {(props) => (
                      <Select {...props} value={stage.approver} onChange={(event) => updateStage(stage.key, { approver: event.target.value as StageApprover })}>
                        <option value="people">{copy.specificPeople}</option>
                        <option value="manager">{copy.requesterManager}</option>
                        <option value="group-owners">{copy.groupOwners}</option>
                      </Select>
                    )}
                  </Field>
                  <Field label={copy.deadline} hint={copy.deadlineHint}>
                    {(props) => (
                      <Select {...props} value={stage.timeout} onChange={(event) => updateStage(stage.key, { timeout: event.target.value })}>
                        <option value="none">{copy.noDeadline}</option>
                        {DEADLINES.map((days) => (
                          <option key={days} value={String(days)}>
                            {durationLabel(days, locale)}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                </div>
                {stage.approver === "people" ? (
                  <Field label={copy.approvers} hint={copy.approversHint}>
                    {(props) => <PeoplePicker {...props} people={people} value={stage.approverIds} onChange={(approverIds) => updateStage(stage.key, { approverIds })} />}
                  </Field>
                ) : null}
                {stage.approver === "group-owners" ? (
                  <Field label={t.group} hint={copy.groupHint}>
                    {(props) => (
                      <Select {...props} value={stage.groupId} onChange={(event) => updateStage(stage.key, { groupId: event.target.value })}>
                        <option value="">{t.chooseGroup}</option>
                        {groups.map((group) => (
                          <option key={group.id} value={group.id}>
                            {group.name}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                ) : null}
                <Field
                  label={copy.fallback}
                  hint={copy.fallbackHint}
                >
                  {(props) => <PeoplePicker {...props} people={people} value={stage.fallbackIds} onChange={(fallbackIds) => updateStage(stage.key, { fallbackIds })} />}
                </Field>
              </li>
            ))}
          </ol>
          {stages.length < 3 ? (
            <Button
              size="sm"
              variant="secondary"
              className="self-start"
              onClick={() => setStages((current) => [...current, { key: ++draftKey, approver: "people", approverIds: [], groupId: "", fallbackIds: [], timeout: "none" }])}
            >
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {copy.addStage}
            </Button>
          ) : null}
          <p className="text-caption text-fg-3">{copy.stagesHint}</p>
        </fieldset>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">{copy.questions}</legend>
          {questions.length ? (
            <ol className="flex flex-col gap-3">
              {questions.map((q, index) => (
                <li key={q.key} className="flex flex-col gap-4 rounded-md border border-border p-4">
                  <div className="flex min-h-8 items-center justify-between gap-3">
                    <span className="text-body-sm font-semibold text-fg">{copy.questionNumber(index + 1)}</span>
                    <Button size="sm" variant="quiet" aria-label={copy.removeQuestion(index + 1)} onClick={() => setQuestions((current) => current.filter((c) => c.key !== q.key))}>
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                      {t.remove}
                    </Button>
                  </div>
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_12rem]">
                    <Field label={copy.question}>
                      {(props) => <Input {...props} autoComplete="off" value={q.text} onChange={(event) => updateQuestion(q.key, { text: event.target.value })} placeholder={copy.questionPlaceholder} />}
                    </Field>
                    <Field label={copy.answer}>
                      {(props) => (
                        <Select {...props} value={q.kind} onChange={(event) => updateQuestion(q.key, { kind: event.target.value as AccessQuestion["kind"] })}>
                          <option value="text">{copy.freeText}</option>
                          <option value="choice">{copy.choice}</option>
                        </Select>
                      )}
                    </Field>
                  </div>
                  {q.kind === "choice" ? (
                    <Field label={copy.choices} hint={copy.choicesHint}>
                      {(props) => <Textarea {...props} rows={3} value={q.choices} onChange={(event) => updateQuestion(q.key, { choices: event.target.value })} />}
                    </Field>
                  ) : null}
                  <label className="flex cursor-pointer items-start gap-3">
                    <Checkbox className="mt-0.5" checked={q.required} onChange={(event) => updateQuestion(q.key, { required: event.target.checked })} />
                    <span className="text-body-sm text-fg">{copy.required}</span>
                  </label>
                </li>
              ))}
            </ol>
          ) : null}
          {questions.length < 10 ? (
            <Button
              size="sm"
              variant="secondary"
              className="self-start"
              onClick={() => setQuestions((current) => [...current, { key: ++draftKey, id: "", text: "", kind: "text", choices: "", required: true }])}
            >
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {copy.addQuestion}
            </Button>
          ) : null}
          <p className="text-caption text-fg-3">{copy.questionsHint}</p>
        </fieldset>
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label={copy.owner} hint={copy.ownerHint}>
            {(props) => (
              <Select {...props} value={ownerId} onChange={(event) => setOwnerId(event.target.value)}>
                <option value="">{copy.noOwner}</option>
                {people.map((person) => (
                  <option key={person.id} value={person.id}>
                    {person.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={copy.maximumDuration} hint={copy.maximumHint}>
            {(props) => (
              <Select {...props} value={maxDays} onChange={(event) => setMaxDays(event.target.value)}>
                {durations.map((days) => (
                  <option key={days} value={String(days)}>
                    {durationLabel(days, locale)}
                  </option>
                ))}
                <option value="none">{t.untilRevoked}</option>
              </Select>
            )}
          </Field>
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={requireJustification} onChange={(event) => setRequireJustification(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{copy.requireJustification}</span>
            <span className="text-caption text-fg-3">{copy.requireJustificationHint}</span>
          </span>
        </label>
        {pkg ? (
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={archived} onChange={(event) => setArchived(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{copy.archived}</span>
              <span className="text-caption text-fg-3">{copy.archiveCheckboxHint}</span>
            </span>
          </label>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

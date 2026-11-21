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
import type { AccessPackage, AccessQuestion, StageApprover } from "@/lib/governance-types";
import type { Group } from "@/lib/types";
import { PeoplePicker } from "./people-picker";
import { DURATIONS, durationLabel, joinNames, stagesLabel, type Person } from "./shared";

const DEADLINES = [1, 2, 3, 7, 14, 30];

type StageDraft = { key: number; approver: StageApprover; approverIds: string[]; groupId: string; fallbackIds: string[]; timeout: string };
type QuestionDraft = { key: number; id: string; text: string; kind: AccessQuestion["kind"]; choices: string; required: boolean };

let draftKey = 0;

function approvalLabel(p: AccessPackage): string {
  const label = stagesLabel(p.stages);
  return label.charAt(0).toUpperCase() + label.slice(1);
}

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
    accessorFn: approvalLabel,
    header: "Approval",
    meta: { label: "Approval" },
    cell: ({ getValue }) => <span className="block max-w-72 min-w-40">{getValue<string>()}</span>,
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
    maxDays: p.maxDays,
    requireJustification: p.requireJustification,
    archived: p.archived,
  };
}

export function PackagesTable({ packages, groups, people }: { packages: AccessPackage[]; groups: Group[]; people: Person[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
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
          searchText={(p) => `${p.name} ${p.description} ${joinNames(p.groups)} ${approvalLabel(p)}`}
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
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
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
                  "inline",
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
  const durations = [...new Set([...DURATIONS, ...(pkg?.maxDays ? [pkg.maxDays] : [])])].sort((a, b) => a - b);

  function updateStage(key: number, change: Partial<StageDraft>) {
    setStages((current) => current.map((stage) => (stage.key === key ? { ...stage, ...change } : stage)));
  }

  function updateQuestion(key: number, change: Partial<QuestionDraft>) {
    setQuestions((current) => current.map((q) => (q.key === key ? { ...q, ...change } : q)));
  }

  function submit() {
    const next = { name: name.trim() ? undefined : "Enter a package name.", groups: groupIds.length ? undefined : "Choose at least one group to grant." };
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
            ? { title: `${saved.name} saved`, description: "Pending requests follow the new stages and deadlines. Access already granted keeps its end date." }
            : { title: `${saved.name} created`, description: "People can request it from the Access page in their account." },
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
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">Approval</legend>
          <ol className="flex flex-col gap-3">
            {stages.map((stage, index) => (
              <li key={stage.key} className="flex flex-col gap-4 rounded-md border border-border p-4">
                <div className="flex min-h-8 items-center justify-between gap-3">
                  <span className="text-body-sm font-semibold text-fg">Stage {index + 1}</span>
                  {stages.length > 1 ? (
                    <Button size="sm" variant="quiet" aria-label={`Remove stage ${index + 1}`} onClick={() => setStages((current) => current.filter((s) => s.key !== stage.key))}>
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                      Remove
                    </Button>
                  ) : null}
                </div>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field label="Decided by">
                    {(props) => (
                      <Select {...props} value={stage.approver} onChange={(event) => updateStage(stage.key, { approver: event.target.value as StageApprover })}>
                        <option value="people">Specific people</option>
                        <option value="manager">The requester&apos;s manager</option>
                        <option value="group-owners">Owners of a group</option>
                      </Select>
                    )}
                  </Field>
                  <Field label="Deadline" hint="Then the fallback approvers get it. Without a decision after that, the request expires.">
                    {(props) => (
                      <Select {...props} value={stage.timeout} onChange={(event) => updateStage(stage.key, { timeout: event.target.value })}>
                        <option value="none">No deadline</option>
                        {DEADLINES.map((days) => (
                          <option key={days} value={String(days)}>
                            {durationLabel(days)}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                </div>
                {stage.approver === "people" ? (
                  <Field label="Approvers" hint="Any one of them decides this stage. Nobody decides a request they made or that is for them.">
                    {(props) => <PeoplePicker {...props} people={people} value={stage.approverIds} onChange={(approverIds) => updateStage(stage.key, { approverIds })} />}
                  </Field>
                ) : null}
                {stage.approver === "group-owners" ? (
                  <Field label="Group" hint="Its owners decide this stage.">
                    {(props) => (
                      <Select {...props} value={stage.groupId} onChange={(event) => updateStage(stage.key, { groupId: event.target.value })}>
                        <option value="">Choose a group</option>
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
                  label="Fallback approvers"
                  hint="Decide when nobody above can, such as a requester without a manager, and when the deadline passes. Without them, global administrators decide."
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
              Add stage
            </Button>
          ) : null}
          <p className="text-caption text-fg-3">Requests go through the stages in order. A denial at any stage ends the request. Global administrators can decide any stage.</p>
        </fieldset>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-3 text-label text-fg">Questions</legend>
          {questions.length ? (
            <ol className="flex flex-col gap-3">
              {questions.map((q, index) => (
                <li key={q.key} className="flex flex-col gap-4 rounded-md border border-border p-4">
                  <div className="flex min-h-8 items-center justify-between gap-3">
                    <span className="text-body-sm font-semibold text-fg">Question {index + 1}</span>
                    <Button size="sm" variant="quiet" aria-label={`Remove question ${index + 1}`} onClick={() => setQuestions((current) => current.filter((c) => c.key !== q.key))}>
                      <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                      Remove
                    </Button>
                  </div>
                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_12rem]">
                    <Field label="Question">
                      {(props) => <Input {...props} autoComplete="off" value={q.text} onChange={(event) => updateQuestion(q.key, { text: event.target.value })} placeholder="Ticket number" />}
                    </Field>
                    <Field label="Answer">
                      {(props) => (
                        <Select {...props} value={q.kind} onChange={(event) => updateQuestion(q.key, { kind: event.target.value as AccessQuestion["kind"] })}>
                          <option value="text">Free text</option>
                          <option value="choice">Choice from a list</option>
                        </Select>
                      )}
                    </Field>
                  </div>
                  {q.kind === "choice" ? (
                    <Field label="Choices" hint="One per line, at least two.">
                      {(props) => <Textarea {...props} rows={3} value={q.choices} onChange={(event) => updateQuestion(q.key, { choices: event.target.value })} />}
                    </Field>
                  ) : null}
                  <label className="flex cursor-pointer items-start gap-3">
                    <Checkbox className="mt-0.5" checked={q.required} onChange={(event) => updateQuestion(q.key, { required: event.target.checked })} />
                    <span className="text-body-sm text-fg">Required</span>
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
              Add question
            </Button>
          ) : null}
          <p className="text-caption text-fg-3">Asked on the request form, such as a ticket number or a project. Approvers see the answers, and Halo keeps them with the request.</p>
        </fieldset>
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

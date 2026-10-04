"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Check, MessageSquare, Plus, X } from "lucide-react";
import { useMemo, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDate, pluralize } from "@/lib/format";
import type { AccessReview, ReviewDecision, ReviewItem } from "@/lib/governance-types";
import type { Group } from "@/lib/types";
import { PeoplePicker } from "./people-picker";
import { OUTCOME, ReviewProgress, type Person } from "./shared";

function isoDay(offsetDays: number) {
  return new Date(Date.now() + offsetDays * 86_400_000).toISOString().slice(0, 10);
}

export function StartReview({ groups, people }: { groups: Group[]; people: Person[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Start review
      </Button>
      {open ? <StartReviewDialog groups={groups} people={people} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function StartReviewDialog({ groups, people, onClose }: { groups: Group[]; people: Person[]; onClose: () => void }) {
  const [name, setName] = useState("");
  const [groupId, setGroupId] = useState(groups.find((g) => g.memberCount > 0)?.id ?? "");
  const [reviewerIds, setReviewerIds] = useState<string[]>([]);
  const [due, setDue] = useState(isoDay(14));
  const [autoApply, setAutoApply] = useState(true);
  const [errors, setErrors] = useState<{ name?: string; group?: string; reviewers?: string; due?: string }>({});
  const { pending, run, toast, router } = useMutation();

  function submit() {
    const next = {
      name: name.trim() ? undefined : "Enter a review name.",
      group: groupId ? undefined : "Choose a group with members.",
      reviewers: reviewerIds.length ? undefined : "Choose at least one reviewer.",
      due: due >= isoDay(1) ? undefined : "Choose a due date after today.",
    };
    setErrors(next);
    if (next.name || next.group || next.reviewers || next.due) return;
    void run(
      "start",
      () =>
        api<AccessReview>("/access-reviews", {
          body: { name: name.trim(), groupId, reviewerIds, dueAt: new Date(`${due}T23:59:59Z`).toISOString(), autoApply },
        }),
      (review) => {
        onClose();
        toast({ title: `${review.name} started`, description: `${pluralize(review.total, "member")} to review. Reviewers see it on their Access page.` });
        router.push(`/admin/access-reviews/${review.id}`);
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title="Start an access review"
      description="Reviewers confirm or remove each current member of the group."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "start"} onClick={submit}>
            Start review
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
            <Input
              {...props}
              autoFocus
              autoComplete="off"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Production access · Q4 2026"
            />
          )}
        </Field>
        <Field
          label="Group"
          error={errors.group}
          hint="Assigned groups only. Rule-based groups can't be reviewed because their members come from a rule; change the rule instead."
        >
          {(props) => (
            <Select {...props} value={groupId} onChange={(event) => setGroupId(event.target.value)}>
              {groups.map((group) => (
                <option key={group.id} value={group.id} disabled={group.memberCount === 0}>
                  {group.name} · {pluralize(group.memberCount, "member")}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Reviewers" error={errors.reviewers} hint="Reviewers don't need an administrator role. They decide from their Access page.">
          {(props) => <PeoplePicker {...props} people={people} value={reviewerIds} onChange={setReviewerIds} />}
        </Field>
        <Field label="Due date" error={errors.due} hint="Reviews past their due date are marked overdue.">
          {(props) => <Input {...props} type="date" min={isoDay(1)} value={due} onChange={(event) => setDue(event.target.value)} />}
        </Field>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={autoApply} onChange={(event) => setAutoApply(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Remove access automatically</span>
            <span className="text-caption text-fg-3">
              When the review is completed, Halo removes everyone a reviewer marked for removal. Leave this off to only record the decisions.
            </span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function NoteDialog({ review, item, decision, onClose }: { review: AccessReview; item: ReviewItem; decision: ReviewDecision; onClose: () => void }) {
  const [note, setNote] = useState(item.note);
  const { pending, run } = useMutation();
  const remove = decision === "remove";

  function submit() {
    void run("note", () => api(`/access-reviews/${review.id}/decisions/${item.user.id}`, { method: "PUT", body: { decision, note: note.trim() } }), onClose);
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title={remove ? `Remove ${item.user.name} from ${review.group.name}?` : `Keep ${item.user.name} in ${review.group.name}?`}
      description={
        remove
          ? review.autoApply
            ? `Halo removes them from ${review.group.name} when the review is completed.`
            : "Automatic removal is off for this review, so the decision is recorded and the group owner removes them by hand."
          : "They keep their membership. You can change the decision until the review is completed."
      }
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant={remove ? "destructive" : "primary"} loading={pending === "note"} onClick={submit}>
            {remove ? "Mark for removal" : "Keep access"}
          </Button>
        </>
      }
    >
      <Field label="Note" hint="Optional. Explain the decision for the group owner and the audit trail.">
        {(props) => <Textarea {...props} autoFocus value={note} onChange={(event) => setNote(event.target.value)} />}
      </Field>
    </Dialog>
  );
}

function DecisionControls({ review, item }: { review: AccessReview; item: ReviewItem }) {
  const { pending, run } = useMutation();
  const [noting, setNoting] = useState<ReviewDecision | null>(null);
  return (
    <span className="flex items-center justify-end gap-1">
      <Button
        size="sm"
        variant={item.decision === "keep" ? "secondary" : "quiet"}
        aria-pressed={item.decision === "keep"}
        loading={pending === "keep"}
        onClick={() =>
          run("keep", () =>
            api(`/access-reviews/${review.id}/decisions/${item.user.id}`, {
              method: "PUT",
              body: { decision: "keep", note: item.decision === "keep" ? item.note : "" },
            }),
          )
        }
      >
        <Check aria-hidden="true" size={16} strokeWidth={1.75} />
        Keep
      </Button>
      <Button
        size="sm"
        variant={item.decision === "remove" ? "secondary" : "quiet"}
        aria-pressed={item.decision === "remove"}
        onClick={() => setNoting("remove")}
      >
        <X aria-hidden="true" size={16} strokeWidth={1.75} />
        Remove
      </Button>
      <Button size="icon-sm" variant="quiet" aria-label={`Add a note about ${item.user.name}`} onClick={() => setNoting(item.decision ?? "keep")}>
        <MessageSquare aria-hidden="true" size={16} strokeWidth={1.75} />
      </Button>
      {noting ? <NoteDialog review={review} item={item} decision={noting} onClose={() => setNoting(null)} /> : null}
    </span>
  );
}

const DECISION = { keep: { label: "Keep", tone: "success" }, remove: { label: "Remove", tone: "danger" } } as const;

const views: SavedView[] = [
  { id: "all", label: "Everyone", filters: [] },
  { id: "undecided", label: "Needs a decision", filters: [{ id: "decision", value: ["none"] }] },
  { id: "remove", label: "Marked for removal", filters: [{ id: "decision", value: ["remove"] }] },
];

export function ReviewWorkspace({ review }: { review: AccessReview }) {
  const [completing, setCompleting] = useState(false);
  const completed = review.status === "completed";
  const undecided = review.total - review.decided;

  const columns = useMemo(() => {
    const list: ColumnDef<ReviewItem, any>[] = [
      {
        id: "person",
        accessorFn: (i) => i.user.name,
        header: "Member",
        meta: { label: "Member", hideable: false },
        cell: ({ row }) => (
          <span className="flex min-w-56 items-center gap-3">
            <Avatar name={row.original.user.name} size="md" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate font-medium text-fg">{row.original.user.name}</span>
              <span className="truncate text-caption text-fg-3">{row.original.title || row.original.user.email}</span>
            </span>
          </span>
        ),
      },
      {
        id: "decision",
        accessorFn: (i) => i.decision ?? "none",
        header: "Decision",
        filterFn: facetFilter,
        meta: { label: "Decision" },
        cell: ({ row }) => (
          <span className="flex max-w-80 flex-col gap-1 py-2">
            {row.original.decision ? (
              <Badge tone={DECISION[row.original.decision].tone} className="self-start">
                {DECISION[row.original.decision].label}
              </Badge>
            ) : (
              <span className="text-fg-3">No decision</span>
            )}
            {row.original.note ? (
              <span className="truncate text-caption text-fg-3" title={row.original.note}>
                “{row.original.note}”
              </span>
            ) : null}
          </span>
        ),
      },
      {
        id: "decidedBy",
        accessorFn: (i) => i.decidedBy?.name ?? "",
        header: "Decided by",
        meta: { label: "Decided by" },
        cell: ({ row }) =>
          row.original.decidedBy && row.original.decidedAt ? (
            <span className="flex flex-col whitespace-nowrap">
              <span>{row.original.decidedBy.name}</span>
              <span className="text-caption text-fg-3"><RelativeTime iso={row.original.decidedAt} /></span>
            </span>
          ) : (
            <span className="text-fg-3">—</span>
          ),
      },
    ];
    if (completed) {
      list.push({
        id: "outcome",
        accessorFn: (i) => i.outcome ?? "",
        header: "Result",
        meta: { label: "Result" },
        cell: ({ row }) => (row.original.outcome ? <Badge tone={OUTCOME[row.original.outcome].tone}>{OUTCOME[row.original.outcome].label}</Badge> : null),
      });
    }
    if (review.canDecide) {
      list.push({
        id: "decide",
        enableSorting: false,
        header: () => <span className="sr-only">Decide</span>,
        meta: { label: "Decide", align: "right", hideable: false },
        cell: ({ row }) => <DecisionControls review={review} item={row.original} />,
      });
    }
    return list;
  }, [review, completed]);

  return (
    <div className="flex flex-col gap-6">
      <Card className="flex flex-col gap-6 p-6 lg:flex-row lg:items-center lg:gap-8">
        <ReviewProgress review={review} className="w-full lg:w-48" />
        <p className="flex-1 text-body-sm text-fg-2">
          {completed
            ? `Completed by ${review.completedBy?.name ?? "a reviewer"}${review.completedAt ? ` on ${formatDate(review.completedAt)}` : ""}. Decisions can no longer change.`
            : `${pluralize(review.removals, "person", "people")} marked for removal, ${pluralize(undecided, "person", "people")} without a decision. ${
                review.autoApply ? "Removals apply when the review is completed." : "Automatic removal is off, so decisions are recorded only."
              }`}
        </p>
        {review.canComplete ? (
          <Button variant="primary" onClick={() => setCompleting(true)}>
            Complete review
          </Button>
        ) : null}
      </Card>
      <DataTable
        data={review.items ?? []}
        columns={columns}
        getRowId={(i) => i.user.id}
        label={`Members of ${review.group.name} under review`}
        noun={["member", "members"]}
        storageKey="access-review-items"
        searchText={(i) => `${i.user.name} ${i.user.email ?? ""} ${i.title} ${i.note}`}
        searchPlaceholder="Search members or notes"
        views={views}
        facets={[
          {
            column: "decision",
            label: "Decision",
            options: [
              { value: "keep", label: "Keep" },
              { value: "remove", label: "Remove" },
              { value: "none", label: "No decision" },
            ],
          },
        ]}
      />
      {completing ? <CompleteDialog review={review} onClose={() => setCompleting(false)} /> : null}
    </div>
  );
}

function CompleteDialog({ review, onClose }: { review: AccessReview; onClose: () => void }) {
  const { pending, run, toast } = useMutation();
  const undecided = review.total - review.decided;
  const removals = review.autoApply
    ? `Halo removes ${pluralize(review.removals, "person", "people")} marked for removal from ${review.group.name}.`
    : `Automatic removal is off, so ${pluralize(review.removals, "removal")} ${review.removals === 1 ? "is" : "are"} recorded but not applied.`;
  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title={`Complete ${review.name}?`}
      description={`${removals}${undecided ? ` ${pluralize(undecided, "person", "people")} without a decision keep their access.` : ""} Decisions can't change afterwards.`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={pending === "complete"}
            onClick={() =>
              run(
                "complete",
                () => api<AccessReview>(`/access-reviews/${review.id}/complete`, { method: "POST" }),
                () => {
                  onClose();
                  toast({ title: `${review.name} completed`, description: "Results are recorded in the audit log." });
                },
              )
            }
          >
            Complete review
          </Button>
        </>
      }
    />
  );
}

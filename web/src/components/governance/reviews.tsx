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
import { formatDate, isoDay } from "@/lib/format";
import type { AccessReview, ReviewDecision, ReviewItem } from "@/lib/governance-types";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { governanceMessages } from "@/lib/i18n/console/governance";
import type { Group } from "@/lib/types";
import { PeoplePicker } from "./people-picker";
import { OUTCOME, ReviewProgress, type Person } from "./shared";

export function StartReview({ groups, people }: { groups: Group[]; people: Person[] }) {
  const [open, setOpen] = useState(false);
  const t = useMessages(governanceMessages);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.reviews.start}
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
  const { pending, run, errorFor, toast, router } = useMutation();
  const t = useMessages(governanceMessages);
  const copy = t.reviews;
  const owners = (groups.find((g) => g.id === groupId)?.owners ?? []).map((o) => o.id).filter((id) => people.some((p) => p.id === id) && !reviewerIds.includes(id));

  function submit() {
    const next = {
      name: name.trim() ? undefined : copy.nameRequired,
      group: groupId ? undefined : copy.groupRequired,
      reviewers: reviewerIds.length ? undefined : copy.reviewersRequired,
      due: due >= isoDay(1) ? undefined : copy.dueRequired,
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
        toast({ title: copy.started(review.name), description: copy.startedHint(review.total) });
        router.push(`/admin/access-reviews/${review.id}`);
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("start")}
      title={copy.startTitle}
      description={copy.startDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "start"} onClick={submit}>
            {copy.start}
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
            <Input
              {...props}
              autoFocus
              autoComplete="off"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={copy.namePlaceholder}
            />
          )}
        </Field>
        <Field
          label={t.group}
          error={errors.group}
          hint={copy.groupHint}
        >
          {(props) => (
            <Select {...props} value={groupId} onChange={(event) => setGroupId(event.target.value)}>
              {groups.map((group) => (
                <option key={group.id} value={group.id} disabled={group.memberCount === 0}>
                  {group.name} · {t.members(group.memberCount)}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <div className="flex flex-col gap-3">
          <Field label={t.facts.reviewers} error={errors.reviewers} hint={copy.reviewersHint}>
            {(props) => <PeoplePicker {...props} people={people} value={reviewerIds} onChange={setReviewerIds} />}
          </Field>
          {owners.length ? (
            <Button size="sm" variant="quiet" className="self-start" onClick={() => setReviewerIds([...new Set([...reviewerIds, ...owners])])}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {copy.addOwners}
            </Button>
          ) : null}
        </div>
        <Field label={copy.dueDate} error={errors.due} hint={copy.dueHint}>
          {(props) => <Input {...props} type="date" min={isoDay(1)} value={due} onChange={(event) => setDue(event.target.value)} />}
        </Field>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={autoApply} onChange={(event) => setAutoApply(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{copy.autoApply}</span>
            <span className="text-caption text-fg-3">{copy.autoApplyHint}</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function NoteDialog({ review, item, decision, onClose }: { review: AccessReview; item: ReviewItem; decision: ReviewDecision; onClose: () => void }) {
  const [note, setNote] = useState(item.note);
  const { pending, run, errorFor } = useMutation();
  const t = useMessages(governanceMessages);
  const copy = t.reviews;
  const remove = decision === "remove";

  function submit() {
    void run("note", () => api(`/access-reviews/${review.id}/decisions/${item.user.id}`, { method: "PUT", body: { decision, note: note.trim() } }), onClose, "inline");
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("note")}
      title={remove ? copy.removeTitle(item.user.name, review.group.name) : copy.keepTitle(item.user.name, review.group.name)}
      description={remove ? (review.autoApply ? copy.removeAuto(review.group.name) : copy.removeManual) : copy.keepDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant={remove ? "destructive" : "primary"} loading={pending === "note"} onClick={submit}>
            {remove ? copy.markForRemoval : copy.keepAccess}
          </Button>
        </>
      }
    >
      <Field label={t.note} hint={copy.noteHint}>
        {(props) => <Textarea {...props} autoFocus value={note} onChange={(event) => setNote(event.target.value)} />}
      </Field>
    </Dialog>
  );
}

function DecisionControls({ review, item }: { review: AccessReview; item: ReviewItem }) {
  const { pending, run } = useMutation();
  const t = useMessages(governanceMessages).reviews;
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
        {t.keep}
      </Button>
      <Button
        size="sm"
        variant={item.decision === "remove" ? "secondary" : "quiet"}
        aria-pressed={item.decision === "remove"}
        onClick={() => setNoting("remove")}
      >
        <X aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.remove}
      </Button>
      <Button size="icon-sm" variant="quiet" aria-label={t.noteAbout(item.user.name)} onClick={() => setNoting(item.decision ?? "keep")}>
        <MessageSquare aria-hidden="true" size={16} strokeWidth={1.75} />
      </Button>
      {noting ? <NoteDialog review={review} item={item} decision={noting} onClose={() => setNoting(null)} /> : null}
    </span>
  );
}

const DECISION = { keep: "success", remove: "danger" } as const;

export function ReviewWorkspace({ review }: { review: AccessReview }) {
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.reviews;
  const views: SavedView[] = [
    { id: "all", label: t.everyone, filters: [] },
    { id: "undecided", label: copy.needsDecision, filters: [{ id: "decision", value: ["none"] }] },
    { id: "remove", label: copy.markedForRemoval, filters: [{ id: "decision", value: ["remove"] }] },
  ];
  const [completing, setCompleting] = useState(false);
  const completed = review.status === "completed";
  const undecided = review.total - review.decided;

  const columns = useMemo(() => {
    const list: ColumnDef<ReviewItem, any>[] = [
      {
        id: "person",
        accessorFn: (i) => i.user.name,
        header: copy.member,
        meta: { label: copy.member, hideable: false },
        cell: ({ row }) => (
          <span className="flex min-w-56 items-center gap-3">
            <Avatar name={row.original.user.name} size="md" />
            <span className="flex min-w-0 flex-col">
              <span className="flex min-w-0 items-center gap-2">
                <span className="truncate font-medium text-fg">{row.original.user.name}</span>
                {row.original.guest ? <Badge tone="info">{copy.guest}</Badge> : null}
              </span>
              <span className="truncate text-caption text-fg-3">{row.original.title || row.original.user.email}</span>
            </span>
          </span>
        ),
      },
      {
        id: "decision",
        accessorFn: (i) => i.decision ?? "none",
        header: t.decision,
        filterFn: facetFilter,
        meta: { label: t.decision },
        cell: ({ row }) => (
          <span className="flex max-w-80 flex-col gap-1 py-2">
            {row.original.decision ? (
              <Badge tone={DECISION[row.original.decision]} className="self-start">
                {copy[row.original.decision]}
              </Badge>
            ) : (
              <span className="text-fg-3">{copy.noDecision}</span>
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
        header: copy.decidedBy,
        meta: { label: copy.decidedBy },
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
        header: t.result,
        meta: { label: t.result },
        cell: ({ row }) => (row.original.outcome ? <Badge tone={OUTCOME[row.original.outcome]}>{t.outcome[row.original.outcome]}</Badge> : null),
      });
    }
    if (review.canDecide) {
      list.push({
        id: "decide",
        enableSorting: false,
        header: () => <span className="sr-only">{copy.decide}</span>,
        meta: { label: copy.decide, align: "right", hideable: false },
        cell: ({ row }) => <DecisionControls review={review} item={row.original} />,
      });
    }
    return list;
  }, [review, completed, t, copy]);

  return (
    <div className="flex flex-col gap-6">
      <Card className="flex flex-col gap-6 p-6 lg:flex-row lg:items-center lg:gap-8">
        <ReviewProgress review={review} className="w-full lg:w-48" locale={locale} />
        <p className="flex-1 text-body-sm text-fg-2">
          {completed
            ? copy.completedBy(review.completedBy?.name, review.completedAt ? formatDate(review.completedAt, locale) : undefined)
            : `${copy.summary(review.removals, undecided)}${review.autoApply ? copy.appliesOnComplete : copy.recordedOnly}`}
        </p>
        {review.canComplete ? (
          <Button variant="primary" onClick={() => setCompleting(true)}>
            {copy.complete}
          </Button>
        ) : null}
      </Card>
      <DataTable
        data={review.items ?? []}
        columns={columns}
        getRowId={(i) => i.user.id}
        label={copy.membersUnderReview(review.group.name)}
        noun={copy.noun}
        storageKey="access-review-items"
        searchText={(i) => `${i.user.name} ${i.user.email ?? ""} ${i.title} ${i.note}`}
        searchPlaceholder={copy.search}
        views={views}
        facets={[
          {
            column: "decision",
            label: t.decision,
            options: [
              { value: "keep", label: copy.keep },
              { value: "remove", label: copy.remove },
              { value: "none", label: copy.noDecision },
            ],
          },
        ]}
      />
      {completing ? <CompleteDialog review={review} onClose={() => setCompleting(false)} /> : null}
    </div>
  );
}

function CompleteDialog({ review, onClose }: { review: AccessReview; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(governanceMessages);
  const copy = t.reviews;
  const undecided = review.total - review.decided;
  const removals = review.autoApply ? copy.removalsAuto(review.removals, review.group.name) : copy.removalsManual(review.removals);
  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("complete")}
      title={copy.completeTitle(review.name)}
      description={copy.completeDescription(removals, undecided)}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
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
                  toast({ title: copy.completed(review.name), description: copy.completedHint });
                },
                "inline",
              )
            }
          >
            {copy.complete}
          </Button>
        </>
      }
    />
  );
}

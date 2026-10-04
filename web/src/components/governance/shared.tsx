import type { Tone } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { cn } from "@/lib/cn";
import { daysUntil, formatDate, formatDateTime, pluralize } from "@/lib/format";
import type {
  AccessRequest,
  AccessReview,
  LifecycleActionType,
  LifecycleTrigger,
  NamedRef,
  RequestStatus,
  ReviewOutcome,
  ReviewStatus,
} from "@/lib/governance-types";

export type Person = { id: string; name: string; email: string };

export const DURATIONS = [1, 2, 7, 14, 30, 90, 180, 365];

export const REQUEST_STATUS: Record<RequestStatus, { label: string; tone: Tone }> = {
  pending: { label: "Pending", tone: "warning" },
  approved: { label: "Approved", tone: "success" },
  denied: { label: "Denied", tone: "danger" },
  cancelled: { label: "Cancelled", tone: "neutral" },
};

export const REVIEW_STATUS: Record<ReviewStatus, { label: string; tone: Tone }> = {
  overdue: { label: "Overdue", tone: "danger" },
  "in-progress": { label: "In progress", tone: "info" },
  completed: { label: "Completed", tone: "success" },
};

export const OUTCOME: Record<ReviewOutcome, { label: string; tone: Tone }> = {
  kept: { label: "Kept", tone: "success" },
  removed: { label: "Removed", tone: "danger" },
  "not-removed": { label: "Marked for removal", tone: "warning" },
  "no-decision": { label: "No decision, kept", tone: "neutral" },
};

export const TRIGGER: Record<LifecycleTrigger, { label: string; description: string }> = {
  joiner: { label: "Joiner", description: "When someone is created in Halo" },
  mover: { label: "Mover", description: "When someone's department, title or location changes" },
  leaver: { label: "Leaver", description: "When someone is suspended or deprovisioned" },
};

export const ACTION: Record<LifecycleActionType, string> = {
  "add-to-group": "Add to group",
  "remove-from-group": "Remove from group",
  "revoke-sessions": "Revoke sessions",
  suspend: "Suspend account",
};

export function joinNames(refs: { name: string }[]): string {
  const names = refs.map((ref) => ref.name);
  return names.length < 2 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

export function approversLabel(refs: NamedRef[]): string {
  return refs.length ? joinNames(refs) : "a global administrator";
}

export function durationLabel(days: number | null): string {
  return days === null ? "Until revoked" : pluralize(days, "day");
}

export function grantLabel(request: AccessRequest, withTime = true): string {
  if (request.status !== "approved") return "—";
  if (request.endedAt) return `${request.endReason === "revoked" ? "Revoked" : "Expired"} ${formatDate(request.endedAt)}`;
  if (!request.expiresAt) return "Until revoked";
  return `Until ${withTime ? formatDateTime(request.expiresAt) : formatDate(request.expiresAt)}`;
}

export function dueLabel(review: AccessReview): string {
  if (review.status === "completed") return review.completedAt ? `Completed ${formatDate(review.completedAt)}` : "Completed";
  const days = daysUntil(review.dueAt);
  if (review.status === "overdue") return `${pluralize(Math.max(1, -days), "day")} overdue`;
  return days <= 0 ? "Due today" : `Due in ${pluralize(days, "day")}`;
}

export function ReviewProgress({ review, className }: { review: AccessReview; className?: string }) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <span className="tnum text-body-sm text-fg-2">
        {review.decided} of {review.total} decided
      </span>
      <div
        role="progressbar"
        aria-label={`${review.name} progress`}
        aria-valuemin={0}
        aria-valuemax={review.total}
        aria-valuenow={review.decided}
        className="h-1 overflow-hidden rounded-full bg-n800"
      >
        <div
          className={review.status === "completed" ? "h-full bg-success" : "h-full bg-fg-2"}
          style={{ width: `${review.total ? (review.decided / review.total) * 100 : 0}%` }}
        />
      </div>
    </div>
  );
}

export function ReviewFacts({ review }: { review: AccessReview }) {
  return (
    <DescriptionList
      columns={2}
      items={[
        { label: "Group", value: review.group.name },
        { label: "Reviewers", value: joinNames(review.reviewers) },
        { label: "Due", value: `${formatDate(review.dueAt)} · ${dueLabel(review)}` },
        { label: "Automatic removal", value: review.autoApply ? "On: removals apply when the review is completed" : "Off: decisions are recorded only" },
        { label: "Started", value: `${formatDate(review.createdAt)}${review.createdBy ? ` by ${review.createdBy.name}` : ""}` },
      ]}
    />
  );
}

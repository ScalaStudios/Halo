import type { Tone } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { cn } from "@/lib/cn";
import { daysUntil, formatDate, formatDateTime } from "@/lib/format";
import type {
  AccessRequest,
  AccessReview,
  AccessStage,
  LifecycleActionType,
  LifecycleTrigger,
  NamedRef,
  RequestStatus,
  ReviewOutcome,
  ReviewStatus,
} from "@/lib/governance-types";
import { governanceMessages } from "@/lib/i18n/console/governance";
import type { Locale } from "@/lib/i18n/locale";

export type Person = { id: string; name: string; email: string };

export const DURATIONS = [1, 2, 7, 14, 30, 90, 180, 365];

export const REQUEST_STATUS: Record<RequestStatus, { tone: Tone }> = {
  pending: { tone: "warning" },
  approved: { tone: "success" },
  denied: { tone: "danger" },
  cancelled: { tone: "neutral" },
  expired: { tone: "neutral" },
};

export const REVIEW_STATUS: Record<ReviewStatus, { tone: Tone }> = {
  overdue: { tone: "danger" },
  "in-progress": { tone: "info" },
  completed: { tone: "success" },
};

export const OUTCOME: Record<ReviewOutcome, Tone> = {
  kept: "success",
  removed: "danger",
  "not-removed": "warning",
  "no-decision": "neutral",
};

export const TRIGGERS = Object.keys(governanceMessages.en.triggers) as LifecycleTrigger[];

export const ACTIONS = Object.keys(governanceMessages.en.actions) as LifecycleActionType[];

export function joinNames(refs: { name: string }[], locale: Locale): string {
  const names = refs.map((ref) => ref.name);
  return names.length < 2 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} ${governanceMessages[locale].and} ${names[names.length - 1]}`;
}

export function approversLabel(refs: NamedRef[], locale: Locale): string {
  return refs.length ? joinNames(refs, locale) : governanceMessages[locale].globalAdmin;
}

export function durationLabel(days: number | null, locale: Locale): string {
  const t = governanceMessages[locale];
  return days === null ? t.untilRevoked : t.days(days);
}

export function requestDurationLabel(request: AccessRequest, locale: Locale): string {
  return request.extends && request.durationDays !== null ? governanceMessages[locale].moreDays(request.durationDays) : durationLabel(request.durationDays, locale);
}

export function stagesLabel(stages: AccessStage[], locale: Locale, manager = governanceMessages[locale].requesterManager): string {
  const t = governanceMessages[locale];
  return stages
    .map((stage) =>
      stage.approver === "manager"
        ? manager
        : stage.approver === "group-owners"
          ? t.ownersOf(stage.group?.name ?? t.removedGroup)
          : approversLabel(stage.approvers, locale),
    )
    .join(t.then);
}

export function grantLabel(request: AccessRequest, locale: Locale, withTime = true): string {
  const t = governanceMessages[locale];
  if (request.status !== "approved") return "—";
  if (request.endedAt) return t.ended[request.endReason ?? "expired"](formatDate(request.endedAt, locale));
  if (!request.expiresAt) return t.untilRevoked;
  return t.until(withTime ? formatDateTime(request.expiresAt, locale) : formatDate(request.expiresAt, locale));
}

export function dueLabel(review: AccessReview, locale: Locale): string {
  const t = governanceMessages[locale];
  if (review.status === "completed") return review.completedAt ? t.completedOn(formatDate(review.completedAt, locale)) : t.reviewStatus.completed;
  const days = daysUntil(review.dueAt);
  if (review.status === "overdue") return t.overdueBy(Math.max(1, -days));
  return days <= 0 ? t.dueToday : t.dueIn(days);
}

export function ReviewProgress({ review, className, locale }: { review: AccessReview; className?: string; locale: Locale }) {
  const t = governanceMessages[locale];
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <span className="tnum text-body-sm text-fg-2">{t.decidedOf(review.decided, review.total)}</span>
      <div
        role="progressbar"
        aria-label={t.progress(review.name)}
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

export function ReviewFacts({ review, locale }: { review: AccessReview; locale: Locale }) {
  const t = governanceMessages[locale];
  return (
    <DescriptionList
      columns={2}
      items={[
        { label: t.group, value: review.group.name },
        { label: t.facts.reviewers, value: joinNames(review.reviewers, locale) },
        { label: t.facts.due, value: `${formatDate(review.dueAt, locale)} · ${dueLabel(review, locale)}` },
        { label: t.facts.autoRemoval, value: review.autoApply ? t.autoRemovalOn : t.autoRemovalOff },
        { label: t.facts.started, value: t.startedBy(formatDate(review.createdAt, locale), review.createdBy?.name) },
      ]}
    />
  );
}

import Link from "next/link";
import { ClipboardCheck } from "lucide-react";
import { StartReview } from "@/components/governance/reviews";
import { dueLabel, joinNames, REVIEW_STATUS, ReviewProgress } from "@/components/governance/shared";
import { Badge } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { AccessReview } from "@/lib/governance-types";
import type { Group, User } from "@/lib/types";

export const metadata = { title: "Access reviews" };

export default async function AccessReviewsPage() {
  const [reviews, groups, users] = await Promise.all([apiGet<AccessReview[]>("/access-reviews"), apiGet<Group[]>("/groups"), apiGet<User[]>("/users")]);
  const open = reviews.filter((r) => r.status !== "completed").length;
  const overdue = reviews.filter((r) => r.status === "overdue").length;

  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Access reviews"
        description={`${pluralize(open, "review")} open, ${overdue} overdue. Reviewers confirm or remove each member's access to a group.`}
        actions={
          <StartReview
            groups={groups.filter((g) => g.kind === "assigned")}
            people={users.filter((u) => u.status !== "deprovisioned").map((u) => ({ id: u.id, name: u.name, email: u.email }))}
          />
        }
      />
      <Card>
        {reviews.length === 0 ? (
          <EmptyState
            icon={ClipboardCheck}
            title="No access reviews yet"
            description="Start a review to have reviewers confirm who still needs each member of a group."
          />
        ) : (
          <>
            <h2 className="sr-only">Reviews, most urgent first</h2>
            <ul>
              {reviews.map((review) => (
                <li key={review.id} className="flex flex-col gap-4 border-t border-border p-6 first:border-t-0 lg:flex-row lg:items-center lg:gap-8">
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <div className="flex flex-wrap items-center gap-3">
                      <h3 className="text-h4 text-fg">{review.name}</h3>
                      <Badge tone={REVIEW_STATUS[review.status].tone}>{REVIEW_STATUS[review.status].label}</Badge>
                    </div>
                    <p className="text-body-sm text-fg-3">
                      {review.group.name}, {pluralize(review.total, "member")}
                    </p>
                    <p className="text-caption text-fg-3">
                      Reviewers: {joinNames(review.reviewers)} · {dueLabel(review)}
                    </p>
                  </div>
                  <ReviewProgress review={review} className="w-full lg:w-48" />
                  <Link href={`/admin/access-reviews/${review.id}`} className={buttonClasses("secondary", "sm", "shrink-0")}>
                    {review.status === "completed" ? "View results" : "Open review"}
                  </Link>
                </li>
              ))}
            </ul>
          </>
        )}
      </Card>
    </div>
  );
}

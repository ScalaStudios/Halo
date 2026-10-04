import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { ReviewWorkspace } from "@/components/governance/reviews";
import { REVIEW_STATUS, ReviewFacts } from "@/components/governance/shared";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { AccessReview } from "@/lib/governance-types";

type Params = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: Params }) {
  const { id } = await params;
  return { title: (await apiGet<AccessReview>(`/access-reviews/${encodeURIComponent(id)}`)).name };
}

export default async function AccountReviewPage({ params }: { params: Params }) {
  const { id } = await params;
  const review = await apiGet<AccessReview>(`/access-reviews/${encodeURIComponent(id)}`);
  return (
    <div className="flex animate-page flex-col gap-12">
      <div className="flex flex-col gap-6">
        <Link
          href="/account/access"
          className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg"
        >
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Access
        </Link>
        <PageHeader
          size="large"
          title={review.name}
          meta={<Badge tone={REVIEW_STATUS[review.status].tone}>{REVIEW_STATUS[review.status].label}</Badge>}
          description={`You were asked to confirm who still needs ${review.group.name}. Keep the people who need it and remove the rest.`}
        />
      </div>
      <Card className="p-6">
        <ReviewFacts review={review} />
      </Card>
      <ReviewWorkspace review={review} />
    </div>
  );
}

import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { ReviewWorkspace } from "@/components/governance/reviews";
import { REVIEW_STATUS, ReviewFacts } from "@/components/governance/shared";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { AccessReview } from "@/lib/governance-types";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";

type Params = Promise<{ id: string }>;

export async function generateMetadata({ params }: { params: Params }) {
  const { id } = await params;
  return { title: (await apiGet<AccessReview>(`/access-reviews/${encodeURIComponent(id)}`)).name };
}

export default async function AccessReviewPage({ params }: { params: Params }) {
  const { id } = await params;
  const [review, locale] = await Promise.all([apiGet<AccessReview>(`/access-reviews/${encodeURIComponent(id)}`), requestLocale()]);
  const t = governanceMessages[locale];
  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link
          href="/admin/access-reviews"
          className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg"
        >
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.accessReviews.label}
        </Link>
        <PageHeader
          title={review.name}
          meta={<Badge tone={REVIEW_STATUS[review.status].tone}>{t.reviewStatus[review.status]}</Badge>}
          description={t.pages.reviewDescription(review.group.name)}
        />
      </div>
      <Card className="p-6">
        <ReviewFacts review={review} locale={locale} />
      </Card>
      <ReviewWorkspace review={review} />
    </div>
  );
}

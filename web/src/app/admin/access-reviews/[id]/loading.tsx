import { ReviewSkeleton } from "@/components/governance/review-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton back={{ href: "/admin/access-reviews", label: frameMessages[locale].nav.items.accessReviews.label }} description={50}>
      <ReviewSkeleton locale={locale} />
    </PageSkeleton>
  );
}

import { ReviewSkeleton } from "@/components/governance/review-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton large back={{ href: "/account/access", label: accountMessages(locale).nav.access }} description={110}>
      <ReviewSkeleton locale={locale} />
    </PageSkeleton>
  );
}

import { PageSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import {
  ApplicationHealthSkeleton,
  AttentionSkeleton,
  InsightsSkeleton,
  OrganizationCardSkeleton,
  PostureSkeleton,
  ProfileCardSkeleton,
  RecentSignInsSkeleton,
  RiskCardSkeleton,
  Shortcuts,
  WaitingOnYouSkeleton,
} from "./cards";

export default async function Loading() {
  const locale = await requestLocale();
  return (
    <PageSkeleton title={frameMessages[locale].nav.items.overview.label} description={44}>
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2 min-[1400px]:grid-cols-3">
        <OrganizationCardSkeleton locale={locale} />
        <ProfileCardSkeleton locale={locale} />
        <RiskCardSkeleton locale={locale} />
      </div>
      <Shortcuts locale={locale} />
      <PostureSkeleton />
      <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
        <div className="flex flex-col gap-8 xl:col-span-8">
          <AttentionSkeleton locale={locale} />
          <RecentSignInsSkeleton locale={locale} />
        </div>
        <div className="flex flex-col gap-8 xl:col-span-4">
          <WaitingOnYouSkeleton locale={locale} />
          <InsightsSkeleton locale={locale} />
          <ApplicationHealthSkeleton locale={locale} />
        </div>
      </div>
    </PageSkeleton>
  );
}

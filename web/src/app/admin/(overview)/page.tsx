import { Suspense } from "react";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { OrganizationProfile } from "@/lib/settings-types";
import {
  ApplicationHealth,
  ApplicationHealthSkeleton,
  Attention,
  AttentionSkeleton,
  Insights,
  InsightsSkeleton,
  OrganizationCard,
  OrganizationCardSkeleton,
  Posture,
  PostureSkeleton,
  ProfileCard,
  ProfileCardSkeleton,
  RecentSignIns,
  RecentSignInsSkeleton,
  RiskCard,
  RiskCardSkeleton,
  Shortcuts,
  WaitingOnYou,
  WaitingOnYouSkeleton,
} from "./cards";

export const metadata = { title: { absolute: "Halo administration" } };

export default async function OverviewPage() {
  const org = await apiGet<OrganizationProfile>("/organization");
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title="Overview" description={`What needs attention in ${org.name} right now.`} />

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2 min-[1400px]:grid-cols-3">
        <Suspense fallback={<OrganizationCardSkeleton />}>
          <OrganizationCard org={org} />
        </Suspense>
        <Suspense fallback={<ProfileCardSkeleton />}>
          <ProfileCard />
        </Suspense>
        <Suspense fallback={<RiskCardSkeleton />}>
          <RiskCard />
        </Suspense>
      </div>

      <Shortcuts />

      <Suspense fallback={<PostureSkeleton />}>
        <Posture />
      </Suspense>

      <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
        <div className="flex flex-col gap-8 xl:col-span-8">
          <Suspense fallback={<AttentionSkeleton />}>
            <Attention />
          </Suspense>
          <Suspense fallback={<RecentSignInsSkeleton />}>
            <RecentSignIns />
          </Suspense>
        </div>

        <div className="flex flex-col gap-8 xl:col-span-4">
          <Suspense fallback={<WaitingOnYouSkeleton />}>
            <WaitingOnYou />
          </Suspense>
          <Suspense fallback={<InsightsSkeleton />}>
            <Insights />
          </Suspense>
          <Suspense fallback={<ApplicationHealthSkeleton />}>
            <ApplicationHealth />
          </Suspense>
        </div>
      </div>
    </div>
  );
}

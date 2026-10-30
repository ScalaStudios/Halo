import { PageSkeleton } from "@/components/ui/skeleton";
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

export default function Loading() {
  return (
    <PageSkeleton title="Overview" description={44}>
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2 min-[1400px]:grid-cols-3">
        <OrganizationCardSkeleton />
        <ProfileCardSkeleton />
        <RiskCardSkeleton />
      </div>
      <Shortcuts />
      <PostureSkeleton />
      <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
        <div className="flex flex-col gap-8 xl:col-span-8">
          <AttentionSkeleton />
          <RecentSignInsSkeleton />
        </div>
        <div className="flex flex-col gap-8 xl:col-span-4">
          <WaitingOnYouSkeleton />
          <InsightsSkeleton />
          <ApplicationHealthSkeleton />
        </div>
      </div>
    </PageSkeleton>
  );
}

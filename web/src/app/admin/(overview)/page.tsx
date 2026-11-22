import { redirect } from "next/navigation";
import { Suspense } from "react";
import { limitedOnly } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import { requestLocale } from "@/lib/i18n/server";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { User } from "@/lib/types";
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

export async function generateMetadata() {
  return { title: { absolute: frameMessages[await requestLocale()].title } };
}

export default async function OverviewPage() {
  const [org, me, locale] = await Promise.all([apiGet<OrganizationProfile>("/organization"), apiGet<User>("/me"), requestLocale()]);
  if (limitedOnly(me)) redirect("/admin/users");
  const governance = !org.featuresOff.includes("governance");
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.overview.label} description={monitoringMessages[locale].overview.description(org.name)} />

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2 min-[1400px]:grid-cols-3">
        <Suspense fallback={<OrganizationCardSkeleton locale={locale} />}>
          <OrganizationCard org={org} locale={locale} />
        </Suspense>
        <Suspense fallback={<ProfileCardSkeleton locale={locale} />}>
          <ProfileCard locale={locale} />
        </Suspense>
        <Suspense fallback={<RiskCardSkeleton locale={locale} />}>
          <RiskCard locale={locale} />
        </Suspense>
      </div>

      <Shortcuts locale={locale} />

      <Suspense fallback={<PostureSkeleton />}>
        <Posture governance={governance} locale={locale} />
      </Suspense>

      <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
        <div className="flex flex-col gap-8 xl:col-span-8">
          <Suspense fallback={<AttentionSkeleton locale={locale} />}>
            <Attention governance={governance} locale={locale} />
          </Suspense>
          <Suspense fallback={<RecentSignInsSkeleton locale={locale} />}>
            <RecentSignIns locale={locale} />
          </Suspense>
        </div>

        <div className="flex flex-col gap-8 xl:col-span-4">
          {governance ? (
            <Suspense fallback={<WaitingOnYouSkeleton locale={locale} />}>
              <WaitingOnYou locale={locale} />
            </Suspense>
          ) : null}
          <Suspense fallback={<InsightsSkeleton locale={locale} />}>
            <Insights />
          </Suspense>
          <Suspense fallback={<ApplicationHealthSkeleton locale={locale} />}>
            <ApplicationHealth locale={locale} />
          </Suspense>
        </div>
      </div>
    </div>
  );
}

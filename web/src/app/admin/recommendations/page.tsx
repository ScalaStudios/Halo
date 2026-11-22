import { Recommendations } from "@/components/monitoring/recommendations";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { Recommendation, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: policyMessages[await requestLocale()].recommendations.title };
}

export default async function RecommendationsPage() {
  const [checks, me, locale] = await Promise.all([apiGet<Recommendation[]>("/recommendations"), apiGet<User>("/me"), requestLocale()]);
  const t = policyMessages[locale].recommendations;
  const open = checks.filter((c) => c.status !== "pass" && !c.dismissal).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title={t.title}
        description={t.summary(open, checks.length)}
      />
      <Recommendations checks={checks} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}

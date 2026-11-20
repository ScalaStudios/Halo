import { Recommendations } from "@/components/monitoring/recommendations";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Recommendation, User } from "@/lib/types";

export const metadata = { title: "Security recommendations" };

export default async function RecommendationsPage() {
  const [checks, me] = await Promise.all([apiGet<Recommendation[]>("/recommendations"), apiGet<User>("/me")]);
  const open = checks.filter((c) => c.status !== "pass" && !c.dismissal).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title="Security recommendations"
        description={`${open} of ${checks.length} checks ${open === 1 ? "needs" : "need"} action. Halo runs them against your current settings, administrators, API keys, policies, applications and signing keys each time this page opens.`}
      />
      <Recommendations checks={checks} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}

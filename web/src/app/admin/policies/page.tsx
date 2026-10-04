import Link from "next/link";
import { Plus } from "lucide-react";
import { PolicyList, ReportInsights } from "@/components/policy/policy-list";
import type { Names } from "@/components/policy/shared";
import { SimulateDialog } from "@/components/policy/simulate-dialog";
import { ZonesSection } from "@/components/policy/zones";
import { buttonClasses } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { NetworkZone, PolicyWithActivity } from "@/lib/policy-types";
import type { Application, Group, User } from "@/lib/types";

export const metadata = { title: "Policies" };

export default async function PoliciesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab = "policies" } = await searchParams;
  const [policies, zones, groups, users, applications] = await Promise.all([
    apiGet<PolicyWithActivity[]>("/policies"),
    apiGet<NetworkZone[]>("/network-zones"),
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
  ]);
  const names: Names = {
    groups: Object.fromEntries(groups.map((g) => [g.id, g.name])),
    users: Object.fromEntries(users.map((u) => [u.id, u.name])),
    apps: Object.fromEntries(applications.map((a) => [a.id, a.name])),
    zones: Object.fromEntries(zones.map((z) => [z.id, z.name])),
  };
  const usedBy = Object.fromEntries(zones.map((z) => [z.id, policies.filter((p) => p.conditions.zoneIds.includes(z.id)).map((p) => p.name)]));
  const enforced = policies.filter((p) => p.enabled && p.mode === "enforce").length;
  const report = policies.filter((p) => p.enabled && p.mode === "report").length;

  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Policies"
        description={`${pluralize(policies.length, "policy", "policies")}: ${enforced} enforced, ${report} report-only. At every sign-in Halo checks each policy that is on, and the most restrictive enforced match decides.`}
        actions={
          <>
            <SimulateDialog
              users={users.filter((u) => u.status !== "deprovisioned").map((u) => ({ id: u.id, name: u.name }))}
              applications={applications.filter((a) => a.type !== "service").map((a) => ({ id: a.id, name: a.name }))}
            />
            {tab === "policies" ? (
              <Link href="/admin/policies/new" className={buttonClasses("primary", "sm")}>
                <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                Create policy
              </Link>
            ) : null}
          </>
        }
      />
      <TabNav
        label="Policy sections"
        tabs={[
          { id: "policies", label: "Policies", count: policies.length },
          { id: "networks", label: "Networks", count: zones.length },
        ]}
        active={tab}
        hrefFor={(id) => (id === "policies" ? "/admin/policies" : `/admin/policies?tab=${id}`)}
      />
      <div key={tab} className="flex animate-enter flex-col gap-8">
        {tab === "networks" ? (
          <ZonesSection zones={zones} usedBy={usedBy} />
        ) : (
          <>
            <ReportInsights policies={policies} />
            <PolicyList policies={policies} names={names} />
          </>
        )}
      </div>
    </div>
  );
}

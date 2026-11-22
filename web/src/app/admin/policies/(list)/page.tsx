import Link from "next/link";
import { Plus } from "lucide-react";
import { PolicyList, ReportInsights } from "@/components/policy/policy-list";
import { ContextsSection } from "@/components/policy/contexts";
import type { Names } from "@/components/policy/shared";
import { SimulateDialog } from "@/components/policy/simulate-dialog";
import { TermsSection } from "@/components/policy/terms";
import { ZonesSection } from "@/components/policy/zones";
import { buttonClasses } from "@/components/ui/button";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { AuthContext, NetworkZone, PolicyWithActivity, Terms } from "@/lib/policy-types";
import type { Application, Group, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.policies.label };
}

export default async function PoliciesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab = "policies" } = await searchParams;
  const [policies, zones, terms, contexts, groups, users, applications, locale] = await Promise.all([
    apiGet<PolicyWithActivity[]>("/policies"),
    apiGet<NetworkZone[]>("/network-zones"),
    apiGet<Terms[]>("/terms"),
    apiGet<AuthContext[]>("/authentication-contexts"),
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
    requestLocale(),
  ]);
  const copy = policyMessages[locale].page;
  const names: Names = {
    groups: Object.fromEntries(groups.map((g) => [g.id, g.name])),
    users: Object.fromEntries(users.map((u) => [u.id, u.name])),
    apps: Object.fromEntries(applications.map((a) => [a.id, a.name])),
    zones: Object.fromEntries(zones.map((z) => [z.id, z.name])),
    terms: Object.fromEntries(terms.map((t) => [t.id, t.name])),
    contexts: Object.fromEntries(contexts.map((c) => [c.id, c.name])),
  };
  const usedBy = Object.fromEntries(zones.map((z) => [z.id, policies.filter((p) => p.conditions.zoneIds.includes(z.id)).map((p) => p.name)]));
  const termsUsedBy = Object.fromEntries(terms.map((t) => [t.id, policies.filter((p) => p.termsId === t.id).map((p) => p.name)]));
  const contextsUsedBy = Object.fromEntries(contexts.map((c) => [c.id, policies.filter((p) => p.contextId === c.id).map((p) => p.name)]));
  const enforced = policies.filter((p) => p.enabled && p.mode === "enforce").length;
  const report = policies.filter((p) => p.enabled && p.mode === "report").length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.policies.label}
        description={copy.summary(policies.length, enforced, report)}
        actions={
          <>
            <SimulateDialog
              users={users.filter((u) => u.status !== "deprovisioned").map((u) => ({ id: u.id, name: u.name }))}
              applications={applications.filter((a) => a.type !== "service" && a.protocol !== "bookmark").map((a) => ({ id: a.id, name: a.name }))}
            />
            {tab === "policies" ? (
              <Link href="/admin/policies/new" className={buttonClasses("primary", "sm")}>
                <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                {copy.create}
              </Link>
            ) : null}
          </>
        }
      />
      <TabNav
        label={copy.sections}
        tabs={[
          { id: "policies", label: copy.tabs.policies, count: policies.length },
          { id: "networks", label: copy.tabs.networks, count: zones.length },
          { id: "terms", label: copy.tabs.terms, count: terms.length },
          { id: "contexts", label: copy.tabs.contexts, count: contexts.length },
        ]}
        active={tab}
        hrefFor={(id) => (id === "policies" ? "/admin/policies" : `/admin/policies?tab=${id}`)}
      />
      <div key={tab} className="flex animate-enter flex-col gap-8">
        {tab === "networks" ? (
          <ZonesSection zones={zones} usedBy={usedBy} />
        ) : tab === "terms" ? (
          <TermsSection terms={terms} usedBy={termsUsedBy} />
        ) : tab === "contexts" ? (
          <ContextsSection contexts={contexts} usedBy={contextsUsedBy} />
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

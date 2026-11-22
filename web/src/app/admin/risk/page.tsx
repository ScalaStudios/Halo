import { ShieldCheck } from "lucide-react";
import { RiskResponsesForm } from "@/components/policy/risk-responses";
import { RiskTable } from "@/components/policy/risk-table";
import { hasRole } from "@/components/settings/roles";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { RiskEvent } from "@/lib/policy-types";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.risk.label };
}

export default async function RiskPage() {
  const [events, users, { settings }, me, locale] = await Promise.all([
    apiGet<RiskEvent[]>("/risk-events"),
    apiGet<User[]>("/users"),
    apiGet<SettingsResponse>("/settings"),
    apiGet<User>("/me"),
    requestLocale(),
  ]);
  const t = policyMessages[locale].risk;
  const open = events.filter((e) => e.status === "open").length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.risk.label}
        description={t.summary(open)}
      />
      {events.length ? (
        <RiskTable events={events} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
      ) : (
        <Card>
          <EmptyState icon={ShieldCheck} title={t.empty} description={t.emptyDescription} />
        </Card>
      )}
      <RiskResponsesForm settings={settings} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}

import { ShieldCheck } from "lucide-react";
import { RiskResponsesForm } from "@/components/policy/risk-responses";
import { RiskTable } from "@/components/policy/risk-table";
import { hasRole } from "@/components/settings/roles";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { RiskEvent } from "@/lib/policy-types";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Risk events" };

export default async function RiskPage() {
  const [events, users, { settings }, me] = await Promise.all([apiGet<RiskEvent[]>("/risk-events"), apiGet<User[]>("/users"), apiGet<SettingsResponse>("/settings"), apiGet<User>("/me")]);
  const open = events.filter((e) => e.status === "open").length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Risk events"
        description={`${pluralize(open, "open risk event")}. Halo raises one when someone signs in from a risky network, after 5 failed sign-ins in 15 minutes, or from a new device.`}
      />
      {events.length ? (
        <RiskTable events={events} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
      ) : (
        <Card>
          <EmptyState icon={ShieldCheck} title="No risk events" description="Nothing risky has happened yet. Medium and high risk sign-ins appear here so you can resolve or dismiss them." />
        </Card>
      )}
      <RiskResponsesForm settings={settings} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}

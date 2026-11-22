import { CheckCheck } from "lucide-react";
import { ApprovalSettingsForm, DecidedApprovals, PendingApprovals } from "@/components/settings/approvals";
import { hasRole } from "@/components/settings/roles";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";
import type { ApprovalRequest, SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.approvals.label };
}

export default async function ApprovalsPage() {
  const [requests, { settings }, me, people, locale] = await Promise.all([
    apiGet<ApprovalRequest[]>("/approvals"),
    apiGet<SettingsResponse>("/settings"),
    apiGet<User>("/me"),
    apiGet<User[]>("/users"),
    requestLocale(),
  ]);
  const t = governanceMessages[locale];
  const pending = requests.filter((r) => r.status === "pending");
  const decided = requests.filter((r) => r.status !== "pending");
  const yours = pending.filter((r) => r.canDecide).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.approvals.label}
        description={settings.approvalActions.length ? t.pages.approvalsDescription(pending.length, yours) : t.pages.approvalsNone}
      />
      <Card>
        <CardHeader title={t.pending} description={pending.length ? t.newestFirst : undefined} />
        <CardBody>
          <PendingApprovals requests={pending} />
        </CardBody>
      </Card>
      <ApprovalSettingsForm key={JSON.stringify([settings.approvalActions, settings.emergencyAccessIds])} settings={settings} people={people} canEdit={hasRole(me)} />
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.decided} description={t.pages.approvalsDecided} />
        {decided.length ? (
          <DecidedApprovals requests={decided} />
        ) : (
          <Card>
            <EmptyState icon={CheckCheck} title={t.noDecisions} description={t.pages.approvalsEmpty} />
          </Card>
        )}
      </section>
    </div>
  );
}

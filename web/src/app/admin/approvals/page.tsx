import { CheckCheck } from "lucide-react";
import { ApprovalSettingsForm, DecidedApprovals, PendingApprovals } from "@/components/settings/approvals";
import { hasRole } from "@/components/settings/roles";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { ApprovalRequest, SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Approvals" };

export default async function ApprovalsPage() {
  const [requests, { settings }, me, people] = await Promise.all([
    apiGet<ApprovalRequest[]>("/approvals"),
    apiGet<SettingsResponse>("/settings"),
    apiGet<User>("/me"),
    apiGet<User[]>("/users"),
  ]);
  const pending = requests.filter((r) => r.status === "pending");
  const decided = requests.filter((r) => r.status !== "pending");
  const yours = pending.filter((r) => r.canDecide).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Approvals"
        description={
          settings.approvalActions.length
            ? `${pluralize(pending.length, "change")} waiting for a second global administrator, ${yours} of them yours to decide. Approving applies the change once, as the person who asked.`
            : "No changes need approval yet. Choose which ones do below."
        }
      />
      <Card>
        <CardHeader title="Pending" description={pending.length ? "Newest first." : undefined} />
        <CardBody>
          <PendingApprovals requests={pending} />
        </CardBody>
      </Card>
      <ApprovalSettingsForm settings={settings} people={people} canEdit={hasRole(me)} />
      <section className="flex flex-col gap-4">
        <SectionTitle title="Decided" description="Approved, rejected, expired and unapplied changes, newest first." />
        {decided.length ? (
          <DecidedApprovals requests={decided} />
        ) : (
          <Card>
            <EmptyState icon={CheckCheck} title="No decisions yet" description="Approved, rejected and expired changes appear here." />
          </Card>
        )}
      </section>
    </div>
  );
}

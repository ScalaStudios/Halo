import { CheckCheck } from "lucide-react";
import { DecidedRequests, PendingRequests } from "@/components/governance/requests";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { AccessRequest } from "@/lib/governance-types";

export const metadata = { title: "Requests" };

export default async function RequestsPage() {
  const [pending, decided] = await Promise.all([
    apiGet<AccessRequest[]>("/access-requests?status=pending"),
    apiGet<AccessRequest[]>("/access-requests?status=approved,denied,cancelled"),
  ]);
  const yours = pending.filter((r) => r.canDecide).length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Requests"
        description={`${pluralize(pending.length, "access request")} waiting for a decision, ${yours} of them yours to decide. Approved access is removed automatically when it expires.`}
      />
      <Card>
        <CardHeader title="Pending" description={pending.length ? "Newest first." : undefined} />
        <CardBody>
          <PendingRequests requests={pending} linkPeople />
        </CardBody>
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Decided" description="Approvals, denials and cancellations, newest first. Revoke approved access here before it ends." />
        {decided.length ? (
          <DecidedRequests requests={decided} />
        ) : (
          <Card>
            <EmptyState icon={CheckCheck} title="No decisions yet" description="Approved, denied and cancelled requests appear here." />
          </Card>
        )}
      </section>
    </div>
  );
}

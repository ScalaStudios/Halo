import { CheckCheck } from "lucide-react";
import { DecidedRequests, PendingRequests } from "@/components/governance/requests";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import type { AccessRequest } from "@/lib/governance-types";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.requests.label };
}

export default async function RequestsPage() {
  const [pending, decided, locale] = await Promise.all([
    apiGet<AccessRequest[]>("/access-requests?status=pending"),
    apiGet<AccessRequest[]>("/access-requests?status=approved,denied,cancelled,expired"),
    requestLocale(),
  ]);
  const t = governanceMessages[locale];
  const yours = pending.filter((r) => r.canDecide).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.requests.label}
        description={t.pages.requestsDescription(pending.length, yours)}
      />
      <Card>
        <CardHeader title={t.pending} description={pending.length ? t.newestFirst : undefined} />
        <CardBody>
          <PendingRequests requests={pending} linkPeople />
        </CardBody>
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.decided} description={t.pages.requestsDecided} />
        {decided.length ? (
          <DecidedRequests requests={decided} />
        ) : (
          <Card>
            <EmptyState icon={CheckCheck} title={t.noDecisions} description={t.pages.requestsEmpty} />
          </Card>
        )}
      </section>
    </div>
  );
}

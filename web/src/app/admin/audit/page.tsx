import { PageHeader } from "@/components/ui/page-header";
import { AuditTable } from "@/components/monitoring/audit-table";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import { requestLocale } from "@/lib/i18n/server";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.audit.label };
}

export default async function AuditPage({ searchParams }: { searchParams: Promise<{ event?: string }> }) {
  const { event } = await searchParams;
  const [audit, users, services, locale] = await Promise.all([
    apiGet<AuditEvent[]>("/audit"),
    apiGet<User[]>("/users"),
    apiGet<ServiceAccount[]>("/service-accounts"),
    requestLocale(),
  ]);
  const names = Object.fromEntries([...users, ...services].map((u) => [u.id, u.name]));
  const people = new Set(audit.flatMap((e) => (e.actorId ? [e.actorId] : []))).size;
  const system = audit.some((e) => e.actorId === null);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.audit.label}
        description={monitoringMessages[locale].audit.description(audit.length, people, system)}
      />
      <AuditTable events={audit} userNames={names} initialEventId={event} />
    </div>
  );
}

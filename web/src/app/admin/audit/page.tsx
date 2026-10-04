import { PageHeader } from "@/components/ui/page-header";
import { AuditTable } from "@/components/monitoring/audit-table";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent, User } from "@/lib/types";

export const metadata = { title: "Audit log" };

export default async function AuditPage() {
  const [audit, users, services] = await Promise.all([
    apiGet<AuditEvent[]>("/audit"),
    apiGet<User[]>("/users"),
    apiGet<ServiceAccount[]>("/service-accounts"),
  ]);
  const names = Object.fromEntries([...users, ...services].map((u) => [u.id, u.name]));
  const people = new Set(audit.flatMap((e) => (e.actorId ? [e.actorId] : []))).size;
  const system = audit.some((e) => e.actorId === null);
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Audit log"
        description={`${pluralize(audit.length, "administrative change")}, made by ${pluralize(people, "administrator or service account", "administrators and service accounts")}${system ? " and Halo itself" : ""}.`}
      />
      <AuditTable events={audit} userNames={names} />
    </div>
  );
}

import { DomainsManager } from "@/components/settings/domains";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Domain } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Domains" };

export default async function DomainsPage() {
  const [domains, me] = await Promise.all([apiGet<Domain[]>("/domains"), apiGet<User>("/me")]);
  const verified = domains.filter((d) => d.verifiedAt).length;
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader title="Domains" description={`${pluralize(verified, "verified domain")}, ${domains.length - verified} waiting. Verify each email domain your organization owns with a DNS TXT record.`} />
      <DomainsManager domains={domains} canEdit={hasRole(me)} />
    </div>
  );
}

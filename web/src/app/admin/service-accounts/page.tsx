import { ServiceAccountsTable } from "@/components/provisioning/service-accounts-table";
import { activeKeys } from "@/components/provisioning/shared";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Service accounts" };

export default async function ServiceAccountsPage() {
  const [accounts, users] = await Promise.all([apiGet<ServiceAccount[]>("/service-accounts"), apiGet<User[]>("/users")]);
  const withKeys = accounts.filter((a) => activeKeys(a.keys).length > 0).length;
  const people = users.filter((u) => u.status !== "deprovisioned").map(({ id, name, email }) => ({ id, name, email }));
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Service accounts"
        description={`${pluralize(accounts.length, "service account")}, ${withKeys} with active API keys. Each one acts with its own roles and appears in the audit log under its own name.`}
      />
      <ServiceAccountsTable accounts={accounts} people={people} />
    </div>
  );
}

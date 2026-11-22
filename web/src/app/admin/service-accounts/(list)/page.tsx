import { ServiceAccountsTable } from "@/components/provisioning/service-accounts-table";
import { activeKeys } from "@/components/provisioning/shared";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { ServiceAccount } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.serviceAccounts.label };
}

export default async function ServiceAccountsPage() {
  const [accounts, users, locale] = await Promise.all([apiGet<ServiceAccount[]>("/service-accounts"), apiGet<User[]>("/users"), requestLocale()]);
  const withKeys = accounts.filter((a) => activeKeys(a.keys).length > 0).length;
  const people = users.filter((u) => u.status !== "deprovisioned").map(({ id, name, email }) => ({ id, name, email }));
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.serviceAccounts.label}
        description={applicationsMessages[locale].serviceAccounts.summary(accounts.length, withKeys)}
      />
      <ServiceAccountsTable accounts={accounts} people={people} />
    </div>
  );
}

import { DomainsManager } from "@/components/settings/domains";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { Domain } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.domains.label };
}

export default async function DomainsPage() {
  const [domains, me, locale] = await Promise.all([apiGet<Domain[]>("/domains"), apiGet<User>("/me"), requestLocale()]);
  const verified = domains.filter((d) => d.verifiedAt).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.domains.label} description={settingsMessages[locale].domains.description(verified, domains.length - verified)} />
      <DomainsManager domains={domains} canEdit={hasRole(me)} />
    </div>
  );
}

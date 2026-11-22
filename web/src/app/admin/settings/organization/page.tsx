import { LogRetentionForm, OrganizationForm } from "@/components/settings/settings-forms";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.organization.label };
}

export default async function OrganizationPage() {
  const [data, me, locale] = await Promise.all([apiGet<SettingsResponse>("/settings"), apiGet<User>("/me"), requestLocale()]);
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.organization.label} description={settingsMessages[locale].organization.description} />
      <OrganizationForm settings={data.settings} defaultName={data.defaultOrganizationName} issuer={data.issuer} canEdit={hasRole(me)} />
      <LogRetentionForm settings={data.settings} canEdit={hasRole(me)} />
    </div>
  );
}

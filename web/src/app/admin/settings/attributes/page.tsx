import { AttributesTable } from "@/components/directory/attributes-table";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { Attribute, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.attributes.label };
}

export default async function AttributesPage() {
  const [attributes, me, locale] = await Promise.all([apiGet<Attribute[]>("/attributes"), apiGet<User>("/me"), requestLocale()]);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.attributes.label} description={settingsMessages[locale].attributes.description} />
      <AttributesTable attributes={attributes} canEdit={hasRole(me)} />
    </div>
  );
}

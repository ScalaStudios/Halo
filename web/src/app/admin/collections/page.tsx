import { CollectionsTable } from "@/components/applications/collections-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Application, Collection, Group } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.collections.label };
}

export default async function CollectionsPage() {
  const [collections, applications, groups, locale] = await Promise.all([
    apiGet<Collection[]>("/collections"),
    apiGet<Application[]>("/applications"),
    apiGet<Group[]>("/groups"),
    requestLocale(),
  ]);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.collections.label}
        description={directoryMessages[locale].collections.description}
      />
      <CollectionsTable collections={collections} applications={applications} groups={groups} />
    </div>
  );
}

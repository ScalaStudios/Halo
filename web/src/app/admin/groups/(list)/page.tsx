import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Group } from "@/lib/types";
import { GroupsTable } from "./groups-table";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.groups.label };
}

export default async function GroupsPage() {
  const [groups, locale] = await Promise.all([apiGet<Group[]>("/groups"), requestLocale()]);
  const dynamic = groups.filter((g) => g.kind === "dynamic").length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.groups.label}
        description={directoryMessages[locale].groups.description(groups.length, dynamic)}
      />
      <GroupsTable groups={groups} />
    </div>
  );
}

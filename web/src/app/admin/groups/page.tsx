import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Group } from "@/lib/types";
import { GroupsTable } from "./groups-table";

export const metadata = { title: "Groups" };

export default async function GroupsPage() {
  const groups = await apiGet<Group[]>("/groups");
  const dynamic = groups.filter((g) => g.kind === "dynamic").length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Groups"
        description={`${groups.length} groups, ${dynamic} of them rule-based. Groups are how people get access to applications.`}
      />
      <GroupsTable groups={groups} />
    </div>
  );
}

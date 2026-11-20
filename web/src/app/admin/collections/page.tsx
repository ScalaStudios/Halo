import { CollectionsTable } from "@/components/applications/collections-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Application, Collection, Group } from "@/lib/types";

export const metadata = { title: "Collections" };

export default async function CollectionsPage() {
  const [collections, applications, groups] = await Promise.all([apiGet<Collection[]>("/collections"), apiGet<Application[]>("/applications"), apiGet<Group[]>("/groups")]);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Collections"
        description="Named sections of the account portal. Each person sees the collections assigned to their groups, with only the applications they can already open."
      />
      <CollectionsTable collections={collections} applications={applications} groups={groups} />
    </div>
  );
}

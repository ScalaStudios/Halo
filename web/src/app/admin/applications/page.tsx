import { ApplicationsTable } from "@/components/applications/applications-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Application } from "@/lib/types";

export const metadata = { title: "Applications" };

export default async function ApplicationsPage({ searchParams }: { searchParams: Promise<{ view?: string }> }) {
  const { view } = await searchParams;
  const applications = await apiGet<Application[]>("/applications");
  const active = applications.filter((a) => a.status === "active").length;
  const signIns = applications.reduce((sum, a) => sum + a.signIns7d, 0);
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Applications"
        description={`${applications.length} applications, ${active} active. ${signIns.toLocaleString("en-US")} sign-ins and token requests in the last 7 days.`}
      />
      <ApplicationsTable applications={applications} initialView={view} />
    </div>
  );
}

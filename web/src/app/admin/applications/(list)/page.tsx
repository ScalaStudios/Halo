import { ApplicationsTable } from "@/components/applications/applications-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Application } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.applications.label };
}

export default async function ApplicationsPage({ searchParams }: { searchParams: Promise<{ view?: string }> }) {
  const { view } = await searchParams;
  const [applications, locale] = await Promise.all([apiGet<Application[]>("/applications"), requestLocale()]);
  const active = applications.filter((a) => a.status === "active").length;
  const signIns = applications.reduce((sum, a) => sum + a.signIns7d, 0);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.applications.label}
        description={applicationsMessages[locale].list.summary(applications.length, active, signIns.toLocaleString(locale))}
      />
      <ApplicationsTable applications={applications} initialView={view} />
    </div>
  );
}

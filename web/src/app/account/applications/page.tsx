import type { Metadata } from "next";
import { AppLauncher } from "@/components/account/app-launcher";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";

export const metadata: Metadata = { title: "Applications" };

export default async function ApplicationsPage() {
  const apps = await apiGet<{ id: string; name: string; description: string; homepage: string; launchUrl: string }[]>("/me/applications");
  return (
    <div className="flex animate-page flex-col gap-12">
      <PageHeader size="large" title="Applications" description="Everything you can open with your Halo account." />
      <AppLauncher apps={apps.map((app) => ({ ...app, host: URL.canParse(app.homepage) ? new URL(app.homepage).host : "" }))} />
    </div>
  );
}

import type { Metadata } from "next";
import { AppLauncher } from "@/components/account/app-launcher";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";

export const metadata: Metadata = { title: "Applications" };

export default async function ApplicationsPage() {
  const [apps, collections] = await Promise.all([
    apiGet<{ id: string; name: string; description: string; homepage: string; launchUrl: string; logoUrl: string | null; favourite: boolean }[]>("/me/applications"),
    apiGet<{ id: string; name: string; applicationIds: string[] }[]>("/me/collections"),
  ]);
  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large" title="Applications" description="Everything you can open with your Halo account." />
      <AppLauncher apps={apps.map((app) => ({ ...app, host: URL.canParse(app.homepage) ? new URL(app.homepage).host : "" }))} collections={collections} />
    </div>
  );
}

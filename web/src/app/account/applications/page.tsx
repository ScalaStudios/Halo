import type { Metadata } from "next";
import { AppAccess } from "@/components/account/app-access";
import { AppLauncher } from "@/components/account/app-launcher";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Consent } from "@/lib/types";

export const metadata: Metadata = { title: "Applications" };

export default async function ApplicationsPage() {
  const [apps, collections, consents] = await Promise.all([
    apiGet<{ id: string; name: string; description: string; homepage: string; launchUrl: string; logoUrl: string | null; favourite: boolean; roles: string[] }[]>("/me/applications"),
    apiGet<{ id: string; name: string; applicationIds: string[] }[]>("/me/collections"),
    apiGet<Consent[]>("/me/consents"),
  ]);
  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large" title="Applications" description="Everything you can open with your Halo account." />
      <AppLauncher apps={apps.map((app) => ({ ...app, host: URL.canParse(app.homepage) ? new URL(app.homepage).host : "" }))} collections={collections} />
      <AppAccess consents={consents} />
    </div>
  );
}

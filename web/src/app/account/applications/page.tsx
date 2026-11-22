import type { Metadata } from "next";
import { AppAccess } from "@/components/account/app-access";
import { AppLauncher } from "@/components/account/app-launcher";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { Consent } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.applications };
}

export default async function ApplicationsPage() {
  const [apps, collections, consents, locale] = await Promise.all([
    apiGet<{ id: string; name: string; description: string; homepage: string; launchUrl: string; logoUrl: string | null; favourite: boolean; roles: string[] }[]>("/me/applications"),
    apiGet<{ id: string; name: string; applicationIds: string[] }[]>("/me/collections"),
    apiGet<Consent[]>("/me/consents"),
    requestLocale(),
  ]);
  const copy = accountMessages(locale);
  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large" title={copy.nav.applications} description={copy.applications.description} />
      <AppLauncher
        apps={apps.map((app) => ({ ...app, host: app.launchUrl.startsWith("/account/") ? copy.applications.sshSession : URL.canParse(app.homepage) ? new URL(app.homepage).host : "" }))}
        collections={collections}
      />
      <AppAccess consents={consents} />
    </div>
  );
}

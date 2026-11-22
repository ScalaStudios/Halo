import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { NewApplicationFlow } from "@/components/applications/new-application-flow";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { requestLocale } from "@/lib/i18n/server";
import type { Outpost } from "@/lib/infra-types";
import type { OrganizationProfile } from "@/lib/settings-types";

export async function generateMetadata() {
  return { title: applicationsMessages[await requestLocale()].list.add };
}

export default async function NewApplicationPage() {
  const [org, locale] = await Promise.all([apiGet<OrganizationProfile>("/organization"), requestLocale()]);
  const t = applicationsMessages[locale];
  const outposts = org.featuresOff.includes("remote-access") ? [] : await apiGet<Outpost[]>("/outposts", []);
  return (
    <div className="flex max-w-3xl flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {t.common.applications}
        </Link>
        <PageHeader title={t.list.add} description={t.list.addDescription} />
      </div>
      <NewApplicationFlow issuer={org.issuer} featuresOff={org.featuresOff} outposts={outposts} />
    </div>
  );
}

import { FeatureSettings } from "@/components/settings/features";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { Feature } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.features.label };
}

export default async function FeaturesPage() {
  const [features, me, locale] = await Promise.all([apiGet<Feature[]>("/features"), apiGet<User>("/me"), requestLocale()]);
  const on = features.filter((f) => f.enabled).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.features.label} description={settingsMessages[locale].features.description(on, features.length)} />
      <FeatureSettings features={features} canEdit={hasRole(me)} />
    </div>
  );
}

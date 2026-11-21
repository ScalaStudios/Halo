import { FeatureSettings } from "@/components/settings/features";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Feature } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Features" };

export default async function FeaturesPage() {
  const [features, me] = await Promise.all([apiGet<Feature[]>("/features"), apiGet<User>("/me")]);
  const on = features.filter((f) => f.enabled).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title="Features"
        description={`${on} of ${features.length} features are on. A feature that is off answers 404 on its routes, stops its background jobs and leaves the console and account portal.`}
      />
      <FeatureSettings features={features} canEdit={hasRole(me)} />
    </div>
  );
}

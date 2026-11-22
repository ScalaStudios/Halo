import { PolicyFormSkeleton } from "@/components/policy/policy-form-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = policyMessages[locale].page;
  return (
    <PageSkeleton back={{ href: "/admin/policies", label: frameMessages[locale].nav.items.policies.label }} title={t.create} description={t.createDescription}>
      <PolicyFormSkeleton />
    </PageSkeleton>
  );
}

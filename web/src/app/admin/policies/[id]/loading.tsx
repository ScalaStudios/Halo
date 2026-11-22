import { PolicyFormSkeleton } from "@/components/policy/policy-form-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  return (
    <PageSkeleton back={{ href: "/admin/policies", label: frameMessages[await requestLocale()].nav.items.policies.label }} description={34}>
      <PolicyFormSkeleton />
    </PageSkeleton>
  );
}

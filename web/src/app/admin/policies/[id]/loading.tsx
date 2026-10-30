import { PolicyFormSkeleton } from "@/components/policy/policy-form-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton back={{ href: "/admin/policies", label: "Policies" }} description={34}>
      <PolicyFormSkeleton />
    </PageSkeleton>
  );
}

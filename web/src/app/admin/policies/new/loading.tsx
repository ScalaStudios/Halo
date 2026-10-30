import { PolicyFormSkeleton } from "@/components/policy/policy-form-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/policies", label: "Policies" }}
      title="Create policy"
      description="New policies start in report-only mode, so you can see what they would do before they affect anyone."
    >
      <PolicyFormSkeleton />
    </PageSkeleton>
  );
}

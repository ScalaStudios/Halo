import { ReviewSkeleton } from "@/components/governance/review-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton back={{ href: "/admin/access-reviews", label: "Access reviews" }} description={50}>
      <ReviewSkeleton />
    </PageSkeleton>
  );
}

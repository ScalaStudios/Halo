import { ReviewSkeleton } from "@/components/governance/review-skeleton";
import { PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton large back={{ href: "/account/access", label: "Access" }} description={110}>
      <ReviewSkeleton />
    </PageSkeleton>
  );
}

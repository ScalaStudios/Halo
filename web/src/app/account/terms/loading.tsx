import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Terms of use"
      description="Terms your organization asks you to accept before you use its applications. When a new version is published, Halo asks you again at your next sign-in."
    >
      <ListSkeleton rows={2} className="overflow-hidden rounded-lg border border-border bg-surface" />
    </PageSkeleton>
  );
}

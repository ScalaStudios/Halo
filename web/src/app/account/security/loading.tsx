import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Security"
      description="How you prove it's you. Keep at least two passkeys or security keys so losing one device doesn't lock you out."
    >
      <div className="flex flex-col gap-12">
        {["Passkeys", "Security keys", "Authenticator apps"].map((title) => (
          <section key={title} className="flex flex-col gap-4">
            <SectionTitle title={title} description={<SkeletonText chars={100} />} action={<Skeleton className="h-8 w-36 rounded-md" />} />
            <ListSkeleton rows={1} media className="overflow-hidden rounded-lg border border-border bg-surface" />
          </section>
        ))}
      </div>
    </PageSkeleton>
  );
}

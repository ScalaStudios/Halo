import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Domains" description={120} className="max-w-4xl">
      <div className="flex flex-col gap-4">
        <SectionTitle title="Verified domains" description="Halo confirmed a TXT record at each of these domains." action={<Skeleton className="h-8 w-32 rounded-md" />} />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
    </PageSkeleton>
  );
}

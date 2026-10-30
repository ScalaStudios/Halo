import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Sign-in logs" description={71}>
      <TableSkeleton columns={8} views={3} facets={4} rows={10} />
    </PageSkeleton>
  );
}

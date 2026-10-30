import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Devices" description={134}>
      <TableSkeleton columns={8} views={4} facets={1} rows={10} />
    </PageSkeleton>
  );
}

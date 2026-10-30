import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Applications" description={78}>
      <TableSkeleton columns={9} views={5} facets={3} rows={10} />
    </PageSkeleton>
  );
}

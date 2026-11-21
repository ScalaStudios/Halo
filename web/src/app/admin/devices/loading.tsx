import { PageSkeleton, TableSkeleton, TabsSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Devices" description={160}>
      <TabsSkeleton labels={["Devices", "Compliance sources"]} />
      <TableSkeleton columns={9} views={5} facets={2} rows={10} />
    </PageSkeleton>
  );
}

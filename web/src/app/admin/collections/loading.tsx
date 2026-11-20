import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Collections" description="Named sections of the account portal. Each person sees the collections assigned to their groups, with only the applications they can already open.">
      <TableSkeleton columns={4} rows={4} />
    </PageSkeleton>
  );
}

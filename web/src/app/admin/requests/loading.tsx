import { Card, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Requests" description={126}>
      <Card>
        <CardHeader title="Pending" description={<SkeletonText chars={12} />} />
        <ListSkeleton media className="pb-2" />
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Decided" description="Approvals, denials and cancellations, newest first. Revoke approved access here before it ends." />
        <TableSkeleton columns={5} facets={2} rows={4} />
      </section>
    </PageSkeleton>
  );
}

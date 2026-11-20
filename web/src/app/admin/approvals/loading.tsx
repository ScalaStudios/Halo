import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, ListSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Approvals" description={140}>
      <Card>
        <CardHeader title="Pending" description={<SkeletonText chars={12} />} />
        <ListSkeleton media className="pb-2" />
      </Card>
      <Card>
        <CardHeader
          title="Changes that need approval"
          description="Each one waits for a second global administrator, who approves or rejects it here within 72 hours. The person who asked can't approve their own change."
        />
        <CardBody className="flex max-w-xl flex-col gap-6">
          <div aria-hidden="true" className="flex flex-col gap-4">
            {[38, 34, 28, 30].map((chars) => (
              <div key={chars} className="flex items-start gap-3">
                <Skeleton className="mt-0.5 size-4 rounded-sm" />
                <span className="flex flex-col">
                  <span className="text-body-sm">
                    <SkeletonText chars={chars} />
                  </span>
                  <span className="text-caption">
                    <SkeletonText chars={chars + 24} />
                  </span>
                </span>
              </div>
            ))}
          </div>
          <FieldSkeleton label="Emergency access accounts" hint={140} />
        </CardBody>
      </Card>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Decided" description="Approved, rejected, expired and unapplied changes, newest first." />
        <TableSkeleton columns={4} facets={1} rows={4} />
      </section>
    </PageSkeleton>
  );
}

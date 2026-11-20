import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Security recommendations" description={160} className="max-w-4xl">
      <Card>
        <CardHeader title="Needs action" description={<SkeletonText chars={26} />} />
        <ListSkeleton media className="border-t border-border" />
      </Card>
      <Card>
        <CardHeader title="Passing" />
        <ListSkeleton media rows={4} className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}

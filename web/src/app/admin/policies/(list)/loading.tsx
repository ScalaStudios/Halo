import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, Skeleton, TabsSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Policies"
      description={140}
      actions={
        <>
          <Skeleton className="h-8 w-36 rounded-md" />
          <Skeleton className="h-8 w-32 rounded-md" />
        </>
      }
    >
      <TabsSkeleton labels={["Policies", "Networks"]} />
      <Card>
        <CardHeader title="Report-only insights" description="What report-only policies would have done in the last 24 hours. None of these sign-ins were changed." />
        <ListSkeleton rows={2} className="border-t border-border" />
      </Card>
      <Card>
        <ListSkeleton />
      </Card>
    </PageSkeleton>
  );
}

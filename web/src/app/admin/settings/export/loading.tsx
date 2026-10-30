import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Data export"
      description="Download your directory, applications and policies as one JSON file, for backups or a move to another system."
      className="max-w-4xl"
    >
      <Card>
        <CardHeader title="What the export contains" description="Halo streams the file as it reads the database, so large directories download without waiting." />
        <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
          {[6, 3].map((items, i) => (
            <div key={i} aria-hidden="true" className="flex flex-col gap-3">
              <p className="text-label">
                <SkeletonText chars={14} />
              </p>
              <div className="flex flex-col gap-2">
                {Array.from({ length: items }, (_, item) => (
                  <p key={item} className="text-body-sm">
                    <SkeletonText chars={[48, 40, 34][item % 3]} />
                  </p>
                ))}
              </div>
            </div>
          ))}
        </CardBody>
      </Card>
      <Skeleton className="h-10 w-44 rounded-md" />
    </PageSkeleton>
  );
}

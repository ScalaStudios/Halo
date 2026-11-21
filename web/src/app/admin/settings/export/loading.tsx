import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

function Lists({ counts }: { counts: number[] }) {
  return counts.map((items, i) => (
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
  ));
}

export default function Loading() {
  return (
    <PageSkeleton
      title="Export and import"
      description="Download your directory, applications and policies as one JSON file for backups or a move to another system, and bring configuration back from one."
      className="max-w-4xl"
    >
      <Card>
        <CardHeader title="What the export contains" description="Halo streams the file as it reads the database, so large directories download without waiting." />
        <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
          <Lists counts={[6, 3]} />
        </CardBody>
      </Card>
      <Skeleton className="h-10 w-44 rounded-md" />
      <Card>
        <CardHeader
          title="Import configuration"
          description="Bring this installation in line with an export from it or from another Halo. Objects are matched by id, then by name, and you see every change before anything is saved."
        />
        <CardBody className="flex flex-col gap-8">
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <Lists counts={[4, 3]} />
          </div>
          <Skeleton className="h-10 w-48 rounded-md" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title="Scheduled exports"
          description="Save the same JSON export to your S3-compatible bucket on a schedule, as exports/halo-export-<time>.json, so a recent copy is there to import."
        />
        <CardBody className="flex flex-col gap-6">
          {["h-10 w-60 rounded-md", "h-10 w-32 rounded-md"].map((size) => (
            <div key={size} aria-hidden="true" className="flex flex-col gap-2">
              <p className="text-label">
                <SkeletonText chars={16} />
              </p>
              <Skeleton className={size} />
            </div>
          ))}
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}

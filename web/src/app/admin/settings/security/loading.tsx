import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Security defaults" description={150} className="max-w-4xl">
      {[1, 2, 1, 3].map((fields, i) => (
        <Card key={i}>
          <CardHeader title={<SkeletonText chars={12} />} description={<SkeletonText chars={80} />} />
          <CardBody className="flex flex-wrap gap-6">
            {Array.from({ length: fields }, (_, field) => (
              <div key={field} aria-hidden="true" className="flex w-60 flex-col gap-2">
                <p className="text-label">
                  <SkeletonText chars={22} />
                </p>
                <Skeleton className="h-10 w-32 rounded-md" />
                <p className="text-caption">
                  <SkeletonText chars={60} />
                </p>
              </div>
            ))}
          </CardBody>
        </Card>
      ))}
      <Card>
        <CardHeader title="Signing keys" description="The keys Halo signs OpenID Connect tokens and SAML responses with. Every rotation is recorded in the audit log." />
        <ListSkeleton rows={2} className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}

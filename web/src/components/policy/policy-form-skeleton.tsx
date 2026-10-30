import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

export function PolicyFormSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
      <div className="flex min-w-0 flex-col gap-6 xl:col-span-8">
        <Card>
          <CardHeader title="Details" />
          <CardBody className="flex flex-col gap-6">
            <FieldSkeleton label="Name" />
            <FieldSkeleton label="Description" hint="Optional. Say why the policy exists so other administrators know whether they can change it." tall />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="People" description="Who the policy applies to. Exclusions always win." />
          <CardBody className="flex flex-col gap-6">
            <FieldSkeleton label="Applies to" />
            <div className="h-40 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Applications" description="Where the sign-in goes. Halo itself counts as an application." />
          <CardBody>
            <FieldSkeleton />
          </CardBody>
        </Card>
      </div>
      <div className="xl:col-span-4">
        <Card>
          <CardHeader title="Summary" actions={<Skeleton className="h-6 w-24 rounded-full" />} />
          <CardBody className="flex flex-col gap-4">
            <p className="text-body">
              <SkeletonText chars={60} />
            </p>
            <div className="flex gap-2">
              <Skeleton className="h-10 w-32 rounded-md" />
              <Skeleton className="h-10 w-24 rounded-md" />
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

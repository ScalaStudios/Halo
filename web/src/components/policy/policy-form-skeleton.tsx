import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export async function PolicyFormSkeleton() {
  const copy = policyMessages[await requestLocale()];
  const t = copy.form;
  return (
    <div className="grid grid-cols-1 gap-8 xl:grid-cols-12">
      <div className="flex min-w-0 flex-col gap-6 xl:col-span-8">
        <Card>
          <CardHeader title={t.details} />
          <CardBody className="flex flex-col gap-6">
            <FieldSkeleton label={copy.name} />
            <FieldSkeleton label={t.description} hint={t.descriptionHint} tall />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t.people} description={t.peopleDescription} />
          <CardBody className="flex flex-col gap-6">
            <FieldSkeleton label={t.appliesTo} />
            <div className="h-40 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title={t.applications} description={t.applicationsDescription} />
          <CardBody>
            <FieldSkeleton />
          </CardBody>
        </Card>
      </div>
      <div className="xl:col-span-4">
        <Card>
          <CardHeader title={t.summary} actions={<Skeleton className="h-6 w-24 rounded-full" />} />
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

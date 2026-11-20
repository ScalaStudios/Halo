import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Organization"
      description="The name and contact address people see when they sign in, the issuer URL your applications trust, and how long Halo keeps its logs."
      className="max-w-4xl"
    >
      <Card>
        <CardHeader title="Profile" description="How Halo names your organization to the people who sign in." />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label="Organization name" hint={110} />
            <FieldSkeleton label="Contact email" hint="The sign-in page links to this address under Get help. Leave empty to hide the link." />
            <FieldSkeleton label="Issuer URL" hint="Set by HALO_PUBLIC_URL when Halo starts. Applications discover every protocol endpoint from it." />
            <Skeleton className="h-10 w-44 rounded-md" />
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Log retention" description="How long Halo keeps sign-in and audit log entries. Once a day it deletes older entries and records how many in the audit log." />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label="Sign-in log" hint={90} />
            <FieldSkeleton label="Audit log" hint={60} />
            <Skeleton className="h-10 w-40 rounded-md" />
          </div>
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}

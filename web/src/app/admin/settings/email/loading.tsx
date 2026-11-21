import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton, TabsSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Email templates"
      description="The subject and text of the emails Halo sends for invitations, resets, sign-in, access requests and security notices. Every email ends with Halo and your organization name."
      className="max-w-6xl"
    >
      <div className="flex flex-col gap-6">
        <TabsSkeleton labels={["Invitation", "Authentication reset", "Sign-in link", "Sign-in code", "Access request", "Access approved", "Access denied"]} />
        <Card>
          <CardHeader title="Invitation" description="Sent when an administrator invites someone to set up their account." />
          <CardBody>
            <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
              <div className="flex flex-col gap-6">
                <FieldSkeleton label="Subject" />
                <FieldSkeleton label="Body" hint={90} tall />
              </div>
              <Skeleton className="h-80 rounded-lg" />
            </div>
          </CardBody>
        </Card>
      </div>
    </PageSkeleton>
  );
}

import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { DetailHeaderSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TabsSkeleton } from "@/components/ui/skeleton";

const facts = ["Status", "Authentication strength", "Admin role", "Last sign-in", "Source", "Member since"];
const profile = ["Email", "Title", "Department", "Location", "Manager", "User ID"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/users", label: "Users" }}
      heading={
        <>
          <DetailHeaderSkeleton media="avatar" actions={3} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <TabsSkeleton labels={["Overview", "Authentication", "Access", "Applications", "Groups", "Devices", "Sessions", "Activity"]} />
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="xl:col-span-7">
          <CardHeader title="Profile" />
          <CardBody>
            <DescriptionList columns={2} items={profile.map((label) => ({ label, value: <SkeletonText chars={18} /> }))} />
          </CardBody>
        </Card>
        <Card className="xl:col-span-5">
          <CardHeader title="Authentication" />
          <CardBody className="flex flex-col gap-4">
            <p className="text-body-sm">
              <SkeletonText chars={70} />
            </p>
            <p className="text-body-sm">
              <SkeletonText chars={40} />
            </p>
          </CardBody>
        </Card>
        <Card className="xl:col-span-12">
          <CardHeader title="Recent sign-ins" />
          <ListSkeleton className="border-t border-border" />
        </Card>
      </div>
    </PageSkeleton>
  );
}

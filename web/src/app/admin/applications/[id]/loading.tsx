import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { DetailHeaderSkeleton, FieldSkeleton, PageSkeleton, SkeletonText, TabsSkeleton } from "@/components/ui/skeleton";

const facts = ["Protocol", "Type", "Status", "Users", "Sign-ins (7 days)", "Owner"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/applications", label: "Applications" }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} className="2xl:flex-row 2xl:items-center 2xl:justify-between" />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <TabsSkeleton labels={["Overview", "Authentication", "Permissions", "Claims", "Users & groups", "Credentials", "Sessions", "Logs"]} />
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title="Connection details" description={<SkeletonText chars={60} />} />
          <CardBody className="flex flex-col gap-4">
            {[0, 1, 2, 3].map((i) => (
              <FieldSkeleton key={i} />
            ))}
          </CardBody>
        </Card>
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader title={<SkeletonText chars={24} />} description={<SkeletonText chars={200} />} />
          <CardBody>
            <div className="h-40 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
      </div>
    </PageSkeleton>
  );
}

import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

const facts = ["Status", "Owner", "Active keys", "Last used", "Created", "Service account ID"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/service-accounts", label: "Service accounts" }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title="API keys" description="Send a key as Authorization: Bearer <key>. The api scope opens the management API; the scim scope opens SCIM provisioning." />
        <TableSkeleton simple columns={7} rows={2} />
      </section>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title="Roles" description="What requests with this service account's keys may change." />
          <CardBody>
            <div className="h-20 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader title="Activity" description={<SkeletonText chars={50} />} />
          <ListSkeleton className="border-t border-border" />
        </Card>
      </div>
    </PageSkeleton>
  );
}

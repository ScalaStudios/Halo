import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

const facts = ["Events", "Last delivery", "Created", "Endpoint ID"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/webhooks", label: "Webhooks" }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title="Deliveries" description={<SkeletonText chars={110} />} />
        <TableSkeleton simple columns={6} rows={3} />
      </section>
      <Card>
        <CardHeader
          title="Verify the signature"
          description="Every request carries Halo-Signature: t=<unix time>,v1=<hex HMAC-SHA256 of t, a dot and the raw body, keyed with the signing secret>. Reject requests with a wrong signature or a timestamp more than 5 minutes old, and use the id field to ignore repeats."
        />
        <CardBody>
          <div className="h-96 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}

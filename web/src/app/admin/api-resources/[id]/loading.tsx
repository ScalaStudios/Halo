import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

const facts = ["Access token lifetime", "Applications", "Created"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/api-resources", label: "API resources" }}
      heading={
        <>
          <DetailHeaderSkeleton actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <div className="flex flex-col gap-6">
        <SectionTitle title="Scopes" description="What the API lets callers do. Tokens list the granted scopes in the scope claim, separated by spaces." />
        <TableSkeleton simple columns={3} rows={2} />
      </div>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Applications"
          description="Applications granted scopes get JWT access tokens with this API's identifier in aud when they request a granted scope or send resource=<identifier>."
          action={<Skeleton className="h-8 w-32 rounded-md" />}
        />
        <TableSkeleton simple columns={3} rows={1} />
      </div>
      <Card>
        <CardHeader title="Validate tokens in the API" description="Access tokens for this API are JWTs signed by Halo. The API validates them itself, without calling Halo on each request." />
        <CardBody>
          <div className="h-40 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}

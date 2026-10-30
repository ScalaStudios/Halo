import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, PageSkeleton, Skeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Infrastructure access" description={110}>
      <Card>
        <CardHeader title="Certificate authority" description="Servers that trust this key accept any certificate Halo signs, for the principals written into it." />
        <CardBody className="flex flex-col gap-6">
          <FieldSkeleton label="CA public key" hint={60} />
          <DescriptionList
            columns={2}
            items={[
              { label: "Fingerprint", value: <SkeletonText chars={52} /> },
              { label: "Created", value: <SkeletonText chars={14} /> },
            ]}
          />
          <FieldSkeleton label="Certificate lifetime" hint={70} />
          <div className="h-16 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title="Trust Halo on your servers"
          description="Run these commands on each server once. People then sign in with halo login, request a certificate with halo ssh-cert and connect with plain ssh."
        />
        <CardBody>
          <div className="h-36 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Principal mappings"
          description="Members of a group may log in as these unix accounts. A certificate lists every principal from every group the person belongs to."
          action={<Skeleton className="h-8 w-32 rounded-md" />}
        />
        <TableSkeleton simple columns={4} rows={2} />
      </div>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Issued certificates"
          description="The 200 most recent certificates. Each key ID is halo:<user id>:<serial>, which sshd writes to its log on every login."
        />
        <TableSkeleton simple columns={6} rows={3} />
      </div>
    </PageSkeleton>
  );
}

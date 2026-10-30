import Link from "next/link";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Provisioning"
      description="SCIM 2.0 in both directions: your HR system or identity provider keeps people in Halo current, and Halo keeps accounts in your applications current."
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title="Inbound" description="Workday, BambooHR, Okta or Microsoft Entra ID create, update and deactivate people and groups in Halo." />
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
          <Card className="min-w-0 xl:col-span-5">
            <CardHeader title="Connection details" description="Enter these in the provisioning settings of the source system." />
            <CardBody className="flex flex-col gap-6">
              <FieldSkeleton label="SCIM base URL" />
              <DescriptionList
                items={[
                  { label: "Authentication", value: "Bearer token: an API key with the scim scope" },
                  { label: "Required role", value: "User administrator on the key's service account" },
                  { label: "Supported", value: "Users and assigned groups, PATCH, filters on userName, externalId and emails, enterprise department and manager" },
                ]}
              />
            </CardBody>
          </Card>
          <Card className="min-w-0 xl:col-span-7">
            <CardHeader
              title="Connectors"
              description="Service accounts with an active SCIM key."
              actions={
                <Link href="/admin/service-accounts" className={buttonClasses("quiet", "sm")}>
                  Manage
                </Link>
              }
            />
            <ListSkeleton rows={2} className="border-t border-border" />
          </Card>
        </div>
        <Card>
          <CardHeader
            title="Recent SCIM changes"
            description="The latest changes connectors made in Halo."
            actions={
              <Link href="/admin/audit" className={buttonClasses("quiet", "sm")}>
                Audit log
              </Link>
            }
          />
          <ListSkeleton rows={4} className="border-t border-border" />
        </Card>
      </section>
      <section className="flex flex-col gap-6">
        <SectionTitle title="Outbound" description={<SkeletonText chars={190} />} />
        <TableSkeleton simple columns={5} rows={6} />
      </section>
    </PageSkeleton>
  );
}

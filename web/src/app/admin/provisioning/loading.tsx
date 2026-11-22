import Link from "next/link";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { FieldSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const nav = frameMessages[locale].nav.items;
  const t = provisioningMessages[locale];
  return (
    <PageSkeleton title={nav.provisioning.label} description={t.page.description}>
      <section className="flex flex-col gap-6">
        <SectionTitle title={t.page.inbound} description={t.page.inboundHint} />
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
          <Card className="min-w-0 xl:col-span-5">
            <CardHeader title={t.page.connection} description={t.page.connectionHint} />
            <CardBody className="flex flex-col gap-6">
              <FieldSkeleton label={t.page.scimBaseUrl} />
              <DescriptionList
                items={[
                  { label: t.page.authentication, value: t.page.authenticationValue },
                  { label: t.page.requiredRole, value: t.page.requiredRoleValue },
                  { label: t.page.supported, value: t.page.supportedValue },
                ]}
              />
            </CardBody>
          </Card>
          <Card className="min-w-0 xl:col-span-7">
            <CardHeader
              title={t.page.connectors}
              description={t.page.connectorsHint}
              actions={
                <Link href="/admin/service-accounts" className={buttonClasses("quiet", "sm")}>
                  {t.page.manage}
                </Link>
              }
            />
            <ListSkeleton rows={2} className="border-t border-border" />
          </Card>
        </div>
        <Card>
          <CardHeader
            title={t.page.recent}
            description={t.page.recentHint}
            actions={
              <Link href="/admin/audit" className={buttonClasses("quiet", "sm")}>
                {nav.audit.label}
              </Link>
            }
          />
          <ListSkeleton rows={4} className="border-t border-border" />
        </Card>
      </section>
      <section className="flex flex-col gap-6">
        <SectionTitle
          title={t.ldap.heading}
          description={t.ldap.headingHint}
        />
        <TableSkeleton simple columns={5} rows={1} />
      </section>
      <section className="flex flex-col gap-6">
        <SectionTitle title={t.page.outbound} description={<SkeletonText chars={190} />} />
        <TableSkeleton simple columns={5} rows={6} />
      </section>
    </PageSkeleton>
  );
}

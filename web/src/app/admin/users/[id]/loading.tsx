import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { DetailHeaderSkeleton, ListSkeleton, PageSkeleton, SkeletonText, TabsSkeleton } from "@/components/ui/skeleton";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = directoryMessages[locale];
  const facts = [t.fields.status, t.user.authenticationStrength, t.fields.adminRole, t.fields.lastSignIn, t.fields.source, t.user.memberSince];
  const profile = [t.fields.email, t.fields.title, t.fields.department, t.fields.location, t.fields.manager, t.fields.userId];
  return (
    <PageSkeleton
      back={{ href: "/admin/users", label: frameMessages[locale].nav.items.users.label }}
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
      <TabsSkeleton labels={Object.values(t.user.tabs)} />
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="xl:col-span-7">
          <CardHeader title={t.user.profile} />
          <CardBody>
            <DescriptionList columns={2} items={profile.map((label) => ({ label, value: <SkeletonText chars={18} /> }))} />
          </CardBody>
        </Card>
        <Card className="xl:col-span-5">
          <CardHeader title={t.fields.authentication} />
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
          <CardHeader title={t.user.recentSignIns} />
          <ListSkeleton className="border-t border-border" />
        </Card>
      </div>
    </PageSkeleton>
  );
}

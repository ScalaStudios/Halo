import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const copy = accountMessages(await requestLocale());
  return (
    <PageSkeleton large title={copy.nav.security} description={copy.security.description}>
      <div className="flex flex-col gap-12">
        {Object.values(copy.security.sections).map(({ title }) => (
          <section key={title} className="flex flex-col gap-4">
            <SectionTitle title={title} description={<SkeletonText chars={100} />} action={<Skeleton className="h-8 w-36 rounded-md" />} />
            <ListSkeleton rows={1} media className="overflow-hidden rounded-lg border border-border bg-surface" />
          </section>
        ))}
      </div>
    </PageSkeleton>
  );
}

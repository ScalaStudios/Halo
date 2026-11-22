import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const t = accountMessages(await requestLocale()).terms;
  return (
    <PageSkeleton large title={t.title} description={t.description}>
      <ListSkeleton rows={2} className="overflow-hidden rounded-lg border border-border bg-surface" />
    </PageSkeleton>
  );
}

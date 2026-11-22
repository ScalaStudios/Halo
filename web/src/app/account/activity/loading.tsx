import { Card, CardHeader } from "@/components/ui/card";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const copy = accountMessages(await requestLocale());
  const t = copy.activity;
  return (
    <PageSkeleton large title={copy.nav.activity} description={t.description}>
      <Card>
        <CardHeader title={t.recent} description={t.recentHint} />
        <ListSkeleton className="border-t border-border" />
      </Card>
      <Card>
        <CardHeader title={t.events} description={t.eventsHint} />
        <ListSkeleton rows={4} media className="border-t border-border" />
      </Card>
    </PageSkeleton>
  );
}

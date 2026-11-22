import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const copy = accountMessages(await requestLocale());
  const t = copy.groups;
  const sections = [
    { title: t.yours, description: t.yoursHint },
    { title: t.joinable, description: t.joinableHint },
  ];
  return (
    <PageSkeleton large title={copy.nav.groups} description={t.description}>
      {sections.map((section) => (
        <section key={section.title} className="flex flex-col gap-4">
          <SectionTitle title={section.title} description={section.description} />
          <ListSkeleton rows={2} className="overflow-hidden rounded-lg border border-border bg-surface" />
        </section>
      ))}
    </PageSkeleton>
  );
}

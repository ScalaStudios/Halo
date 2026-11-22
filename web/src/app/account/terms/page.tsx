import type { Metadata } from "next";
import { FileCheck } from "lucide-react";
import { TermsToAccept } from "@/components/account/terms-to-accept";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDateTime } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { Terms, TermsAcceptance } from "@/lib/policy-types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).terms.title };
}

export default async function TermsPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  const { next } = await searchParams;
  const [{ terms, accepted }, locale] = await Promise.all([apiGet<{ terms: (Terms & { acceptedAt: string | null })[]; accepted: TermsAcceptance[] }>("/me/terms"), requestLocale()]);
  const t = accountMessages(locale).terms;
  const pending = terms.filter((t) => !t.acceptedAt);

  return (
    <div className="flex flex-col gap-12">
      <PageHeader
        size="large"
        title={t.title}
        description={t.description}
      />
      {pending.length ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title={t.toAccept} description={t.toAcceptHint} />
          <TermsToAccept terms={pending} next={next} />
        </section>
      ) : (
        <TermsToAccept terms={[]} next={next} />
      )}
      {accepted.length ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title={t.accepted} description={t.acceptedHint} />
          <SimpleTable
            caption={t.caption}
            head={[t.terms, t.version, t.accepted]}
            rows={accepted.map((a) => [
              <span key="name" className="font-medium text-fg">
                {a.name}
              </span>,
              <span key="version" className="tabular-nums">
                {a.version}
              </span>,
              <time key="time" dateTime={a.acceptedAt} className="whitespace-nowrap">
                {formatDateTime(a.acceptedAt, locale)}
              </time>,
            ])}
          />
        </section>
      ) : null}
      {!pending.length && !accepted.length ? (
        <Card>
          <EmptyState icon={FileCheck} title={t.empty} description={t.emptyHint} />
        </Card>
      ) : null}
    </div>
  );
}

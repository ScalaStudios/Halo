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
import type { Terms, TermsAcceptance } from "@/lib/policy-types";

export const metadata: Metadata = { title: "Terms of use" };

export default async function TermsPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  const { next } = await searchParams;
  const { terms, accepted } = await apiGet<{ terms: (Terms & { acceptedAt: string | null })[]; accepted: TermsAcceptance[] }>("/me/terms");
  const pending = terms.filter((t) => !t.acceptedAt);

  return (
    <div className="flex flex-col gap-12">
      <PageHeader
        size="large"
        title="Terms of use"
        description="Terms your organization asks you to accept before you use its applications. When a new version is published, Halo asks you again at your next sign-in."
      />
      {pending.length ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title="To accept" description="Read each document, then accept. Halo keeps a record of the version you accept and when." />
          <TermsToAccept terms={pending} next={next} />
        </section>
      ) : (
        <TermsToAccept terms={[]} next={next} />
      )}
      {accepted.length ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title="Accepted" description="Every version you accepted. Your administrators can include this record in the organization export." />
          <SimpleTable
            caption="Accepted terms of use"
            head={["Terms", "Version", "Accepted"]}
            rows={accepted.map((a) => [
              <span key="name" className="font-medium text-fg">
                {a.name}
              </span>,
              <span key="version" className="tabular-nums">
                {a.version}
              </span>,
              <time key="time" dateTime={a.acceptedAt} className="whitespace-nowrap">
                {formatDateTime(a.acceptedAt)}
              </time>,
            ])}
          />
        </section>
      ) : null}
      {!pending.length && !accepted.length ? (
        <Card>
          <EmptyState icon={FileCheck} title="No terms to accept" description="Your organization doesn't ask you to accept any terms of use." />
        </Card>
      ) : null}
    </div>
  );
}

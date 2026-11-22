import type { Metadata } from "next";
import { PageHeader } from "@/components/ui/page-header";
import { SessionList } from "@/components/identity/session-list";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { Session } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.sessions };
}

export default async function SessionsPage() {
  const [sessions, locale] = await Promise.all([apiGet<Session[]>("/me/sessions"), requestLocale()]);
  const copy = accountMessages(locale);
  return (
    <div lang={locale} className="flex flex-col gap-12">
      <PageHeader size="large"
        title={copy.nav.sessions}
        description={copy.sessions.intro}
      />
      <SessionList locale={locale} sessions={sessions} />
    </div>
  );
}

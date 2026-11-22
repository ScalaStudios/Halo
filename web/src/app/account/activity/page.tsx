import type { Metadata } from "next";
import { LogIn } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SignInRows } from "@/components/monitoring/sign-in-rows";
import { Card, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import { requestLocale } from "@/lib/i18n/server";
import { METHOD } from "@/lib/labels";
import type { SignInEvent, User } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.activity };
}

export default async function ActivityPage() {
  const [user, signIns, apps, locale] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<SignInEvent[]>("/me/sign-ins"),
    apiGet<{ id: string; name: string }[]>("/me/applications"),
    requestLocale(),
  ]);
  const copy = accountMessages(locale);
  const t = copy.activity;
  const events = [...user.methods].sort((a, b) => b.addedAt.localeCompare(a.addedAt));

  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large"
        title={copy.nav.activity}
        description={t.description}
      />

      <Card>
        <CardHeader title={t.recent} description={t.recentHint} />
        {signIns.length > 0 ? (
          <SignInRows events={signIns} appNames={Object.fromEntries(apps.map((app) => [app.id, app.name]))} />
        ) : (
          <EmptyState icon={LogIn} title={t.empty} description={t.emptyHint} className="py-8" />
        )}
      </Card>

      <Card>
        <CardHeader title={t.events} description={t.eventsHint} />
        <ul className="border-t border-border">
          {events.map((method) => {
            const Icon = METHOD[method.kind].icon;
            const recovery = method.kind === "recovery-codes";
            return (
              <li key={method.id} className="flex items-center gap-4 border-t border-border px-6 py-4 first:border-t-0">
                <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                <div className="flex min-w-0 flex-1 flex-col">
                  <p className="text-body-sm font-semibold text-fg">{recovery ? t.recoveryGenerated : t.added(monitoringMessages[locale].methods[method.kind])}</p>
                  <p className="truncate text-body-sm text-fg-3">{recovery ? t.recoveryDetail : method.label}</p>
                </div>
                <span className="tnum shrink-0 text-body-sm text-fg-3">{formatDate(method.addedAt, locale)}</span>
              </li>
            );
          })}
        </ul>
      </Card>
    </div>
  );
}

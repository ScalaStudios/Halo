import { PageHeader } from "@/components/ui/page-header";
import { SignInsTable } from "@/components/monitoring/sign-ins-table";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { Application, SignInEvent, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.signIns.label };
}

export default async function SignInsPage({ searchParams }: { searchParams: Promise<{ event?: string; view?: string }> }) {
  const { event, view } = await searchParams;
  const [signIns, users, applications, locale] = await Promise.all([
    apiGet<SignInEvent[]>("/sign-ins"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
    requestLocale(),
  ]);
  const failed = signIns.filter((e) => e.result === "failure" || e.result === "interrupted").length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.signIns.label}
        description={policyMessages[locale].signIns.summary(signIns.length, failed)}
      />
      <SignInsTable
        events={signIns}
        userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))}
        appNames={Object.fromEntries(applications.map((a) => [a.id, a.name]))}
        initialEventId={event}
        initialView={view}
      />
    </div>
  );
}

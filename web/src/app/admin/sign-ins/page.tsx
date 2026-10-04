import { PageHeader } from "@/components/ui/page-header";
import { SignInsTable } from "@/components/monitoring/sign-ins-table";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Application, SignInEvent, User } from "@/lib/types";

export const metadata = { title: "Sign-in logs" };

export default async function SignInsPage({ searchParams }: { searchParams: Promise<{ event?: string; view?: string }> }) {
  const { event, view } = await searchParams;
  const [signIns, users, applications] = await Promise.all([
    apiGet<SignInEvent[]>("/sign-ins"),
    apiGet<User[]>("/users"),
    apiGet<Application[]>("/applications"),
  ]);
  const failed = signIns.filter((e) => e.result !== "success").length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Sign-in logs"
        description={`${pluralize(signIns.length, "authentication attempt")}, newest first. ${failed} failed or were interrupted.`}
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

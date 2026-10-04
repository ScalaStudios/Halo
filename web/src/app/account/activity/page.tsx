import type { Metadata } from "next";
import { LogIn } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SignInRows } from "@/components/monitoring/sign-in-rows";
import { Card, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { formatDate } from "@/lib/format";
import { apiGet } from "@/lib/api/server";
import { METHOD } from "@/lib/labels";
import type { SignInEvent, User } from "@/lib/types";

export const metadata: Metadata = { title: "Activity" };

export default async function ActivityPage() {
  const [user, signIns, apps] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<SignInEvent[]>("/me/sign-ins"),
    apiGet<{ id: string; name: string }[]>("/me/applications"),
  ]);
  const events = [...user.methods].sort((a, b) => b.addedAt.localeCompare(a.addedAt));

  return (
    <div className="flex animate-page flex-col gap-12">
      <PageHeader size="large"
        title="Activity"
        description="Recent sign-ins and changes to how you sign in. If something looks unfamiliar, sign out of that session and remove the method."
      />

      <Card>
        <CardHeader title="Recent sign-ins" description="Your latest sign-ins to Halo and the applications it protects, newest first." />
        {signIns.length > 0 ? (
          <SignInRows events={signIns} appNames={Object.fromEntries(apps.map((app) => [app.id, app.name]))} />
        ) : (
          <EmptyState icon={LogIn} title="No sign-ins yet" description="Each time you sign in, it appears here." className="py-8" />
        )}
      </Card>

      <Card>
        <CardHeader title="Security events" description="Methods added to your account and recovery codes generated." />
        <ul className="border-t border-border">
          {events.map((method) => {
            const Icon = METHOD[method.kind].icon;
            const recovery = method.kind === "recovery-codes";
            return (
              <li key={method.id} className="flex items-center gap-4 border-t border-border px-6 py-4 first:border-t-0">
                <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                <div className="flex min-w-0 flex-1 flex-col">
                  <p className="text-body-sm font-semibold text-fg">{recovery ? "Recovery codes generated" : `${METHOD[method.kind].label} added`}</p>
                  <p className="truncate text-body-sm text-fg-3">{recovery ? "10 codes, each usable once" : method.label}</p>
                </div>
                <span className="tnum shrink-0 text-body-sm text-fg-3">{formatDate(method.addedAt)}</span>
              </li>
            );
          })}
        </ul>
      </Card>
    </div>
  );
}

import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { SignUpQueue } from "@/components/directory/sign-up-queue";
import { UsersTable } from "@/components/directory/users-table";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { SettingsResponse, SignUp } from "@/lib/settings-types";
import type { Group, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.users.label };
}

export default async function UsersPage({ searchParams }: { searchParams: Promise<{ view?: string; invite?: string }> }) {
  const { view, invite } = await searchParams;
  const locale = await requestLocale();
  const t = directoryMessages[locale];
  const [users, groups, me, signUps, response] = await Promise.all([
    apiGet<User[]>("/users"),
    apiGet<Group[]>("/groups"),
    apiGet<User>("/me"),
    apiGet<SignUp[]>("/sign-ups", []),
    apiGet<SettingsResponse | null>("/settings", null),
  ]);
  const active = users.filter((u) => u.status === "active").length;
  const assigned = groups.filter((g) => g.kind === "assigned");
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.users.label} description={t.users.description(users.length, active)} />
      {signUps.length && response ? (
        <SignUpQueue
          signUps={signUps}
          groups={assigned.filter((g) => response.settings.signUpGroupIds.includes(g.id))}
          canDecide={hasRole(me, "user_admin")}
          proofingRequired={
            response.settings.identityProofingSignUps &&
            (response.settings.identityProofingGroupIds.length === 0 || response.settings.identityProofingGroupIds.some((g) => response.settings.signUpGroupIds.includes(g)))
          }
        />
      ) : null}
      <UsersTable users={users} groups={assigned} initialView={view} inviteOpen={invite === "1"} canGrantRoles={hasRole(me)} />
    </div>
  );
}

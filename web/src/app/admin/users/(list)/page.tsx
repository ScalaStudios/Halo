import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { SignUpQueue } from "@/components/directory/sign-up-queue";
import { UsersTable } from "@/components/directory/users-table";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { SettingsResponse, SignUp } from "@/lib/settings-types";
import type { Group, User } from "@/lib/types";

export const metadata = { title: "Users" };

export default async function UsersPage({ searchParams }: { searchParams: Promise<{ view?: string; invite?: string }> }) {
  const { view, invite } = await searchParams;
  const [users, groups, me, signUps, { settings }] = await Promise.all([
    apiGet<User[]>("/users"),
    apiGet<Group[]>("/groups"),
    apiGet<User>("/me"),
    apiGet<SignUp[]>("/sign-ups"),
    apiGet<SettingsResponse>("/settings"),
  ]);
  const active = users.filter((u) => u.status === "active").length;
  const assigned = groups.filter((g) => g.kind === "assigned");
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title="Users" description={`${pluralize(users.length, "person", "people")}, ${active} active.`} />
      {signUps.length ? (
        <SignUpQueue signUps={signUps} groups={assigned.filter((g) => settings.signUpGroupIds.includes(g.id))} canDecide={hasRole(me, "user_admin")} />
      ) : null}
      <UsersTable users={users} groups={assigned} initialView={view} inviteOpen={invite === "1"} canGrantRoles={hasRole(me)} />
    </div>
  );
}

import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { UsersTable } from "@/components/directory/users-table";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Group, User } from "@/lib/types";

export const metadata = { title: "Users" };

export default async function UsersPage({ searchParams }: { searchParams: Promise<{ view?: string; invite?: string }> }) {
  const { view, invite } = await searchParams;
  const [users, groups, me] = await Promise.all([apiGet<User[]>("/users"), apiGet<Group[]>("/groups"), apiGet<User>("/me")]);
  const active = users.filter((u) => u.status === "active").length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader title="Users" description={`${pluralize(users.length, "person", "people")}, ${active} active.`} />
      <UsersTable users={users} groups={groups.filter((g) => g.kind === "assigned")} initialView={view} inviteOpen={invite === "1"} canGrantRoles={hasRole(me)} />
    </div>
  );
}

import { hasRole } from "@/components/settings/roles";
import { RolesBoard } from "@/components/settings/roles-board";
import { ReadOnlyNote } from "@/components/settings/shared";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Group, User } from "@/lib/types";

export const metadata = { title: "Roles" };

export default async function RolesPage() {
  const [users, groups, me] = await Promise.all([apiGet<User[]>("/users"), apiGet<Group[]>("/groups"), apiGet<User>("/me")]);
  const people = users.map(({ id, name, email, roles, status, eligibleRoles, activations, roleScopes, eligibleScopes }) => ({
    id,
    name,
    email,
    roles,
    status,
    eligibleRoles,
    activations,
    roleScopes,
    eligibleScopes,
  }));
  const admins = people.filter((p) => p.roles.length > 0).length;
  const canEdit = hasRole(me);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Roles"
        description={`${pluralize(admins, "person holds", "people hold")} an administrator role. Every role can read every administration page; the lists below are what each one may change. Eligible people activate a role for up to 8 hours when they need it. A user or helpdesk administrator limited to groups reaches only those groups' members.`}
      />
      {canEdit ? null : <ReadOnlyNote>Only a global administrator can assign or remove roles.</ReadOnlyNote>}
      <RolesBoard people={people} groups={groups} meId={me.id} canEdit={canEdit} />
    </div>
  );
}

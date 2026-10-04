import { hasRole } from "@/components/settings/roles";
import { RolesBoard } from "@/components/settings/roles-board";
import { ReadOnlyNote } from "@/components/settings/shared";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { User } from "@/lib/types";

export const metadata = { title: "Roles" };

export default async function RolesPage() {
  const [users, me] = await Promise.all([apiGet<User[]>("/users"), apiGet<User>("/me")]);
  const people = users.map(({ id, name, email, roles, status }) => ({ id, name, email, roles, status }));
  const admins = people.filter((p) => p.roles.length > 0).length;
  const canEdit = hasRole(me);
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Roles"
        description={`${pluralize(admins, "person holds", "people hold")} an administrator role. Every role can read every administration page; the lists below are what each one may change.`}
      />
      {canEdit ? null : <ReadOnlyNote>Only a global administrator can assign or remove roles.</ReadOnlyNote>}
      <RolesBoard people={people} meId={me.id} canEdit={canEdit} />
    </div>
  );
}

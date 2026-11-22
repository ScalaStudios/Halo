import { hasRole } from "@/components/settings/roles";
import { RolesBoard } from "@/components/settings/roles-board";
import { ReadOnlyNote } from "@/components/settings/shared";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { Group, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.roles.label };
}

export default async function RolesPage() {
  const [users, groups, me, locale] = await Promise.all([apiGet<User[]>("/users"), apiGet<Group[]>("/groups"), apiGet<User>("/me"), requestLocale()]);
  const t = policyMessages[locale].roles;
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
        title={frameMessages[locale].nav.items.roles.label}
        description={t.summary(admins)}
      />
      {canEdit ? null : <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>}
      <RolesBoard people={people} groups={groups} meId={me.id} canEdit={canEdit} />
    </div>
  );
}

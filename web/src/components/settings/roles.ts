import { ROLES, rolesFor } from "@/components/provisioning/shared";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { Locale } from "@/lib/i18n/locale";
import type { RoleKey } from "@/lib/provisioning-types";
import type { Group, User } from "@/lib/types";

export function hasRole(user: Pick<User, "roles" | "roleScopes">, ...keys: RoleKey[]) {
  return (["global_admin", ...keys] as RoleKey[]).some((key) => user.roles.includes(ROLES[key].label) && !user.roleScopes[key]);
}

export const SCOPED_ROLES: RoleKey[] = ["user_admin", "helpdesk_admin"];

export function scopeNames(scope: string[], groups: Pick<Group, "id" | "name">[], deleted = settingsMessages.en.roles.deletedGroup) {
  return scope.map((id) => groups.find((g) => g.id === id)?.name ?? deleted).join(", ");
}

export function roleLabel(label: string, locale: Locale) {
  const key = KEY_BY_LABEL[label];
  return key ? rolesFor(locale)[key].label : label;
}

export function roleNames(user: Pick<User, "roles" | "roleScopes">, groups: Pick<Group, "id" | "name">[], locale: Locale) {
  const t = settingsMessages[locale].roles;
  return user.roles.map((label) => {
    const scope = user.roleScopes[KEY_BY_LABEL[label]];
    return scope ? t.scopedTo(roleLabel(label, locale), scopeNames(scope, groups, t.deletedGroup)) : roleLabel(label, locale);
  });
}

export function limitedOnly(user: Pick<User, "roles" | "roleScopes">) {
  return user.roles.length > 0 && !ROLE_ORDER.some((key) => hasRole(user, key));
}

export const ROLE_ORDER: RoleKey[] = ["global_admin", "security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"];

const KEY_BY_LABEL = Object.fromEntries(ROLE_ORDER.map((key) => [ROLES[key].label, key])) as Record<string, RoleKey>;

export function keysOf(person: Pick<User, "roles" | "activations">) {
  const activated = person.activations.map((a) => a.role);
  return person.roles.map((label) => KEY_BY_LABEL[label]).filter((key): key is RoleKey => Boolean(key) && !activated.includes(key));
}

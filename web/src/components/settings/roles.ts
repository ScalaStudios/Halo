import { ROLES } from "@/components/provisioning/shared";
import type { RoleKey } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";

export function hasRole(user: Pick<User, "roles">, ...keys: RoleKey[]) {
  return (["global_admin", ...keys] as RoleKey[]).some((key) => user.roles.includes(ROLES[key].label));
}

export const ROLE_ORDER: RoleKey[] = ["global_admin", "security_admin", "user_admin", "helpdesk_admin", "app_admin", "auditor"];

export const CAPABILITIES: Record<RoleKey, string[]> = {
  global_admin: [
    "Everything the other five roles can do",
    "Assign and remove administrator roles",
    "Change the accounts of people who hold an administrator role",
    "Change the organization profile, branding and verified domains",
    "Export the directory, applications and policies as JSON",
  ],
  security_admin: [
    "Create, reorder and delete access policies and networks",
    "Turn sign-in methods on and off, set device trust, resolve risk events",
    "Suspend people and end their sessions",
    "Manage service accounts and API keys",
    "Change security defaults and webhook endpoints",
  ],
  user_admin: [
    "Invite, suspend and restore people",
    "End sessions, reset authentication and remove sign-in methods",
    "Create groups and change their members",
    "Manage access packages, access reviews and lifecycle rules, and revoke grants",
  ],
  helpdesk_admin: ["End someone's sessions", "Reset authentication and remove sign-in methods so people can enroll again"],
  app_admin: [
    "Register, disable and delete applications",
    "Rotate and revoke client secrets, assign groups and configure SAML",
    "Set up, test and run SCIM provisioning to applications",
  ],
  auditor: ["Read every administration page, the sign-in log and the audit log"],
};

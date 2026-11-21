export type RoleKey = "global_admin" | "security_admin" | "user_admin" | "helpdesk_admin" | "app_admin" | "auditor";
export type KeyScope = "api" | "scim" | "registration";

export type ApiKey = {
  id: string;
  serviceAccountId: string;
  serviceAccountName: string;
  label: string;
  prefix: string;
  scopes: KeyScope[];
  createdBy: string | null;
  createdAt: string;
  expiresAt: string | null;
  lastUsedAt: string | null;
  revokedAt: string | null;
  rateLimitHits: number;
};

export type ServiceAccount = {
  id: string;
  name: string;
  email: string;
  description: string;
  ownerId: string | null;
  status: "active" | "suspended";
  rateLimit: number;
  roles: RoleKey[];
  keys: ApiKey[];
  federatedCredentials: FederatedCredential[];
  createdBy: string | null;
  createdAt: string;
};

export type FederatedCredential = {
  id: string;
  serviceAccountId: string;
  name: string;
  issuer: string;
  subject: string;
  audience: string;
  conditions: Record<string, string>;
  scopes: KeyScope[];
  createdBy: string | null;
  createdAt: string;
  lastUsedAt: string | null;
};

export type AppProvisioning = {
  appId: string;
  baseUrl: string;
  enabled: boolean;
  lastSyncAt: string | null;
  lastError: string;
  created: number;
  updated: number;
  deactivated: number;
  provisioned: number;
  groupIds: string[];
  groupsCreated: number;
  groupsUpdated: number;
  groupsDeleted: number;
  updatedAt: string;
};

export type LDAPSecurity = "ldaps" | "starttls" | "none";

export type LDAPMapping = {
  id: string;
  email: string;
  name: string;
  title: string;
  department: string;
  location: string;
  manager: string;
  groupName: string;
  member: string;
  attributes: Record<string, string>;
};

export type LDAPDirectory = {
  id: string;
  name: string;
  url: string;
  security: LDAPSecurity;
  caCertificate: string;
  bindDn: string;
  userBaseDn: string;
  userFilter: string;
  groupBaseDn: string;
  groupFilter: string;
  mapping: LDAPMapping;
  enabled: boolean;
  intervalMinutes: number;
  syncedAt: string | null;
  syncError: string;
  createdAt: string;
  updatedAt: string;
  people: number;
  groups: number;
};

export type LDAPCounts = {
  created: number;
  updated: number;
  suspended: number;
  restored: number;
  groupsCreated: number;
  groupsRenamed: number;
  membersAdded: number;
  membersRemoved: number;
};

export type LDAPSync = {
  id: string;
  startedAt: string;
  finishedAt: string;
  result: "succeeded" | "failed";
  error: string;
  counts: LDAPCounts;
  skipped: string[];
};

export type LDAPSyncReport = LDAPSync & {
  dryRun: boolean;
  changes: { action: string; target: string; detail: string }[];
};

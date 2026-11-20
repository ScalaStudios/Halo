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

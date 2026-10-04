export type RoleKey = "global_admin" | "security_admin" | "user_admin" | "helpdesk_admin" | "app_admin" | "auditor";
export type KeyScope = "api" | "scim";

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
};

export type ServiceAccount = {
  id: string;
  name: string;
  email: string;
  description: string;
  ownerId: string | null;
  status: "active" | "suspended";
  roles: RoleKey[];
  keys: ApiKey[];
  createdBy: string | null;
  createdAt: string;
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
  updatedAt: string;
};

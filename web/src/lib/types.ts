export type UserStatus = "active" | "suspended" | "invited" | "deprovisioned";
export type AuthStrength = "phishing-resistant" | "multi-factor" | "single-factor";
export type MethodKind = "passkey" | "security-key" | "totp" | "magic-link" | "recovery-codes" | "federated";

export type AuthMethod = {
  id: string;
  kind: MethodKind;
  label: string;
  addedAt: string;
  lastUsedAt: string | null;
  detail?: string;
};

export type User = {
  id: string;
  name: string;
  email: string;
  emailVerified: boolean;
  title: string;
  department: string;
  location: string;
  status: UserStatus;
  strength: AuthStrength;
  methods: AuthMethod[];
  roles: string[];
  groupIds: string[];
  appIds: string[];
  managerId: string | null;
  source: string;
  createdAt: string;
  lastSignInAt: string | null;
};

export type Group = {
  id: string;
  name: string;
  description: string;
  kind: "assigned" | "dynamic";
  rule?: string;
  memberCount: number;
  source: string;
  createdAt: string;
};

export type Protocol = "oidc" | "saml" | "oauth";
export type AppType = "web" | "spa" | "native" | "service";

export type Credential = {
  id: string;
  kind: "secret" | "certificate";
  label: string;
  hint: string;
  createdAt: string;
  expiresAt: string;
  lastUsedAt: string | null;
};

export type Application = {
  id: string;
  name: string;
  description: string;
  protocol: Protocol;
  type: AppType;
  status: "active" | "disabled";
  clientId: string;
  homepage: string;
  redirectUris: string[];
  postLogoutUris: string[];
  scopes: string[];
  claims: { name: string; source: string }[];
  groupIds: string[];
  userCount: number;
  credentials: Credential[];
  owner: string | null;
  createdAt: string;
  signIns7d: number;
  failureRate7d: number;
  tokenPolicy: { accessTokenTtl: number; refreshTokenTtl: number; idTokenTtl: number; rotation: boolean };
  setupGuide: "grafana" | "forgejo" | "generic-oidc" | "generic-saml" | "kubernetes" | "aws-iam-identity-center" | "slack";
};

export type Session = {
  id: string;
  userId: string;
  device: string;
  os: string;
  browser: string;
  ip: string;
  location: string;
  method: MethodKind;
  createdAt: string;
  lastActiveAt: string;
  current?: boolean;
  client?: "halo-cli";
};

export type SignInResult = "success" | "failure" | "interrupted";
export type RiskLevel = "none" | "low" | "medium" | "high";

export type SignInEvent = {
  id: string;
  time: string;
  userId: string | null;
  email: string;
  appId: string | null;
  result: SignInResult;
  method: MethodKind;
  ip: string;
  location: string;
  device: string;
  risk: RiskLevel;
  reason?: string;
};

export type AuditEvent = {
  id: string;
  time: string;
  actorId: string | null;
  action: string;
  summary: string;
  targetType: string;
  targetId: string;
  target: string;
  ip: string;
};

export type Organization = { name: string; issuer: string };

export type Overview = {
  weakAdmins: { id: string; name: string }[];
  expiringCredentials: { appId: string; appName: string; credentialId: string; label: string; expiresAt: string; lastUsedAt: string | null; daysLeft: number }[];
  highRiskSignIns: SignInEvent[];
  recentSignIns: SignInEvent[];
  failureRates: { appId: string; appName: string; failureRate7d: number }[];
  counts: { users: number; activeUsers: number; applications: number; activeApplications: number };
};

import type { NamedRef } from "./governance-types";
import type { RoleKey } from "./provisioning-types";

export type UserStatus = "active" | "suspended" | "invited" | "deprovisioned";
export type AuthStrength = "phishing-resistant" | "multi-factor" | "single-factor";
export type MethodKind = "passkey" | "security-key" | "totp" | "magic-link" | "recovery-codes" | "password" | "federated" | "access-pass" | "certificate";

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
  eligibleRoles: RoleKey[];
  activations: RoleActivation[];
  roleScopes: RoleScopes;
  eligibleScopes: RoleScopes;
  groupIds: string[];
  appIds: string[];
  managerId: string | null;
  source: string;
  createdAt: string;
  lastSignInAt: string | null;
  avatarUrl: string | null;
  guest: boolean;
  sponsor: NamedRef | null;
  expiresAt: string | null;
  directory: NamedRef | null;
  attributes: Record<string, AttributeValue>;
  impersonation?: Impersonation;
};

export type AttributeType = "string" | "number" | "boolean" | "date" | "list";
export type AttributeValue = string | number | boolean | string[];

export type Attribute = {
  id: string;
  name: string;
  label: string;
  type: AttributeType;
  selfService: boolean;
  createdAt: string;
  people: number;
};

export type Impersonation = {
  id: string;
  actor: { id: string; name: string };
  user: { id: string; name: string };
  reason: string;
  status: "ready" | "active" | "ended" | "expired";
  createdAt: string;
  startedAt: string | null;
  expiresAt: string;
};

export type RoleScopes = Partial<Record<RoleKey, string[]>>;

export type RoleActivation = {
  id: string;
  role: RoleKey;
  justification: string;
  activatedAt: string;
  expiresAt: string;
};

export type JoinPolicy = "closed" | "request" | "open";

export type Group = {
  id: string;
  name: string;
  description: string;
  kind: "assigned" | "dynamic";
  rule?: string;
  joinPolicy: JoinPolicy;
  owners: NamedRef[];
  memberCount: number;
  source: string;
  createdAt: string;
};

export type OwnedGroup = Group & { requestCount: number };
export type JoinableGroup = Group & { requested: boolean };
export type JoinRequest = { user: NamedRef; createdAt: string };
export type OwnedGroupDetail = { group: OwnedGroup; members: Required<NamedRef>[]; requests: JoinRequest[]; people: Required<NamedRef>[] };

export type Protocol = "oidc" | "saml" | "oauth" | "bookmark" | "forward-auth";
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

export type ApplicationHook = {
  appId: string;
  host: string;
  claims: string[];
  failOpen: boolean;
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
  claims: Claim[];
  claimMappings: Claim[];
  roles: AppRole[];
  groupIds: string[];
  userCount: number;
  credentials: Credential[];
  owner: string | null;
  createdAt: string;
  signIns7d: number;
  failureRate7d: number;
  tokenPolicy: { accessTokenTtl: number; refreshTokenTtl: number; idTokenTtl: number; idTokenAlg: SigningAlgorithm; rotation: boolean; exchange: boolean; exchangeFrom: string[] };
  maxAuthAge: number;
  setupGuide: "grafana" | "forgejo" | "aether" | "generic-oidc" | "generic-saml" | "kubernetes" | "aws-iam-identity-center" | "slack";
  logoUrl: string | null;
  registeredDynamically: boolean;
  awaitingApproval: boolean;
  requiresConsent: boolean;
  backchannelLogoutUri: string;
  backchannelLogoutSessionRequired: boolean;
  requirePar: boolean;
  requirePkce: boolean;
  requireDpop: boolean;
};

export type Claim = { name: string; source: string };

export type AppRole = { id: string; key: string; name: string; description: string; groupIds: string[]; userIds: string[] };

export type Scope = { name: string; description: string };

export type Consent = { appId: string; name: string; homepage: string; logoUrl: string | null; scopes: Scope[]; grantedAt: string };

export type Collection = {
  id: string;
  name: string;
  applicationIds: string[];
  groupIds: string[];
  createdAt: string;
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
  impersonationId?: string;
};

export type SignInResult = "success" | "failure" | "interrupted" | "ended";
export type RiskLevel = "none" | "low" | "medium" | "high";

export type SignInEvent = {
  id: string;
  time: string;
  userId: string | null;
  email: string;
  appId: string | null;
  result: SignInResult;
  method: MethodKind | "token";
  ip: string;
  location: string;
  device: string;
  risk: RiskLevel;
  reason?: string;
  challenge?: "passed" | "failed" | "unavailable";
  hook?: "allowed" | "denied" | "failed_open" | "failed_closed";
  impersonatorId?: string;
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
  changes: AuditChange[];
};

export type AuditChange = { field: string; before?: unknown; after?: unknown; redacted?: boolean };

export type Organization = { name: string; issuer: string };

export type Overview = {
  weakAdmins: { id: string; name: string }[];
  expiringCredentials: { appId: string; appName: string; credentialId: string; label: string; expiresAt: string; lastUsedAt: string | null; daysLeft: number }[];
  highRiskSignIns: SignInEvent[];
  recentSignIns: SignInEvent[];
  failureRates: { appId: string; appName: string; failureRate7d: number }[];
  counts: { users: number; activeUsers: number; applications: number; activeApplications: number };
};

export type RegistrationFilter = "no-passkey" | "password-only" | "authenticator-only" | "no-method";

export type MethodRegistrationReport = {
  people: { id: string; name: string; email: string; department: string; methods: MethodKind[]; strength: AuthStrength; lastSignInAt: string | null }[];
  methods: { method: MethodKind; people: number }[];
  counts: { people: number; filters: Partial<Record<RegistrationFilter, number>> };
};

export type SignInCount = { signIns: number; people: number; lastSignInAt: string | null };

export type ReportPeriod = { from: string; to: string; retentionDays: number; limitedByRetention: boolean };

export type MethodUsageReport = { period: ReportPeriod; methods: (SignInCount & { method: MethodKind })[] };

export type ApplicationUsageReport = {
  period: ReportPeriod;
  applications: (SignInCount & { id: string; name: string; status: "active" | "disabled"; assigned: number })[];
};

export type InactivePeopleReport = {
  days: number;
  people: { id: string; name: string; email: string; status: UserStatus; department: string; lastSignInAt: string | null; manager: NamedRef | null; groups: NamedRef[] }[];
};

export type Recommendation = {
  id: string;
  title: string;
  status: "pass" | "attention" | "fail";
  detail: string;
  href: string;
  action: string;
  dismissal: { reason: string; by: NamedRef | null; at: string } | null;
};

export type SigningAlgorithm = "RS256" | "ES256" | "EdDSA";

export const SIGNING_ALGORITHMS: [SigningAlgorithm, string][] = [
  ["RS256", "RS256 (RSA)"],
  ["ES256", "ES256 (P-256)"],
  ["EdDSA", "EdDSA (Ed25519)"],
];

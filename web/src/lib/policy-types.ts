import type { MethodKind, RiskLevel } from "./types";

export type PolicyMode = "enforce" | "report";
export type PolicyEffect = "allow" | "require-phishing-resistant" | "require-terms" | "block";
export type NetworkCondition = "any" | "in" | "not-in";
export type DeviceCondition = "any" | "trusted" | "untrusted";
export type DeviceTrust = "unknown" | "trusted" | "blocked";
export type ComplianceCondition = "any" | "compliant" | "not-compliant";
export type ComplianceState = "compliant" | "non-compliant" | "unknown";
export type Outcome = "allow" | "require" | "terms" | "block";
export type RiskEventType = "risky-network" | "failed-sign-ins" | "new-device";

export type PolicyConditions = {
  allUsers: boolean;
  groupIds: string[];
  userIds: string[];
  excludeGroupIds: string[];
  excludeUserIds: string[];
  allApps: boolean;
  appIds: string[];
  network: NetworkCondition;
  zoneIds: string[];
  device: DeviceCondition;
  compliance: ComplianceCondition;
  risk: RiskLevel[];
  methods: MethodKind[];
};

export type AccessPolicy = {
  id: string;
  name: string;
  description: string;
  priority: number;
  enabled: boolean;
  mode: PolicyMode;
  effect: PolicyEffect;
  termsId: string | null;
  conditions: PolicyConditions;
  createdAt: string;
  updatedAt: string;
};

export type PolicyWithActivity = AccessPolicy & { activity: { matched: number; stopped: number } };

export type NetworkZone = {
  id: string;
  name: string;
  kind: "trusted" | "risky";
  cidrs: string[];
  createdAt: string;
  updatedAt: string;
};

export type Terms = {
  id: string;
  name: string;
  version: number;
  text: string;
  url: string;
  createdAt: string;
  updatedAt: string;
};

export type TermsAcceptance = {
  termsId: string;
  name: string;
  version: number;
  acceptedAt: string;
};

export type CertificateAuthority = {
  id: string;
  name: string;
  mapping: "email" | "upn" | "subject";
  certificates: { subject: string; expiresAt: string }[];
  createdAt: string;
};

export type MethodSetting = {
  method: MethodKind;
  enabled: boolean;
  updatedAt: string;
  users: number;
  exclusive: number;
};

export type Device = {
  id: string;
  userId: string;
  name: string;
  os: string;
  browser: string;
  trust: DeviceTrust;
  firstSeenAt: string;
  lastSeenAt: string;
  lastIp: string;
  signIns: number;
  compliance: DeviceCompliance | null;
  current?: boolean;
};

export type DeviceCompliance = {
  state: ComplianceState;
  sourceId: string;
  source: string;
  externalId: string;
  record: string;
  reportedAt: string;
  stale: boolean;
};

export type ComplianceSource = {
  id: string;
  name: string;
  kind: "intune" | "agent";
  tenantId: string;
  clientId: string;
  maxAgeMinutes: number;
  reportedAt: string | null;
  syncError: string;
  createdAt: string;
  updatedAt: string;
  records: number;
  linked: number;
};

export type ComplianceRecord = {
  id: string;
  name: string;
  owner: string;
  state: ComplianceState;
  osVersion: string;
  encrypted: boolean | null;
  patchLevel: string;
  reportedAt: string;
};

export type RiskEvent = {
  id: string;
  time: string;
  type: RiskEventType;
  level: "medium" | "high";
  status: "open" | "resolved" | "dismissed";
  userId: string | null;
  ip: string;
  deviceId: string | null;
  device: string;
  signInId: string | null;
  detail: string;
  resolvedBy: string | null;
  resolvedAt: string | null;
};

export type Simulation = {
  effect: PolicyEffect;
  outcome: Outcome;
  policy: string;
  reason: string;
  risk: RiskLevel;
  signals: { type: RiskEventType | "new-network" | "password"; level: RiskLevel; detail: string }[];
  policies: { id: string; name: string; mode: PolicyMode; effect: PolicyEffect; matched: boolean; outcome: Outcome | ""; detail: string }[];
};

import type { MethodKind, RiskLevel } from "./types";

export type PolicyMode = "enforce" | "report";
export type PolicyEffect = "allow" | "require-phishing-resistant" | "block";
export type NetworkCondition = "any" | "in" | "not-in";
export type DeviceCondition = "any" | "trusted" | "untrusted";
export type DeviceTrust = "unknown" | "trusted" | "blocked";
export type Outcome = "allow" | "require" | "block";
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
  current?: boolean;
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
  signals: { type: RiskEventType | "new-network"; level: RiskLevel; detail: string }[];
  policies: { id: string; name: string; mode: PolicyMode; effect: PolicyEffect; matched: boolean; outcome: Outcome | ""; detail: string }[];
};

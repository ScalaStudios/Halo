import type { SigningAlgorithm } from "./types";

export type SshAuthority = { publicKey: string; fingerprint: string; createdAt: string; certificateLifetime: number };

export type SshHostAuthority = SshAuthority & { knownHosts: string };

export type SshHost = {
  id: string;
  hostnames: string[];
  publicKey: string | null;
  fingerprint: string;
  keyType: string;
  serial: number | null;
  validBefore: string | null;
  enrollmentExpiresAt: string | null;
  revokedAt: string | null;
  createdAt: string;
  hostGroupId: string | null;
};

export type PosixSettings = { uidMin: number; uidMax: number; gidMin: number; gidMax: number; nextUid: number; nextGid: number; loginShell: string };

export type HostRule = { groupId: string; sudo: string[] };

export type HostGroup = { id: string; name: string; description: string; rules: HostRule[]; hostCount: number; createdAt: string; updatedAt: string };

export type RemoteAuth = "certificate" | "password" | "key";

export type RemoteTarget = {
  outpostId: string;
  protocol: "ssh";
  host: string;
  port: number;
  username: string;
  auth: RemoteAuth;
  hasCredential: boolean;
  hostKey: string;
  sessionLimit: number;
  record: boolean;
};

export type Outpost = {
  id: string;
  name: string;
  enrolledAt: string | null;
  enrollmentExpiresAt: string | null;
  lastSeenAt: string | null;
  lastIp: string;
  version: string;
  createdAt: string;
  targets: number;
  connected: boolean;
};

export type RemoteSession = {
  id: string;
  createdAt: string;
  applicationId: string | null;
  applicationName: string;
  userId: string | null;
  email: string;
  outpostId: string | null;
  protocol: "ssh";
  address: string;
  username: string;
  ip: string;
  limitSeconds: number;
  record: boolean;
  status: "pending" | "active" | "ended";
  startedAt: string | null;
  endedAt: string | null;
  endReason: string;
  bytesIn: number;
  bytesOut: number;
  recorded: boolean;
  terminatedBy: string | null;
};

export type PrincipalMapping = { id: string; groupId: string; principals: string[]; createdAt: string; updatedAt: string };

export type SshCertificate = {
  serial: number;
  keyId: string;
  userId: string | null;
  userName: string;
  principals: string[];
  fingerprint: string;
  keyType: string;
  ip: string;
  validAfter: string;
  validBefore: string;
  createdAt: string;
};

export type ApiScope = { name: string; description: string };

export type ApiResource = {
  id: string;
  name: string;
  identifier: string;
  description: string;
  accessTokenTtl: number;
  accessTokenAlg: SigningAlgorithm;
  scopes: ApiScope[];
  grants: { appId: string; scopes: string[] }[];
  createdAt: string;
  updatedAt: string;
};

export type DeviceRequest = {
  userCode: string;
  application: string;
  logoUrl: string | null;
  clientId: string;
  scopes: string[];
  ip: string;
  userAgent: string;
  requestedAt: string;
  expiresAt: string;
};

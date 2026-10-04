export type SshAuthority = { publicKey: string; fingerprint: string; createdAt: string; certificateLifetime: number };

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
  scopes: ApiScope[];
  grants: { appId: string; scopes: string[] }[];
  createdAt: string;
  updatedAt: string;
};

export type DeviceRequest = {
  userCode: string;
  application: string;
  clientId: string;
  scopes: string[];
  ip: string;
  userAgent: string;
  requestedAt: string;
  expiresAt: string;
};

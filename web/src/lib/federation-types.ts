export type ProviderKind = "google" | "microsoft" | "github" | "oidc" | "saml";

export type ProfileField = "name" | "title" | "department" | "location";

export type ProviderMapping = {
  profile: { claim: string; field: ProfileField }[];
  groupClaim: string;
  groups: { value: string; groupId: string }[];
};

export type IdentityProvider = {
  id: string;
  kind: ProviderKind;
  name: string;
  issuer: string;
  clientId: string;
  hasSecret: boolean;
  scopes: string[];
  enabled: boolean;
  showOnSignIn: boolean;
  routeByDomain: boolean;
  allowedDomains: string[];
  jit: boolean;
  jitGroupIds: string[];
  mapping: ProviderMapping;
  metadataXml: string;
  emailAttribute: string;
  linkedCount: number;
  createdAt: string;
  updatedAt: string;
};

export type IdentityProviderList = { callbackUrl: string; providers: IdentityProvider[] };

export type SignInProvider = { id: string; name: string; kind: ProviderKind };

export type LinkedAccount = {
  id: string;
  providerId: string;
  providerName: string;
  providerKind: ProviderKind;
  email: string;
  createdAt: string;
  lastUsedAt: string | null;
};

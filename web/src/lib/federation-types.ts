export type ProviderKind = "google" | "microsoft" | "github" | "oidc";

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
  allowedDomains: string[];
  jit: boolean;
  jitGroupIds: string[];
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

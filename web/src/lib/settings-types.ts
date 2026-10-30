export type Theme = {
  accent: string;
  background: string;
  surface: string;
  border: string;
  text: string;
  link: string;
};

export type Settings = {
  organizationName: string;
  contactEmail: string;
  signInMessage: string;
  theme: Theme;
  customCss: string;
  consoleBranding: boolean;
  sessionHours: number;
  lockoutThreshold: number;
  lockoutMinutes: number;
  inviteDays: number;
  accessTokenMinutes: number;
  idTokenMinutes: number;
  refreshTokenHours: number;
};

export type SettingsResponse = {
  settings: Settings;
  defaultOrganizationName: string;
  issuer: string;
  customCssDisabled: boolean;
};

export type OrganizationProfile = {
  name: string;
  issuer: string;
  contactEmail: string;
  signInMessage: string;
  logoUrl: string | null;
  theme: Theme;
  customCss: string;
  consoleBranding: boolean;
};

export type Domain = {
  id: string;
  name: string;
  token: string;
  createdAt: string;
  verifiedAt: string | null;
};

export type Webhook = {
  id: string;
  url: string;
  description: string;
  events: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  lastStatus: DeliveryStatus | null;
  lastDeliveryAt: string | null;
};

export type WebhookEventFamily = { family: string; types: { type: string; description: string }[] };

export type DeliveryStatus = "pending" | "delivered" | "failed";

export type WebhookDelivery = {
  id: string;
  endpointId: string;
  eventId: string;
  eventType: string;
  body: string;
  status: DeliveryStatus;
  attempts: number;
  nextAttemptAt: string;
  lastAttemptAt: string | null;
  responseStatus: number | null;
  error: string;
  createdAt: string;
};

export type OIDCSigningKey = {
  kid: string;
  algorithm: string;
  active: boolean;
  createdAt: string;
  retiresAt: string | null;
  retiredAt: string | null;
};

export type SAMLCertificate = {
  id: string;
  fingerprint: string;
  notAfter: string;
  createdAt: string;
  retiredAt: string | null;
};

export type SigningKeys = { oidc: OIDCSigningKey[]; saml: SAMLCertificate[] };

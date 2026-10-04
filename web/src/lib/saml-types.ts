import type { Application } from "@/lib/types";

export type NameIdFormat = "email" | "persistent" | "unspecified";

export type SamlAttributes = { email: string; name: string; givenName: string; familyName: string; groups: string };

export type SamlConnection = {
  entityId: string;
  metadataUrl: string;
  ssoUrl: string;
  certificateUrl: string;
  certificateFingerprint: string;
  certificateExpiresAt: string;
};

export type SamlSettings = {
  entityId: string;
  acsUrls: string[];
  nameIdFormat: NameIdFormat;
  signResponse: boolean;
  attributes: SamlAttributes;
  metadataXml: string;
  idp: SamlConnection;
};

export type SamlApplication = Application & { saml?: SamlSettings };

export const NAME_ID_FORMATS: Record<NameIdFormat, { label: string; urn: string; value: string }> = {
  email: { label: "Email address", urn: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress", value: "user.email" },
  persistent: { label: "Persistent", urn: "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent", value: "user.id" },
  unspecified: { label: "Unspecified", urn: "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified", value: "user.email" },
};

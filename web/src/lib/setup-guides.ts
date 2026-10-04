import { NAME_ID_FORMATS, type SamlSettings } from "@/lib/saml-types";
import type { Application } from "@/lib/types";

export type SetupGuide = { title: string; description: string; file: string; code: string };

export function endpoints(issuer: string) {
  return {
    issuer,
    discovery: `${issuer}/.well-known/openid-configuration`,
    authorize: `${issuer}/oauth2/authorize`,
    token: `${issuer}/oauth2/token`,
    userinfo: `${issuer}/oauth2/userinfo`,
    jwks: `${issuer}/oauth2/keys`,
    samlMetadata: `${issuer}/saml/metadata`,
    samlSso: `${issuer}/saml/sso`,
    samlCertificate: `${issuer}/saml/certificate`,
  };
}

const pairs = (rows: [string, string][]) => {
  const width = Math.max(16, ...rows.map(([key]) => key.length + 2));
  return rows.map(([key, value]) => `${key.padEnd(width)}${value}`).join("\n");
};

type GuideApp = Pick<Application, "name" | "clientId" | "redirectUris"> & { setupGuide: string; saml?: SamlSettings };

export function setupGuide(app: GuideApp, issuer: string): SetupGuide {
  const urls = endpoints(issuer);
  switch (app.setupGuide) {
    case "grafana":
      return {
        title: "Set up Grafana",
        description: "Add this block to grafana.ini, save the client secret to /etc/grafana/halo-client-secret, then restart Grafana.",
        file: "grafana.ini",
        code: [
          "[auth.generic_oauth]",
          "enabled = true",
          "name = Halo",
          "allow_sign_up = true",
          `client_id = ${app.clientId}`,
          "client_secret = $__file{/etc/grafana/halo-client-secret}",
          "scopes = openid profile email groups",
          `auth_url = ${urls.authorize}`,
          `token_url = ${urls.token}`,
          `api_url = ${urls.userinfo}`,
          "use_pkce = true",
          "groups_attribute_path = groups",
          "role_attribute_path = contains(groups[*], 'Grafana editors') && 'Editor' || 'Viewer'",
        ].join("\n"),
      };
    case "forgejo":
      return {
        title: "Set up Forgejo",
        description: "Run this on the Forgejo host as the user that runs Forgejo. Keep the name halo: it is part of the redirect URI.",
        file: "Shell",
        code: [
          "forgejo admin auth add-oauth \\",
          "  --name halo \\",
          "  --provider openidConnect \\",
          `  --key ${app.clientId} \\`,
          '  --secret "$HALO_CLIENT_SECRET" \\',
          `  --auto-discover-url ${urls.discovery} \\`,
          "  --scopes openid --scopes profile --scopes email --scopes groups \\",
          "  --group-claim-name groups \\",
          '  --admin-group "Forgejo maintainers"',
        ].join("\n"),
      };
    case "kubernetes":
      return {
        title: "Set up kubectl",
        description: "Add this user to each engineer's kubeconfig. It needs the kubelogin plugin (kubectl oidc-login) and no client secret.",
        file: "~/.kube/config",
        code: [
          "users:",
          "- name: halo",
          "  user:",
          "    exec:",
          "      apiVersion: client.authentication.k8s.io/v1beta1",
          "      command: kubectl",
          "      args:",
          "      - oidc-login",
          "      - get-token",
          `      - --oidc-issuer-url=${urls.issuer}`,
          `      - --oidc-client-id=${app.clientId}`,
          "      - --oidc-extra-scope=email",
          "      - --oidc-extra-scope=groups",
          "      interactiveMode: IfAvailable",
        ].join("\n"),
      };
    case "generic-saml":
      return {
        title: "SAML values",
        description: `Most applications import the metadata URL. Use the rest if ${app.name} asks for each value separately.`,
        file: "SAML 2.0",
        code: pairs([
          ["Metadata URL", urls.samlMetadata],
          ["IdP entity ID", urls.samlMetadata],
          ["SSO URL", urls.samlSso],
          ["Certificate", urls.samlCertificate],
          ["Fingerprint", app.saml ? `SHA-256 ${app.saml.idp.certificateFingerprint}` : "—"],
          ["SP entity ID", app.clientId],
          ["ACS URL", app.redirectUris[0] ?? "—"],
          ["NameID format", NAME_ID_FORMATS[app.saml?.nameIdFormat ?? "email"].urn],
        ]),
      };
    case "aws-iam-identity-center":
      return {
        title: "Set up AWS IAM Identity Center",
        description:
          "In IAM Identity Center, open Settings, change the identity source to an external identity provider and upload the metadata file from the metadata URL, or enter these values. Copy the ACS URL and issuer URL AWS shows into this application's Authentication tab.",
        file: "IAM Identity Center",
        code: pairs([
          ["IdP SAML metadata", urls.samlMetadata],
          ["IdP sign-in URL", urls.samlSso],
          ["IdP issuer URL", urls.samlMetadata],
          ["IdP certificate", urls.samlCertificate],
        ]),
      };
    case "slack":
      return {
        title: "Set up Slack",
        description:
          "In Slack, open the workspace settings and configure SAML authentication with these values. For the certificate, download it from the certificate URL and paste the whole file, including the BEGIN and END lines.",
        file: "Slack",
        code: pairs([
          ["SAML 2.0 Endpoint (HTTP)", urls.samlSso],
          ["Identity Provider Issuer", urls.samlMetadata],
          ["Public Certificate", urls.samlCertificate],
        ]),
      };
    default:
      return {
        title: "Endpoints",
        description: `Most applications only need the discovery endpoint. Use the rest if ${app.name} asks for each endpoint separately.`,
        file: "OpenID Connect",
        code: pairs([
          ["Discovery", urls.discovery],
          ["Issuer", urls.issuer],
          ["Authorization", urls.authorize],
          ["Token", urls.token],
          ["Userinfo", urls.userinfo],
          ["JWKS", urls.jwks],
          ["Client ID", app.clientId],
        ]),
      };
  }
}

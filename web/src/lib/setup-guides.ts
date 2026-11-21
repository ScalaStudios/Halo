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

export function proxyGuides(app: Pick<Application, "id" | "claims" | "redirectUris">, issuer: string): SetupGuide[] {
  const verify = `${issuer}/forward-auth/${app.id}/verify`;
  const origins = app.redirectUris.map((uri) => new URL(uri).origin);
  const hosts = app.redirectUris.map((uri) => new URL(uri).host);
  const headers = app.claims.map((claim) => claim.name);
  const variable = (header: string) => `$halo_${header.toLowerCase().replace(/[^a-z0-9]/g, "_")}`;
  return [
    {
      title: "Traefik",
      description: `Add the middleware to the routers of ${hosts.join(", ")}. Traefik sends X-Forwarded-Host and X-Forwarded-Uri itself.`,
      file: "dynamic.yml",
      code: ["http:", "  middlewares:", "    halo:", "      forwardAuth:", `        address: ${verify}`, "        authResponseHeaders:", ...headers.map((h) => `          - ${h}`)].join("\n"),
    },
    {
      title: "nginx",
      description: "Needs the auth_request module. nginx gets a 401 with the sign-in address and turns it into a redirect.",
      file: "nginx.conf",
      code: [
        "server {",
        `    server_name ${app.redirectUris.map((uri) => new URL(uri).hostname).join(" ")};`,
        "",
        "    location / {",
        "        auth_request /halo-auth;",
        "        auth_request_set $halo_location $upstream_http_location;",
        "        auth_request_set $halo_cookie $upstream_http_set_cookie;",
        ...headers.map((h) => `        auth_request_set ${variable(h)} $upstream_http_${h.toLowerCase().replace(/-/g, "_")};`),
        "        error_page 401 =302 $halo_location;",
        "        add_header Set-Cookie $halo_cookie always;",
        ...headers.map((h) => `        proxy_set_header ${h} ${variable(h)};`),
        "        proxy_pass http://127.0.0.1:8080;",
        "    }",
        "",
        "    location = /halo-auth {",
        "        internal;",
        `        proxy_pass ${verify}?nginx;`,
        "        proxy_ssl_server_name on;",
        "        proxy_pass_request_body off;",
        '        proxy_set_header Content-Length "";',
        "        proxy_set_header X-Forwarded-Host $http_host;",
        "        proxy_set_header X-Forwarded-Uri $request_uri;",
        "    }",
        "}",
      ].join("\n"),
    },
    {
      title: "Caddy",
      description: "Caddy sends X-Forwarded-Host and X-Forwarded-Uri itself, and removes these headers from requests it doesn't let through.",
      file: "Caddyfile",
      code: [
        `${origins.join(", ")} {`,
        `\tforward_auth ${issuer} {`,
        `\t\turi /forward-auth/${app.id}/verify`,
        "\t\theader_up Host {upstream_hostport}",
        `\t\tcopy_headers ${headers.join(" ")}`,
        "\t}",
        "\treverse_proxy 127.0.0.1:8080",
        "}",
      ].join("\n"),
    },
  ];
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
    case "aether": {
      const [, address = "https://mail.example.com", slug = "halo"] = app.redirectUris[0]?.match(/^(.+)\/api\/v1\/auth\/oidc\/([^/]+)\/callback$/) ?? [];
      return {
        title: "Set up Aether",
        description:
          "In Aether, open Administration → Identity providers → Connect Halo and enter these values. Group mappings match Halo group names and give their members the owner, admin or member role in an organization. Optional: to suspend Aether accounts as soon as Halo suspends them, add an endpoint under Webhooks in Halo for the webhook URL with the user, scim and session events, and paste its signing secret into Webhook signing secret in Aether. To create accounts before people sign in, turn on SCIM for the provider in Aether, then set up this application under Provisioning in Halo with the SCIM base URL and Aether's bearer token.",
        file: "Aether",
        code: pairs([
          ["Issuer URL", urls.issuer],
          ["Slug", slug],
          ["Client ID", app.clientId],
          ["Client secret", "shown once when you created this application"],
          ["Administrator group", "a Halo group whose members become platform administrators"],
          ["Group mappings", "Halo group name → organization → owner, admin or member"],
          ["Webhook URL", `${address}/api/v1/hooks/identity-providers/${slug}`],
          ["Webhook events", "user.*  scim.*  session.*"],
          ["SCIM base URL", `${address}/api/v1/scim/${slug}`],
        ]),
      };
    }
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

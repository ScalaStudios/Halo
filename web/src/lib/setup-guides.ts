import { NAME_ID_FORMATS, type SamlSettings } from "@/lib/saml-types";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
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

export function proxyGuides(app: Pick<Application, "id" | "claims" | "redirectUris">, issuer: string, locale: Locale): SetupGuide[] {
  const t = applicationsMessages[locale].guides;
  const verify = `${issuer}/forward-auth/${app.id}/verify`;
  const origins = app.redirectUris.map((uri) => new URL(uri).origin);
  const hosts = app.redirectUris.map((uri) => new URL(uri).host);
  const headers = app.claims.map((claim) => claim.name);
  const variable = (header: string) => `$halo_${header.toLowerCase().replace(/[^a-z0-9]/g, "_")}`;
  return [
    {
      title: "Traefik",
      description: t.traefik(hosts.join(", ")),
      file: "dynamic.yml",
      code: ["http:", "  middlewares:", "    halo:", "      forwardAuth:", `        address: ${verify}`, "        authResponseHeaders:", ...headers.map((h) => `          - ${h}`)].join("\n"),
    },
    {
      title: "nginx",
      description: t.nginx,
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
      description: t.caddy,
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

export function setupGuide(app: GuideApp, issuer: string, locale: Locale): SetupGuide {
  const t = applicationsMessages[locale].guides;
  const urls = endpoints(issuer);
  switch (app.setupGuide) {
    case "grafana":
      return {
        title: t.setUp("Grafana"),
        description: t.grafana,
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
        title: t.setUp("Forgejo"),
        description: t.forgejo,
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
        title: t.setUp("Aether"),
        description: t.aether,
        file: "Aether",
        code: pairs([
          ["Issuer URL", urls.issuer],
          ["Slug", slug],
          ["Client ID", app.clientId],
          ["Client secret", t.aetherSecret],
          ["Administrator group", t.aetherAdmins],
          ["Group mappings", t.aetherMappings],
          ["Webhook URL", `${address}/api/v1/hooks/identity-providers/${slug}`],
          ["Webhook events", "user.*  scim.*  session.*"],
          ["SCIM base URL", `${address}/api/v1/scim/${slug}`],
        ]),
      };
    }
    case "kubernetes":
      return {
        title: t.setUp("kubectl"),
        description: t.kubernetes,
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
        title: t.samlTitle,
        description: t.saml(app.name),
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
        title: t.setUp("AWS IAM Identity Center"),
        description: t.aws,
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
        title: t.setUp("Slack"),
        description: t.slack,
        file: "Slack",
        code: pairs([
          ["SAML 2.0 Endpoint (HTTP)", urls.samlSso],
          ["Identity Provider Issuer", urls.samlMetadata],
          ["Public Certificate", urls.samlCertificate],
        ]),
      };
    default:
      return {
        title: t.endpointsTitle,
        description: t.endpoints(app.name),
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

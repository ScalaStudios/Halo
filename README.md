<h1 align="center"><img src=".github/assets/banner.png" alt="Halo — open-source identity and access management" width="100%"></h1>

<p align="center">
  <a href="https://github.com/ScalaStudios/Halo/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/ScalaStudios/Halo/ci.yml?branch=main&amp;style=flat&amp;label=ci&amp;labelColor=2a2724" alt="CI status"></a>
  <a href="https://github.com/ScalaStudios/Halo/releases/latest"><img src="https://img.shields.io/github/v/release/ScalaStudios/Halo?style=flat&amp;label=release&amp;labelColor=2a2724" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-57534d?style=flat&amp;labelColor=2a2724" alt="Apache 2.0 license"></a>
</p>

<p align="center">
  <a href="https://halo.scala.gg">Website</a> ·
  <a href="https://halo.scala.gg/docs">Documentation</a> ·
  <a href="#why-halo">Why Halo</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#screenshots">Screenshots</a> ·
  <a href="https://github.com/ScalaStudios/Halo/releases">Releases</a>
</p>

## Identity and access for apps and infrastructure

Halo is an open-source identity and access management platform from Scala Studios. People sign in to their apps with one account. Administrators manage access from a shared directory and review who still needs it.

Use passkeys and security keys, connect apps over OpenID Connect or SAML, and manage provisioning and access policies from the admin console. Run Halo on your own infrastructure or use invite-only Halo Cloud.

> [!NOTE]
> Halo is in active development. The API and configuration may change before 1.0. See the [latest release](https://github.com/ScalaStudios/Halo/releases/latest) for release notes and image versions.

## Why Halo

Halo is a fit for teams that want app sign-in, workforce access reviews, and Linux host access tied to the same directory. You can run that stack on your own infrastructure under Apache-2.0 or use invite-only Halo Cloud.

- **Approve, review, and expire access.** People request time-limited access packages. Reviewers can remove stale group access when a review closes, and lifecycle rules update membership as people join, change roles, or leave. [Governance guide](https://halo.scala.gg/docs/governance)
- **Use directory groups for apps and servers.** Assign apps, sync people and groups over SCIM or LDAP, and map group memberships to short-lived SSH certificates, Linux accounts, and sudo rules. [Provisioning](https://halo.scala.gg/docs/provisioning) · [Infrastructure access](https://halo.scala.gg/docs/infrastructure-access)
- **Test sign-in rules before enforcing them.** Start with passkeys and security keys. Use report-only mode to see how a policy would affect recent sign-ins, or simulate one with a specific person, application, and device. [Access policies](https://halo.scala.gg/docs/policies)

### How Halo compares

| Platform | When to choose it | What to evaluate alongside Halo |
| --- | --- | --- |
| **Halo** | You want workforce sign-in, access approvals and reviews, and SSH or Linux host access in one self-hostable stack. | Each installation serves one organization. Self-host with PostgreSQL, or use invite-only Halo Cloud. [Deployment options](#quick-start) |
| **Keycloak** | You need a self-hosted identity provider with realms, LDAP federation, and customizable authentication flows. | Compare Keycloak's provider extensions with Halo's built-in access packages, group reviews, and SSH certificate authority. [Keycloak administration](https://www.keycloak.org/docs/latest/server_admin/) · [Server extensions](https://www.keycloak.org/docs/latest/server_development/) |
| **authentik** | You want to compose sign-in flows and deploy proxy, LDAP, or RADIUS outposts near your applications. | authentik also offers time-limited access requests in Enterprise Preview. Evaluate those alongside Halo's group reviews, lifecycle rules, and Linux host management. [Outposts](https://docs.goauthentik.io/add-secure-apps/outposts/) · [Access requests](https://docs.goauthentik.io/add-secure-apps/access-requests/) |
| **ZITADEL** | You are building B2B login and need multiple customer organizations in one instance, with delegated administration. | ZITADEL can delegate access to projects across organizations. Halo's directory and governance workflows serve one organization per installation. [ZITADEL organizations](https://zitadel.com/docs/guides/manage/console/organizations-overview) |
| **Okta** | You want managed workforce identity with access certifications and lifecycle automation. | Okta Identity Governance includes requests, certification campaigns, Lifecycle Management, and Workflows. Halo gives you a self-hosted option with group-based reviews and infrastructure access. [Okta Identity Governance](https://help.okta.com/oie/en-us/Content/Topics/identity-governance/iga.htm) |

## Quick start

Choose the setup that fits how you want to run Halo.

| Option | Start here |
| --- | --- |
| **Managed** | [Halo Cloud](#halo-cloud) runs Halo for you. |
| **Self-hosted** | [Install on a Linux server](#install-on-a-server) with Docker Compose, or deploy with the [Helm chart](deploy/helm/halo/README.md). |
| **Local development** | [Run from source](#run-from-source) and load the Fernway demo. |

### Install on a server

You need a Linux server with `curl` and `openssl`, a domain name such as `auth.example.com` that points at the server, and ports 80 and 443 open to the internet. Run the script as root and it installs Docker if the server doesn't have it.

```bash
curl -fsSL https://halo.scala.gg/install.sh | sh
```

The script asks for your domain and organization name, sets Halo up in `/opt/halo` (or `~/halo` when you are not root), generates a database password and `HALO_SECRET_KEY`, then downloads and starts PostgreSQL, Halo and Caddy, which obtains a TLS certificate for your domain. [Read the script](deploy/install.sh) before you run it if you prefer.

When it finishes, create the first administrator with the command it prints:

```bash
/opt/halo/halo.sh admin you@example.com "Your Name"
```

Open the link it prints and register a passkey. The console is at `https://auth.example.com/admin`. To install step by step instead, and for backups, upgrades and email, follow the [self-hosting guide](https://halo.scala.gg/docs/self-hosting).

The public installer currently uses `v0.1.1`. To upgrade to [`v0.5.0`](https://github.com/ScalaStudios/Halo/releases/tag/v0.5.0), [back up PostgreSQL and `HALO_SECRET_KEY`](https://halo.scala.gg/docs/self-hosting), then set both Halo image tags in your installed `compose.yml` to `0.5.0`. Run these commands from the installation directory:

```bash
docker compose pull halo-server halo-web
docker compose up -d --no-deps halo-server halo-web
```

### Halo Cloud

Scala Studios manages the installation, database, backups, and upgrades. You manage your directory, applications, and access policies. Halo Cloud is invite-only. [See plans and sign in](https://halo.scala.gg/cloud).

### Run from source

To work on Halo or try it on your own computer, you need Go 1.27, [Bun](https://bun.sh) 1.3 or Node.js 22, and Docker.

```bash
git clone https://github.com/ScalaStudios/Halo.git && cd Halo
docker compose -f compose.dev.yml up -d
cp .env.example .env
```

Set `HALO_SECRET_KEY` in `.env` to the output of `openssl rand -base64 32`. Then create an administrator and start the server:

```bash
set -a && . ./.env && set +a
go run ./cmd/halo bootstrap --email you@example.com --name "Your Name"
go run ./cmd/halo serve
```

Start the web interface in a second terminal:

```bash
cd web && bun install && bun run dev
```

Open the setup link that `bootstrap` printed to register your passkey. Halo runs at http://localhost:3200. To explore a populated organization instead, `go run ./cmd/halo seed-demo` loads Fernway Systems, a demo company with 68 people; [Get started](https://halo.scala.gg/docs/getting-started#explore-with-demo-data) explains how to sign in to it.

## Screenshots

The screenshots show Fernway Systems, a fictional organization with 68 people, 14 groups, 13 applications, and sample activity. The demo includes generated profile photos and example integrations. [Load the built-in demo data](https://halo.scala.gg/docs/getting-started#explore-with-demo-data) to explore Halo locally.

[![The Fernway Systems overview with the administrator's profile photo, directory counts, open sign-in risk, and security posture](.github/assets/fernway-overview.png)](.github/assets/fernway-overview.png)

<table>
  <tr>
    <td width="50%"><a href=".github/assets/sign-in.png"><img src=".github/assets/sign-in.png" alt="Halo sign-in with Continue with passkey as the main action"></a><br>Passkey-first sign-in</td>
    <td width="50%"><a href=".github/assets/app-launcher.png"><img src=".github/assets/app-launcher.png" alt="The account portal showing assigned applications with Grafana, Forgejo, and Nextcloud logos"></a><br>Account app launcher</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/directory.png"><img src=".github/assets/directory.png" alt="The Fernway Systems directory showing profile photos, user status, and authentication strength"></a><br>User directory</td>
    <td width="50%"><a href=".github/assets/identity-checks.png"><img src=".github/assets/identity-checks.png" alt="Priya Raman's profile photo, registered sign-in methods, and verified in-person identity check"></a><br>Authentication methods and identity checks</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/policies.png"><img src=".github/assets/policies.png" alt="An enforced policy and two report-only policies with impact estimates from recent sign-ins"></a><br>Conditional access</td>
    <td width="50%"><a href=".github/assets/access-review.png"><img src=".github/assets/access-review.png" alt="An overdue review of the production access group with keep and remove decisions for individual members"></a><br>Access review</td>
  </tr>
</table>

<details>
<summary>Applications and integrations</summary>

<table>
  <tr>
    <td width="50%"><a href=".github/assets/application.png"><img src=".github/assets/application.png" alt="Grafana's OpenID Connect connection details and grafana.ini setup example"></a><br>OpenID Connect setup</td>
    <td width="50%"><a href=".github/assets/saml.png"><img src=".github/assets/saml.png" alt="Slack's SAML metadata, sign-in endpoint, certificate, and setup guide"></a><br>SAML setup</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/client-security.png"><img src=".github/assets/client-security.png" alt="Billing API configured with private key JWT, the FAPI 2.0 security profile, and required DPoP"></a><br>OAuth client security</td>
    <td width="50%"><a href=".github/assets/federation.png"><img src=".github/assets/federation.png" alt="The enabled Fernway Partner SSO provider with automatic account provisioning"></a><br>Federated sign-in</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/provisioning.png"><img src=".github/assets/provisioning.png" alt="A Workday connector and recent SCIM changes to the directory"></a><br>Inbound SCIM</td>
    <td width="50%"><a href=".github/assets/outbound-scim.png"><img src=".github/assets/outbound-scim.png" alt="Slack provisioning settings with sample account and group sync totals"></a><br>Outbound SCIM</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/directory-sync.png"><img src=".github/assets/directory-sync.png" alt="Two successful example runs in the Fernway Workforce Directory sync history"></a><br>LDAP directory sync</td>
    <td width="50%"><a href=".github/assets/webhooks.png"><img src=".github/assets/webhooks.png" alt="The sample security events webhook with successful delivery history"></a><br>Webhook deliveries</td>
  </tr>
</table>

</details>

<details>
<summary>Governance and infrastructure</summary>

<table>
  <tr>
    <td width="50%"><a href=".github/assets/groups.png"><img src=".github/assets/groups.png" alt="Fernway groups including Engineering, Contractors, and Production access"></a><br>Groups and membership</td>
    <td width="50%"><a href=".github/assets/access-packages.png"><img src=".github/assets/access-packages.png" alt="Requestable access packages for Grafana, Kubernetes, and production systems"></a><br>Access packages</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/access-requests.png"><img src=".github/assets/access-requests.png" alt="Pending approval requests and recent decisions for temporary access"></a><br>Access requests</td>
    <td width="50%"><a href=".github/assets/lifecycle.png"><img src=".github/assets/lifecycle.png" alt="Enabled joiner and leaver rules with recent run history"></a><br>Lifecycle automation</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/ssh-certificates.png"><img src=".github/assets/ssh-certificates.png" alt="Group-to-principal mappings and an example short-lived SSH certificate"></a><br>SSH certificates</td>
    <td width="50%"><a href=".github/assets/infrastructure.png"><img src=".github/assets/infrastructure.png" alt="Fernway bastions and CI runners with Linux account and sudo rules by host group"></a><br>Linux host access</td>
  </tr>
</table>

</details>

<details>
<summary>Monitoring and reporting</summary>

<table>
  <tr>
    <td width="50%"><a href=".github/assets/sign-in-logs.png"><img src=".github/assets/sign-in-logs.png" alt="Successful and failed demo sign-ins with methods, applications, and risk levels"></a><br>Sign-in logs</td>
    <td width="50%"><a href=".github/assets/risk.png"><img src=".github/assets/risk.png" alt="An open sign-in risk event with the reason and actions to investigate it"></a><br>Risk events</td>
  </tr>
  <tr>
    <td width="50%"><a href=".github/assets/audit-log.png"><img src=".github/assets/audit-log.png" alt="Administrator and service account actions in the sample audit history"></a><br>Audit log</td>
    <td width="50%"><a href=".github/assets/reports.png"><img src=".github/assets/reports.png" alt="Sign-in method enrollment counts and a per-person breakdown"></a><br>Authentication reports</td>
  </tr>
</table>

</details>

## Features

Halo covers sign-in, directory sync, access governance, infrastructure access, and audit logging. [Explore the documentation](https://halo.scala.gg/docs) for setup guides.

<details>
<summary>Full feature list</summary>

| Area | What Halo includes |
| --- | --- |
| **Sign-in** | Passkeys and security keys (WebAuthn), authenticator apps (TOTP), single-use recovery codes, and magic links or one-time codes by email. Sign-in through Google, Microsoft Entra ID, GitHub or any OpenID Connect provider, with account linking and accounts created on first sign-in, and Windows sign-in with the Kerberos ticket of a computer joined to Active Directory. |
| **Single sign-on** | An OpenID Connect provider and OAuth 2.0 authorization server for web, single-page, native and machine-to-machine applications. A SAML 2.0 identity provider with service provider metadata import and IdP-initiated sign-in. Forward auth for Traefik, nginx and Caddy in front of applications that support neither, and a read-only LDAP directory for applications that only speak LDAP. |
| **Directory** | People with profiles and managers, assigned and rule-based groups, service accounts, devices, and scoped administrative roles. Organizations can require an identity check before an invitation or self-service sign-up becomes active. |
| **Provisioning** | A SCIM 2.0 server, tested with the request formats of Okta and Microsoft Entra ID, so either of them or an HR system can provision people into Halo; LDAP directory sync that reads people and groups from Active Directory or OpenLDAP on a schedule and suspends people who leave; and outbound SCIM that creates, updates and deactivates accounts in your applications and keeps the groups you choose in step. |
| **Access policies** | Conditional access by person, group, application, network zone, device trust, device compliance from Microsoft Intune or a device agent, sign-in risk and sign-in method, with effects that allow, block, require a passkey or security key, or require accepting versioned terms of use, report-only mode and a simulator. |
| **Governance** | Access packages that people request, with approvers, justifications and time limits; access reviews that can remove access automatically when they complete; administrator roles that eligible people activate for a few hours with a reason; guest accounts with a sponsor and an end date; and joiner, mover and leaver rules, plus rules that run on a date such as a contract end date or after a number of days without signing in. |
| **Infrastructure access** | An SSH certificate authority that issues short-lived certificates to people in mapped groups, through `halo login` and `halo ssh-cert`, and a host certificate authority that signs registered servers' host keys with `halo ssh-host`, so people connect without the first-connection prompt; Linux accounts and sudo rules from Halo groups on your servers through `halo-hostd`; and a RADIUS server that signs people in to VPNs and network devices with their Halo password. |
| **Monitoring** | A sign-in log, an audit log of every administrative change, risk events for risky networks, repeated failures, new devices and new networks, signed webhooks for audit and sign-in events, and streaming of both to syslog or OpenTelemetry. |
| **Administration** | An admin console whose home page shows the directory, which features are on, your roles, open risk events and shortcuts, a self-service account portal with profile pictures, organization branding, sign-in methods you can switch off, optional areas you can switch off with their routes and background jobs (SAML, SCIM in either direction, SSH certificates, governance, webhooks, forward auth, RADIUS, the LDAP server, Kerberos sign-in and log streaming), email over SMTP with retries, JSON export with scheduled exports to an S3-compatible bucket, import of configuration from an export with a dry run, and rotation of client secrets, signing keys and `HALO_SECRET_KEY`. |
| **Developers** | An OpenAPI 3.1 REST API, service accounts and API keys, scoped API resources and JWT access tokens, a relationship-based authorization API, and setup guides for common applications. |

</details>

## Supported standards

Halo supports OpenID Connect, OAuth 2.0, SAML 2.0, SCIM 2.0, WebAuthn, and other standards. See the [conformance report](CONFORMANCE.md) for test results and known gaps.

<details>
<summary>Protocol support and limitations</summary>

| Standard | Status | How Halo uses it |
| --- | --- | --- |
| OpenID Connect Core 1.0 and Discovery 1.0 | Implemented | ID tokens signed with RS256, ES256 or EdDSA, userinfo, `prompt` and `max_age`, discovery at `/.well-known/openid-configuration`, RP-initiated logout, and back-channel logout with signed logout tokens |
| OAuth 2.0 (RFC 6749) | Implemented | Authorization code, refresh token and client credentials grants; refresh tokens rotate by default, and a used one is rejected |
| PKCE (RFC 7636) | Implemented | `S256`, required for single-page and native applications and for any application set to require it |
| Device Authorization Grant (RFC 8628) | Implemented | Used by `halo login` to sign people in on the command line |
| Token Introspection (RFC 7662) and Revocation (RFC 7009) | Implemented | Introspection for APIs that check opaque tokens, and revocation for applications that sign people out |
| Resource Indicators (RFC 8707) | Partial | The `resource` parameter selects an API resource on authorization, device, client credentials and token exchange requests, and its JWT access tokens carry it in `aud` |
| Token Exchange (RFC 8693) | Partial | Web and service applications with token exchange turned on trade a person's access token for one to an API they have scopes on, with the application in the `act` claim. Access tokens only, and no `actor_token` |
| Pushed Authorization Requests (RFC 9126) | Implemented | `/oauth2/par`, with a `request_uri` that works once within 60 seconds, and an option per application to require it |
| DPoP (RFC 9449) | Partial | Access and refresh tokens bound to the client's key with `cnf.jkt`, proofs checked at the token and userinfo endpoints, authorization codes bound with `dpop_jkt` or a proof at `/oauth2/par`, and an option per application to require it. A [Go resource-server example](examples/go-dpop-resource/) verifies bound access tokens. Server-provided nonces remain open. |
| JWT client authentication (RFC 7523) | Implemented | `private_key_jwt` for web and service applications, with keys from a registered JWKS or `jwks_uri`, an algorithm allowlist without `none` or HMAC, and each `jti` refused a second time across instances |
| Authorization Server Issuer Identification (RFC 9207) | Implemented | `iss` in every authorization response, in the query, the fragment or the `form_post` form |
| FAPI 2.0 Security Profile | Partial | An option per application that requires private_key_jwt with PS256, ES256 or EdDSA assertions, pushed requests, PKCE with S256, DPoP, https redirect URIs, ES256 or EdDSA ID tokens and one-minute authorization codes. Refresh token rotation is disabled for the profile. Mutual TLS client authentication is unavailable |
| SAML 2.0 | Implemented | Web browser single sign-on as an identity provider: SP- and IdP-initiated, HTTP-Redirect and HTTP-POST bindings, signed assertions and metadata, and assertions encrypted with AES-256-GCM or AES-256-CBC per service provider. No single logout |
| Shared Signals Framework 1.0, CAEP and RISC | Partial | Security Event Tokens (RFC 8417) for ended sessions, credential and device compliance changes, suspensions, restores, deletions and email changes, pushed (RFC 8935) or polled (RFC 8936) by service applications a security administrator turns on. One stream per receiver, every person as subject, no subject management |
| SCIM 2.0 (RFC 7643 and RFC 7644) | Implemented | A server for users and groups with the full filter grammar and bulk operations, and a client that provisions users into applications |
| WebAuthn and FIDO2 | Implemented | Passkeys and security keys as discoverable credentials, for sign-in without a username |
| TOTP (RFC 6238) | Implemented | Six-digit codes from authenticator apps |
| RADIUS (RFC 2865 and RFC 2866) | Partial | PAP sign-in, Access-Challenge for authenticator codes, Message-Authenticator (RFC 3579) required on every Access-Request and sent first in every reply, and accounting stops in the sign-in log. No EAP, CHAP or RADIUS over TLS |
| SPNEGO (RFC 4559) and Kerberos V5 (RFC 4120) | Partial | Windows sign-in on the sign-in page: Kerberos tickets for Halo's service principal, checked with a keytab, with a replay cache shared by every Halo server and 5 minutes of clock skew. No NTLM, mutual authentication, delegation or PAC |
| LDAP (RFC 4511 and RFC 4513) | Partial | A client that syncs people and groups from a directory over LDAPS or StartTLS with the simple paged results control (RFC 2696), and a read-only server for applications that only speak LDAP: simple binds over LDAPS or StartTLS, search with and, or, not, equality, substring and presence filters and paged results, and no writes or SASL |

"Implemented" means the feature works in the current release. Halo is not OpenID Certified or certified in any other way. [CONFORMANCE.md](CONFORMANCE.md) records which OpenID Foundation conformance suite test plans pass when we run the suite against Halo, with the date, the Halo version and every known warning.

</details>

### Organization-wide OAuth client policies

A global administrator can set `clientPolicies` through `PATCH /api/v1/settings`. An `all` rule applies to OAuth and OIDC applications. A `web`, `spa`, `native`, or `service` rule adds requirements for that type. The built-in Halo CLI keeps its own settings. This example requires signed client assertions, pushed requests, PKCE, and DPoP for web applications and caps their configured access token lifetime at ten minutes.

```json
{
  "clientPolicies": [
    {
      "applicationType": "web",
      "allowedAuthMethods": ["private_key_jwt"],
      "requirePar": true,
      "requirePkce": true,
      "requireDpop": true,
      "maxAccessTokenTtl": 600
    }
  ]
}
```

New applications inherit the required protections and stricter lifetime settings. Global rules leave existing applications as configured until they are saved or enabled. For a new app that requires signed client assertions, send `tokenEndpointAuthMethod: "private_key_jwt"` with its public `jwks` or `jwksUri` when you create it through the API. [OpenAPI reference](api/openapi.yaml)

## Connect your first application

1. In the console, open **Applications**, choose **Add application**, then **OpenID Connect** and **Web application**. Pick a template such as **Grafana**, or **Custom**.
2. Enter the application's redirect URI and choose **Create application**. Copy the client ID and the client secret; Halo shows the secret only once.
3. Open the application, go to **Users & groups**, and assign the groups whose members may sign in. Until a group is assigned, nobody can.
4. Configure the application with Halo's issuer, such as `https://auth.example.com`. Applications that support discovery read every endpoint from `https://auth.example.com/.well-known/openid-configuration`.

Each application page in the console shows its connection details and a setup guide with your values filled in. For Grafana, the guide is a block for `grafana.ini`:

```ini
[auth.generic_oauth]
enabled = true
name = Halo
allow_sign_up = true
client_id = hl_…
client_secret = $__file{/etc/grafana/halo-client-secret}
scopes = openid profile email groups
auth_url = https://auth.example.com/oauth2/authorize
token_url = https://auth.example.com/oauth2/token
api_url = https://auth.example.com/oauth2/userinfo
use_pkce = true
groups_attribute_path = groups
role_attribute_path = contains(groups[*], 'Grafana editors') && 'Editor' || 'Viewer'
```

Step-by-step guides cover [Grafana](https://halo.scala.gg/docs/integrations/grafana), [Forgejo](https://halo.scala.gg/docs/integrations/forgejo), [Nextcloud](https://halo.scala.gg/docs/integrations/nextcloud), [Kubernetes with kubelogin](https://halo.scala.gg/docs/integrations/kubernetes), [Proxmox VE](https://halo.scala.gg/docs/integrations/proxmox), [Outline](https://halo.scala.gg/docs/integrations/outline) and [Headscale](https://halo.scala.gg/docs/integrations/headscale) over OpenID Connect, and [AWS IAM Identity Center](https://halo.scala.gg/docs/integrations/aws-iam-identity-center) and [Slack](https://halo.scala.gg/docs/integrations/slack) over SAML. For anything else that supports OpenID Connect or SAML 2.0, start from the [generic OpenID Connect](https://halo.scala.gg/docs/integrations/generic-oidc) or [generic SAML](https://halo.scala.gg/docs/integrations/generic-saml) guide. [`examples/`](examples) has small Go applications that sign in with Halo over each protocol.

To automate Halo itself, create a service account with the user administrator role and an API key, and call the API. This invites a person and returns their setup link:

```bash
curl -X POST https://auth.example.com/api/v1/users \
  -H "Authorization: Bearer $HALO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email": "sam@example.com", "name": "Sam Lee", "department": "Engineering"}'
```

Every route is described in the OpenAPI document at `/api/v1/openapi.yaml`; the [API guide](https://halo.scala.gg/docs/api) covers authentication, roles and errors.

### Applications without OpenID Connect or SAML

Forward auth puts Halo's sign-in in front of applications that have their own login or none, such as admin consoles, file servers and internal tools. The reverse proxy in front of the application asks Halo before every request, and keeps carrying the traffic itself.

1. Choose **Add application**, then **Forward auth**, and enter the address people open the application at, such as `https://files.example.com`. Add more hosts on the **Authentication** tab.
2. Assign groups on **Users & groups**.
3. Copy the Traefik, nginx or Caddy configuration from the application's **Overview** tab. For Caddy:

```caddy
https://files.example.com {
	forward_auth https://auth.example.com {
		uri /forward-auth/app_…/verify
		header_up Host {upstream_hostport}
		copy_headers X-Halo-User X-Halo-Email X-Halo-Name X-Halo-Groups
	}
	reverse_proxy 127.0.0.1:8080
}
```

- People who are signed in and assigned get through, and the application receives `X-Halo-User` (the person's id), `X-Halo-Email`, `X-Halo-Name` and `X-Halo-Groups` (group names separated by commas, with a comma inside a name sent as a space), and `X-Halo-Roles` when the application has roles. Change the names, or add custom attributes, on the **Headers** tab. The proxy replaces any copy of these headers that the browser sends, so make the application reachable only through the proxy.
- Everyone else goes to Halo's sign-in page. Halo checks the group assignment, the sign-in frequency and the access policies, writes the sign-in log, and only sends people back to the application's own addresses.
- Halo never gives its session cookie to the application. After sign-in it sets a separate cookie on each protected host through `/_halo/callback`, with a code that works once and only in the browser that started the sign-in. The cookie lasts as long as the Halo session: signing out, revoking the session, the session limit, suspending the person or removing their access ends it on the next request, and Halo checks the access policies again at least every hour.
- `/_halo/sign-out` on the application's address signs the person out of the application and of Halo.
- Halo reads `X-Forwarded-Host` and `X-Forwarded-Uri` only from addresses in `HALO_TRUSTED_PROXIES`. The web interface replaces `X-Forwarded-Host` with Halo's own host, so through Halo's public address only applications with one protected address work. For more than one, point the proxy at the Halo server directly, such as `http://halo-server:8080` in the Compose setup, and add the proxy's address to `HALO_TRUSTED_PROXIES`.

### VPNs and network devices

Halo has a RADIUS server for VPN gateways, firewalls and network devices that sign people in with a username and password. It is off until you set `HALO_RADIUS_LISTEN`, such as `:1812`, and answers authentication and accounting on that one UDP port.

1. Choose **Add application**, then **RADIUS**, and enter the IP address or CIDR range the device sends from. Halo shows the shared secret once.
2. Assign groups on **Users & groups**.
3. In the device, add a RADIUS server with the Halo server's address, the port and the shared secret. Use PAP, send a Message-Authenticator with every request, and send accounting to the same port.

- People sign in with their email address and Halo password, so passwords must be on under sign-in methods and allowed for them. Accounts that only have passkeys can't sign in over RADIUS.
- When the organization asks for an authenticator app with passwords, Halo answers a correct password with an Access-Challenge, and the device asks for the code. Devices that can't show a challenge can't sign these people in.
- Halo checks the group assignment, suspension, access policies, lockout and risk like any other sign-in, writes the sign-in log, and returns the sign-in frequency, or the organization's session lifetime, as Session-Timeout. Policies and the per-address lockout use the IP address in Calling-Station-Id when the device sends one, and the device's own address otherwise.
- Halo doesn't reply to requests from addresses no RADIUS client lists, to requests signed with another secret, or to Access-Requests without a valid Message-Authenticator, which protects against BlastRADIUS (CVE-2024-3596). Every Access-Accept, Access-Reject and Access-Challenge starts with a Message-Authenticator.
- Accounting stops appear in the sign-in log as ended sessions with the session time.
- Each RADIUS client can send 600 sign-in requests a minute. Changes to its addresses and secrets reach the RADIUS server within 10 seconds.
- Shared secrets expire after a year, like client secrets. Rotate one on the **Credentials** tab before then; the old one keeps working for 24 hours.
- Halo recognizes a RADIUS client by the source address of its packets, so nothing between the device and Halo may rewrite it. The Helm chart's `radius` option sets `externalTrafficPolicy: Local` on its LoadBalancer or NodePort Service for that reason.

### SSH in the browser

People open an SSH terminal to a host in a private network from their account portal, without a client or a VPN. For SSH, Halo never connects to the host network itself: `halo-outpost`, a small agent inside it, keeps one connection open to Halo and reaches the host when a session starts. Halo runs the SSH session, so the host's credentials never reach the outpost or the browser.

1. On **Remote access** in the console, choose **Register outpost** and run the command it shows on a machine that reaches your hosts. The outpost requires an HTTPS URL. The `halo-server` image includes `halo-outpost`, or build it with `go build ./cmd/halo-outpost`.
2. Choose **Add application**, then **Remote access**, and enter the host, its port, its host key from `ssh-keyscan` (or nothing, for hosts with a Halo host certificate) and how Halo signs in: a one-minute certificate from Halo's SSH CA for the person's principals, so nothing is stored, or a password or private key that Halo keeps encrypted.
3. Assign groups. People in them see the host in their account portal and open the terminal there.

Starting a session runs the checks of any other application sign-in: assignment, access policies with passkey requirements and network zones, sign-in frequency and authentication contexts, and it is in the sign-in log. Each session ends at the application's time limit, when an administrator ends it under **Remote access**, or within a minute of the person being suspended, unassigned or signed out. The session log keeps who connected where, when and how much they transferred, and, when recording is on, the terminal output as an asciicast file, for as long as the sign-in log. Outposts and browsers must reach the same Halo instance, as [Running several instances](#running-several-instances) explains.

Configuration imports report new password- or key-authenticated remote applications as skipped. Create those applications on the destination and configure their outpost and credential there.

### RDP and VNC in the browser

1. Run Apache Guacamole 1.6's native proxy, `guacd`, alongside the outpost. Bind its listener to loopback in the outpost's network namespace. If you run the outpost on the host and Guacamole in Docker, publish port 4822 with `-p 127.0.0.1:4822:4822`.
2. Start the outpost with `--guacd-port 4822` or set `HALO_OUTPOST_GUACD_PORT=4822`. The outpost relays the protected connection to its local proxy, which connects to the registered desktop host.
3. Add a **Remote access** application, choose **RDP** or **VNC**, and configure the host and password. For RDP, choose **Network Level Authentication** for servers that require it, or **TLS** for servers such as xrdp. Enter the SHA-256 fingerprint obtained from the RDP server certificate through a trusted administration channel. Use `DOMAIN\username` for a domain account. VNC can leave the username blank.
4. Assign groups. People open the desktop from **Applications** in the account portal. Press **Shift + Escape** to return keyboard focus to Halo's controls.

Halo stores the password encrypted and delivers it to the configured Guacamole proxy over the outpost connection. Desktop sessions use the same policy checks, time limits, termination controls, and session logs as SSH. Clipboard and file transfer are disabled. Terminal recordings apply to SSH. Place VNC hosts and their outpost on a trusted private network.

### Applications that only speak LDAP

Some applications and devices can only check people against an LDAP directory, such as Proxmox VE realms, NAS systems and printers. Halo serves its people and groups to them as a read-only directory on its own port, which is off until you turn it on. While the LDAP server is turned off under Settings, Features, the port closes every connection without answering.

1. Give the server a certificate for Halo's host and set `HALO_LDAP_LISTEN`, such as `:636`, `HALO_LDAP_CERT_FILE` and `HALO_LDAP_KEY_FILE`, both PEM files. The web interface doesn't carry LDAP, so open the port on the server itself.
2. Choose **Add application**, then **LDAP**. Halo shows the server address, the DNs and the bind password, the password only once.
3. Assign groups on **Users & groups**. The application sees only the people in them, and only those groups.

For an application with the ID `app_…` on `auth.example.com`:

| Entry | DN | Contents |
| --- | --- | --- |
| Base | `ou=app_…,dc=auth,dc=example,dc=com` | |
| Bind DN | `cn=bind,ou=app_…,dc=auth,dc=example,dc=com` | The application binds as this with its bind password. |
| People | `uid=<email>,ou=people,ou=app_…,dc=auth,dc=example,dc=com` | `inetOrgPerson`: `uid` and `mail` hold the email address, `cn`, `sn` and `displayName` the name, and `memberOf` the person's groups. Also `posixAccount`: a second `uid` with the Linux user name, `uidNumber`, `gidNumber`, `homeDirectory` and `loginShell`. Active people only. |
| Groups | `cn=<name>,ou=groups,ou=app_…,dc=auth,dc=example,dc=com` | `groupOfNames` with a `member` for each person, and `posixGroup` with `gidNumber` and a `memberUid` for each person's Linux user name. |

- `HALO_LDAP_TLS` is `ldaps` by default. `starttls` serves plain LDAP on the port and refuses binds until the client runs StartTLS. `none` accepts passwords without TLS, so use it only on a network you trust.
- Halo refuses anonymous binds and binds without a password, and answers add, modify, delete, rename, compare and password changes with `unwillingToPerform`. Rotating the bind password on the application's page stops the old one at once.
- Applications that check a person's password by binding as them use the person's DN. Halo applies the checks of its own sign-in page: passwords must be on for the organization and allowed for the person, the account must be active and assigned to the application, the access policies and sign-in risk apply, with the address of the LDAP client, usually the application's server, and no device, failed binds count toward the account lockout, and every bind is in the sign-in log. Halo doesn't make a second password for LDAP, so people without a Halo password, such as passkey-only accounts, are in the directory but can't bind. A bind can't carry an authenticator code either, so person binds fail while **Password rules** ask for one, which is the default.
- A person who bound sees only their own entry. A search returns at most 1,000 entries unless it uses paged results, and stops after its time limit or 30 seconds. Each client address can send 1,200 requests a minute.

### Linux accounts on your servers

Halo gives every person and group a Linux identity: a user name, a `uidNumber` or `gidNumber`, a login shell and a home directory. Halo takes the numbers from the ranges under **Infrastructure access**, 1000000 and up for people and 2000000 and up for groups, keeps them unique across people and groups, never changes them and never hands one out again after a deletion. A person's private group has their user name and number. User administrators change a person's user name, shell and home directory on their **Access** tab.

`halo-hostd` runs as root on each Linux server. It signs in to Halo with the host key enrolled through `halo ssh-host enroll`, fetches the people and groups allowed on that host, and writes them where [libnss-extrausers](https://packages.debian.org/stable/libnss-extrausers) reads them, with their sudo rules in `/etc/sudoers.d/halo`. People log in with the SSH certificate from `halo ssh-cert`, which names `halo:<uidNumber>` once one of their groups may log in to a host group; the server keeps no passwords.

1. On **Infrastructure access**, create a host group, for example Production, with a rule for each group that may log in: full sudo, a list of commands, or no sudo.
2. Enroll the server, then move it into the host group from the Hosts table.
3. On the server, as root, with `halo-hostd` built by `CGO_ENABLED=0 go build ./cmd/halo-hostd`:

```sh
apt install libnss-extrausers
install -m 0755 halo-hostd /usr/local/bin/halo-hostd
sed 's#https://auth.example.com#https://auth.yourcompany.com#' deploy/halo-hostd.service > /etc/systemd/system/halo-hostd.service
sed -i '/^\(passwd\|group\):/ s/$/ extrausers/' /etc/nsswitch.conf
curl -fsS https://auth.yourcompany.com/api/v1/ssh/ca.pub > /etc/ssh/halo_ca.pub
cat >> /etc/ssh/sshd_config <<'EOF'
TrustedUserCAKeys /etc/ssh/halo_ca.pub
AuthorizedPrincipalsCommand /usr/local/bin/halo-hostd principals %u
AuthorizedPrincipalsCommandUser nobody
EOF
echo 'session required pam_mkhomedir.so skel=/etc/skel umask=0077' >> /etc/pam.d/sshd
systemctl enable --now halo-hostd && systemctl reload ssh
```

- Refreshes happen every 5 minutes, or at once after `systemctl reload halo-hostd`. Suspended and deprovisioned people, and people who leave the rule's groups, stop resolving and lose sudo at the next refresh. Their certificates stop working on the host at the same moment, because `halo-hostd principals` no longer names them.
- The cache in `/var/lib/halo-hostd` is readable by root only. While Halo is unreachable, the host keeps the last directory, and a restart writes it again from the cache. A suspended person can't get a new certificate, so their access ends when their certificate does, at most 24 hours later.
- Accounts and groups in the local `/etc/passwd` and `/etc/group` always win: `halo-hostd` leaves out any person or group whose name or number is already there, and never names a principal for a local account. People get `*` as their password, so password logins fail.
- Sudo rules name the person by number, such as `#1000001 ALL=(root) NOPASSWD: ALL`, without a password prompt, because nobody has a password to type. Halo accepts `ALL` or absolute paths with plain arguments, and `halo-hostd` checks them again and installs the file only when `visudo -c` accepts it; otherwise it removes Halo's sudo rules until the next good refresh.
- The systemd unit runs with no capabilities and can write only its cache, `/var/lib/extrausers` and `/etc/sudoers.d`. On distributions without libnss-extrausers, point `--nss` at the directory of another files-based NSS module, such as nss-altfiles.

## Architecture

```mermaid
flowchart LR
    people["People in a browser"] --> web
    apps["Your applications"] --> web
    cli["halo command"] --> web
    subgraph halo [Halo]
        web["Web interface<br>Next.js"] --> server["Halo server<br>Go"]
    end
    server --> db[("PostgreSQL")]
    server -.-> out["SCIM, webhooks<br>and email"]
    server -.-> idp["External identity<br>providers"]
```

The web interface is the only public entry point. It serves the console, the account portal and the sign-in pages, and forwards API and protocol paths to the Go server, so cookies, the OpenID Connect issuer and the passkey domain share one address. The Go server holds the logic, runs background jobs such as email delivery and outbound SCIM, and stores everything in PostgreSQL.

| Path | What it serves |
| --- | --- |
| `/admin`, `/account`, `/sign-in` | The console, the account portal and the sign-in pages |
| `/api/v1/*` | The management and account API, described at `/api/v1/openapi.yaml` |
| `/oauth2/*`, `/.well-known/openid-configuration` | The OpenID Connect provider |
| `/saml/*` | The SAML 2.0 identity provider |
| `/forward-auth/*` | The check reverse proxies run before each request to a forward-auth application, and its sign-in |
| `/scim/v2/*` | The SCIM 2.0 server |

[Architecture](https://halo.scala.gg/docs/architecture) covers the Go packages and how a sign-in flows through them.

## Self-hosting

Self-hosted Halo runs on your own infrastructure and needs no external service. A production installation is four containers: PostgreSQL 17, the Halo server, the web interface, and Caddy for TLS. The [self-hosting guide](https://halo.scala.gg/docs/self-hosting) walks through it step by step.

- **Requirements.** A Linux host with Docker Engine and the Compose plugin, which the install script installs when you run it as root, a domain name, and ports 80 and 443 reachable from the internet. Images are published for `linux/amd64` and `linux/arm64`. On a test machine, the four running containers used about 200 MB of memory at idle.
- **Choose a domain you will keep.** Halo's address is the OpenID Connect issuer that every application stores, and passkeys only work on the domain they were created for.
- **Back up two things.** The database, and `HALO_SECRET_KEY`, which encrypts signing keys and other stored secrets. A backup cannot be restored without the key that was in use when it was taken.
- **One server or several.** A single Halo server is enough for most organizations. To keep sign-in working through restarts, upgrades and host failures, run several, as [Running several instances](#running-several-instances) describes.
- **Email is optional.** Without an SMTP server, administrators pass invite and reset links on themselves. With one, Halo emails them, and people can sign in with magic links.
- **Kubernetes.** A Helm chart in [`deploy/helm/halo`](deploy/helm/halo) deploys the server and the web interface, with an optional PostgreSQL, and reads `HALO_SECRET_KEY` and the database URL from an existing Secret. [Its README](deploy/helm/halo/README.md) covers installation and upgrades.

### Running several instances

Run two or more Halo servers behind a load balancer when sign-in must keep working while one restarts or its host fails. Every instance keeps its state in PostgreSQL, so the load balancer needs no sticky sessions.

- **Same database, same configuration.** Point every instance at one PostgreSQL database and give each the same `HALO_PUBLIC_URL`, `HALO_SECRET_KEY` and Vault settings. Connect directly or through a pooler in session mode: Halo holds PostgreSQL advisory locks and `LISTEN` on dedicated connections, which transaction pooling breaks. Each instance opens two connections for this beside its pool, and a third while it pushes changes over outbound SCIM.
- **Background jobs run in one instance at a time.** The instance that holds an advisory lock runs email delivery, outbound SCIM, lifecycle rules, access expiry, webhook and log deliveries and the other jobs; the others check every 2 seconds. When it stops, another takes over within about 2 seconds, and within about 30 seconds when its host or network disappears.
- **Shared state.** Sessions, sign-in ceremonies, authorization codes and tokens, rate limits, signing keys, and the replay checks for authenticator codes, DPoP proofs, SAML assertions, Kerberos authenticators and RADIUS retransmissions are rows in the database. A settings change reaches every instance at once through `LISTEN`/`NOTIFY`; if that connection drops, an instance reads settings again at least every 30 seconds.
- **Migrations.** An instance applies migrations under an advisory lock before it listens, so instances that start together wait for each other instead of migrating twice.
- **Health checks.** `/healthz` answers `ok` while the process runs and reaches the database. `/readyz` answers `ok` once migrations are applied, the background workers have started and the instance listens for settings changes, and `503` while it starts or stops. Point the load balancer's health check at `/readyz`.
- **Upgrades.** Back up the database and `HALO_SECRET_KEY`, then restart the instances one at a time, and wait until `/readyz` answers `ok` on each before the next. On `SIGTERM` an instance reports `503` on `/readyz`, stops accepting connections, finishes the requests in progress for up to 10 seconds, and releases the jobs lock.
- **RADIUS and LDAP.** Every instance can serve RADIUS and LDAP. RADIUS clients are recognized by their source address, so the load balancer must keep it.
- **Remote access.** An outpost holds its connection to one instance, and a browser session works only on that instance. With several instances, a session that reaches another one fails to start with `ERR_OUTPOST_OFFLINE`, so run remote access with a single instance for now. Ending a session from the console, and deleting or enrolling an outpost again, still reach the instance that holds it, within a few seconds.

With Kubernetes, set `server.replicaCount` in the [Helm chart](deploy/helm/halo/README.md).

### Windows sign-in with Kerberos

People on Windows computers joined to an Active Directory domain can sign in without typing anything: the browser sends Halo the Kerberos ticket Windows already holds (SPNEGO, RFC 4559). It is off until you set it up under **Methods**.

1. Create a service account used only by Halo, such as `svc-halo`, with **This account supports Kerberos AES 256 bit encryption** on, and register Halo's service principal on it. The host name is the one in `HALO_PUBLIC_URL`:

   ```powershell
   setspn -S HTTP/auth.example.com CORP\svc-halo
   ```

2. Export its keytab. `ktpass` sets a new password on the account, and asks for it:

   ```powershell
   ktpass /princ HTTP/auth.example.com@CORP.EXAMPLE.COM /mapuser CORP\svc-halo /crypto AES256-SHA1 /ptype KRB5_NT_PRINCIPAL /pass * /out halo.keytab
   ```

3. Under **Methods**, in **Windows sign-in**, enter `HTTP/auth.example.com@CORP.EXAMPLE.COM`, map each realm to the email domain of its people, such as `CORP.EXAMPLE.COM` to `example.com`, upload `halo.keytab` and save. Then turn on **Windows sign-in** in the list of methods above it.
4. Let browsers send tickets to Halo. For Edge and Chrome on Windows, add `https://auth.example.com` to the Local intranet zone with the Group Policy **Site to Zone Assignment List**, or set the `AuthServerAllowlist` policy to `auth.example.com`. For Firefox, set `network.negotiate-auth.trusted-uris` to `https://auth.example.com`.

- Halo keeps only the AES and RC4 keys for the service principal from the keytab, encrypted with `HALO_SECRET_KEY`, and never shows them again. Upload a new keytab whenever the service account's password changes, because its key version changes with it.
- A ticket for `ada@CORP.EXAMPLE.COM` signs in the person whose email address is `ada@example.com`, using the domain mapped to the realm. The principal is the account's `sAMAccountName`, so this works when email addresses use the same name. Halo links the principal to that person on their first successful sign-in and goes by the link afterwards. It never matches by display name, and refuses tickets from realms you didn't map and from machine or service principals.
- Halo checks the ticket's lifetime and the authenticator's time with 5 minutes of clock skew, refuses an authenticator it already accepted on any Halo server, because the replay cache is in PostgreSQL, refuses NTLM, and refuses tickets for any other service principal. Give Halo's service principal its own account: every service principal on an account shares the account's key.
- The sign-in page shows **Sign in with Windows** while the method is on. It only tries it without a click from the trusted network zones you choose under **Try automatically from**, which is empty by default, so browsers elsewhere never see the Negotiate challenge. Only choose zones where every computer is joined to the domain, because a browser that can't send a ticket may ask for a Windows user name and password instead.
- The account status, group assignment, access policies, sign-in risk and lockout apply like any other sign-in, and every ticket Halo receives is in the sign-in log, with the reason when it fails. Access policies can target the method `kerberos`, and each application can turn Windows sign-in off on its Authentication tab, which also blocks single sign-on to it from a session that started with Windows sign-in.
- The reverse proxy in front of Halo must pass the `Authorization` request header and the `WWW-Authenticate` response header through unchanged, and accept request headers of up to 64 KB: tickets for people in many groups are large. The web interface image starts Node with `--max-http-header-size=131072` for this; start it the same way if you run it yourself. Kerberos sign-in can be turned off under Settings, Features.

### Metrics

The server can expose Prometheus metrics at `/metrics`. They are off until you set one of these:

- `HALO_METRICS_LISTEN`, such as `:9090`, serves `/metrics` on that address only. Keep the port off the internet.
- `HALO_METRICS_TOKEN` requires `Authorization: Bearer <token>`. On its own it serves `/metrics` on `HALO_LISTEN`; with `HALO_METRICS_LISTEN` it protects that address too. Generate one with `openssl rand -hex 32`.

Labels never hold people, email addresses, IP addresses or client IDs, and HTTP routes are the route pattern, such as `GET /api/v1/users/{id}`, not the requested path.

| Series | Labels |
| --- | --- |
| `halo_sign_in_events_total` | `method`, `result` (`success`, `failure`, `interrupted`, `ended`), `risk` |
| `halo_lockouts_total` | `scope`: `account`, or `address` for too many failed passwords from one IP address |
| `halo_tokens_issued_total` | `grant`: `authorization_code`, `refresh_token`, `device_code`, `client_credentials`, `token_exchange`, `workload_identity` |
| `halo_refresh_tokens_refused_total` | |
| `halo_scim_outbound_runs_total` | `result`: `success`, `failure` |
| `halo_scim_outbound_records_total` | `resource`: `user`, `group`; `action`: `created`, `updated`, `deactivated`, `deleted` |
| `halo_ldap_sync_runs_total` | `result`: `success`, `failure` |
| `halo_ldap_sync_changes_total` | `action`: `created`, `updated`, `suspended`, `restored`, `group_created`, `group_renamed`, `member_added`, `member_removed` |
| `halo_webhook_deliveries_total` | `outcome`: `delivered`, `pending` (retried later), `failed` (out of retries) |
| `halo_log_export_events_total` | `outcome`: `sent`, `failed` (retried later), `rejected` (refused by the collector), `dropped` (queue full) |
| `halo_rate_limited_requests_total` | `scope`: `api`, `scim`, `bulk`, `radius`, `ldap` |
| `halo_radius_requests_total` | `outcome`: `accept`, `reject`, `challenge`, `accounting`, `dropped` (no reply: unknown client, missing or wrong Message-Authenticator, malformed packet, rate limit or server error) |
| `halo_ldap_binds_total` | `kind`: `application`, `person`, or `other` for anonymous binds and unknown DNs; `result`: `success`, `failure` |
| `halo_ldap_searches_total` | `result`: `success`, `failure` |
| `halo_http_requests_total`, `halo_http_request_duration_seconds` | `route`, plus `status` on the counter |
| `halo_kms_request_duration_seconds`, `halo_kms_errors_total` | `operation`: `decrypt`, `create`, `read`, `sign`, `ping`, `token` (credentials from instance metadata or an OAuth token endpoint), `renew` (Vault token renewal); requests to the [external key manager](#keys-in-an-external-key-manager) |
| `halo_db_pool_acquired_connections`, `_idle_`, `_total_`, `_max_connections` | |
| `go_goroutines`, `go_memstats_heap_alloc_bytes`, `go_memstats_sys_bytes`, `go_gc_cycles_completed` | |

Example alert rules:

```yaml
groups:
  - name: halo
    rules:
      - alert: HaloFailedSignInSpike
        expr: sum(increase(halo_sign_in_events_total{result="failure"}[10m])) > 50
      - alert: HaloSCIMFailing
        expr: sum(increase(halo_scim_outbound_runs_total{result="failure"}[1h])) > 0 unless sum(increase(halo_scim_outbound_runs_total{result="success"}[1h])) > 0
      - alert: HaloWebhookGaveUp
        expr: sum(increase(halo_webhook_deliveries_total{outcome="failed"}[15m])) > 0
      - alert: HaloLogExportFailing
        expr: sum(increase(halo_log_export_events_total{outcome=~"failed|rejected|dropped"}[15m])) > 0
      - alert: HaloLockouts
        expr: sum(increase(halo_lockouts_total[15m])) > 10
      - alert: HaloKeyManagerFailing
        expr: sum(increase(halo_kms_errors_total[5m])) > 0
```

### Log streaming

Halo can stream every sign-in event and audit event, with the audit event's changes, to a log collector or SIEM. Set it up under Monitoring, Log streaming, or with environment variables, which then take precedence and make the console read-only:

- `HALO_LOG_EXPORT_PROTOCOL`: `syslog-tls` for RFC 5424 over TCP with TLS, `syslog-udp` for lab use, or `otlp-http` for an OpenTelemetry collector over OTLP/HTTP with JSON.
- `HALO_LOG_EXPORT_ENDPOINT`: `host:port` for syslog, such as `siem.example.com:6514`, or the collector URL, such as `https://otel.example.com:4318/v1/logs`.
- `HALO_LOG_EXPORT_HEADERS`: OTLP headers as `name=value` pairs separated by commas, such as `Authorization=Bearer <token>`. Halo stores them encrypted and never shows them again.
- `HALO_LOG_EXPORT_ALLOW_PRIVATE=1`: send to private, loopback and link-local addresses, and allow `http://` for OTLP. Without it Halo only connects to public addresses.

Each event is the same JSON as a webhook's, in the syslog message or the OTLP log record body, with secret values already redacted from audit changes. Syslog messages use the `authpriv` facility, the `halo` app name and the event type as the message ID. Halo sends new events every 10 seconds, starting with those recorded after the exporter is first saved. When the collector is down, events wait in the database and Halo retries with a growing delay, up to 5 minutes, so sign-ins never wait for the collector. The console shows the last successful send and how many events are waiting. When more than 100,000 events of one kind are waiting, the oldest are dropped, and an outage longer than the log retention loses the events the retention deletes. To trust a private certificate authority for syslog over TLS, point `SSL_CERT_FILE` at a bundle that holds the system certificates and your authority, since it replaces the system bundle for every connection Halo makes.

### Keys in an external key manager

Halo can keep its keys in HashiCorp Vault Transit, AWS KMS, Google Cloud KMS, Azure Key Vault or a PKCS #11 hardware security module, in two ways, separately or together:

- **Wrap `HALO_SECRET_KEY`.** The environment then holds only the key encrypted by the key manager, so a copy of the environment and a database backup are not enough to read stored secrets. Halo decrypts it once when it starts and doesn't start while the key manager is unreachable. Restoring a backup then also needs the wrapping key.
- **Sign inside the key manager.** New OpenID Connect, SAML and SSH user and host certificate authority keys are created there as keys that can't be exported. Halo sends what it needs signed, never holds the private key, and publishes the public key it reads back. Halo starts only when it can authenticate to the key manager and read every external key it signs with. While the key manager is unreachable, requests that need a signature fail; Halo never falls back to a key of its own. Authentication requests to upstream SAML identity providers are signed with the SAML key wherever it lives.

Set one key manager. Each has a setting that selects it and two that work the same everywhere, `HALO_<manager>_WRAPPING_KEY` and `HALO_<manager>_SIGNING=1`. With signing on, new keys are created in the key manager. Existing keys stay in the database until they are rotated: rotate the OpenID Connect keys and the SAML certificate under Settings, Security defaults, and the console shows where each of those keys lives. The SSH user and host certificate authorities have no rotation, so each is created in the key manager only on an installation that doesn't have it yet.

Not every key manager signs with every algorithm. With signing on, Halo refuses to set an application's ID tokens or an API's access tokens to an algorithm the key manager can't sign with, and doesn't start while one is set:

| Key manager | RS256 | ES256 | EdDSA | SSH certificate authorities |
| --- | --- | --- | --- | --- |
| Vault Transit | Yes | Yes | Yes | Ed25519 |
| AWS KMS | Yes | Yes | Yes, for tokens up to 4096 bytes | Ed25519 |
| Google Cloud KMS | Yes | Yes | Software keys only | Ed25519, or ECDSA P-256 with `HALO_GCP_KMS_HSM=1` |
| Azure Key Vault | Yes | Yes | No | ECDSA P-256 |
| PKCS #11 | Yes | Yes | If the token supports `CKM_EDDSA` | Ed25519 |

Every key manager reports to `halo_kms_request_duration_seconds` and `halo_kms_errors_total`. Halo calls Vault and the cloud key managers with the Go standard library, follows no redirects and waits at most 10 seconds for an answer. Their addresses must use https, except to the loopback address or with `HALO_DEV=1`; only the cloud metadata services are reached over plain http.

#### HashiCorp Vault

- `HALO_VAULT_ADDR`: Vault's address, such as `https://vault.example.com:8200`. Plain `http` is accepted only to the loopback address, such as a Vault Proxy at `http://127.0.0.1:8100`, or with `HALO_DEV=1`.
- `HALO_VAULT_TOKEN`: a token with the policy below. Halo renews it with `auth/token/renew-self` after two thirds of each TTL and logs failed renewals. Vault never extends a token past its maximum TTL, so use a periodic token or give Halo a new token before then. Instead of a token, you can run [Vault Proxy](https://developer.hashicorp.com/vault/docs/agent-and-proxy/proxy/apiproxy) next to Halo with auto-auth and `api_proxy { use_auto_auth_token = true }`, point `HALO_VAULT_ADDR` at it and leave `HALO_VAULT_TOKEN` unset.
- `HALO_VAULT_NAMESPACE`: the namespace, on Vault Enterprise and HCP Vault Dedicated.
- `HALO_VAULT_TRANSIT_MOUNT`: where Transit is mounted, `transit` by default.
- `HALO_VAULT_WRAPPING_KEY`: a Transit encryption key, such as `halo-secret-key`. `HALO_SECRET_KEY` then holds the key encrypted by Vault. Create a new one with `vault write -field=ciphertext transit/encrypt/halo-secret-key plaintext="$(openssl rand -base64 32)"`, or wrap the key of an existing installation with `plaintext="$HALO_SECRET_KEY"`. To rotate the wrapping key, run `vault write -f transit/keys/halo-secret-key/rotate`, then replace `HALO_SECRET_KEY` with the output of `vault write -field=ciphertext transit/rewrap/halo-secret-key ciphertext=<current value>`; stored secrets stay as they are. `halo rotate-secret-key` reads `HALO_NEW_SECRET_KEY` in the same encrypted form, for every key manager.
- `HALO_VAULT_SIGNING=1`: create new signing keys in Vault, named `halo-key_` and an ID.

The token needs this policy, with the wrapping key's name in the first path:

```hcl
path "transit/decrypt/halo-secret-key" {
  capabilities = ["update"]
}
path "transit/keys/halo-key_*" {
  capabilities = ["create", "read", "update"]
}
path "transit/sign/halo-key_*" {
  capabilities = ["update"]
}
path "auth/token/lookup-self" {
  capabilities = ["read"]
}
path "auth/token/renew-self" {
  capabilities = ["update"]
}
```

#### AWS KMS

- `HALO_AWS_KMS_REGION`: the Region, such as `eu-central-1`.
- `HALO_AWS_KMS_ENDPOINT`: another endpoint, such as `https://kms-fips.us-east-1.amazonaws.com` or a VPC endpoint.
- Credentials: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and, for temporary credentials, `AWS_SESSION_TOKEN`. Without them Halo uses the EC2 instance role through IMDSv2; `AWS_EC2_METADATA_SERVICE_ENDPOINT` changes the metadata address. Halo signs requests with Signature Version 4 itself and doesn't read `~/.aws`, ECS task roles, EKS Pod Identity or IAM roles for service accounts.
- `HALO_AWS_KMS_WRAPPING_KEY`: a symmetric encryption key's ID, ARN or alias, such as `alias/halo-secret-key`. `HALO_SECRET_KEY` then holds the ciphertext blob, base64 encoded: `aws kms encrypt --key-id alias/halo-secret-key --plaintext fileb://<(openssl rand 32) --query CiphertextBlob --output text`. Halo names the key when it decrypts, so a blob from another key is refused. Rotating the KMS key needs no new blob.
- `HALO_AWS_KMS_SIGNING=1`: create new signing keys as `RSA_2048`, `ECC_NIST_P256` or `ECC_NIST_EDWARDS25519` keys for `SIGN_VERIFY`, tagged `halo=signing-key`. Halo stores each key's ARN and refuses keys with imported key material. AWS KMS signs at most 4096 bytes with Ed25519, so an EdDSA ID token with many claims fails; use RS256 or ES256 for those applications.

The credentials need this IAM policy, with the wrapping key's ARN in the first statement:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "kms:Decrypt", "Resource": "arn:aws:kms:eu-central-1:111122223333:key/WRAPPING-KEY-ID" },
    { "Effect": "Allow", "Action": "kms:CreateKey", "Resource": "*" },
    { "Effect": "Allow", "Action": "kms:TagResource", "Resource": "*", "Condition": { "StringEquals": { "aws:RequestTag/halo": "signing-key" } } },
    { "Effect": "Allow", "Action": ["kms:DescribeKey", "kms:GetPublicKey", "kms:Sign"], "Resource": "*", "Condition": { "StringEquals": { "aws:ResourceTag/halo": "signing-key" } } },
    { "Effect": "Allow", "Action": "kms:ListKeys", "Resource": "*" }
  ]
}
```

#### Google Cloud KMS

- `HALO_GCP_KMS_KEY_RING`: the key ring, such as `projects/acme/locations/europe-west1/keyRings/halo`.
- `HALO_GCP_KMS_HSM=1`: create signing keys at the `HSM` protection level instead of `SOFTWARE`. Cloud HSM has no Ed25519, so EdDSA isn't available and the SSH certificate authorities use ECDSA P-256.
- `HALO_GCP_KMS_ENDPOINT`: another endpoint, such as a Private Service Connect address.
- Credentials: `GOOGLE_APPLICATION_CREDENTIALS`, the path of a service account key file. Without it Halo asks the metadata server for the attached service account's token, on Compute Engine, Cloud Run, and GKE with Workload Identity; `GCE_METADATA_HOST` changes the metadata address. Halo doesn't read user credentials or workload identity federation files.
- `HALO_GCP_KMS_WRAPPING_KEY`: an encryption key in the key ring, such as `halo-secret-key`. `HALO_SECRET_KEY` then holds the ciphertext, base64 encoded: `openssl rand 32 | gcloud kms encrypt --project acme --location europe-west1 --keyring halo --key halo-secret-key --plaintext-file - --ciphertext-file - | base64 -w0`. Rotating the key needs no new ciphertext.
- `HALO_GCP_KMS_SIGNING=1`: create new signing keys in the key ring, named `halo-key_` and an ID, with the `RSA_SIGN_PKCS1_2048_SHA256`, `EC_SIGN_P256_SHA256` or `EC_SIGN_ED25519` algorithm. Halo stores each key version's name and refuses imported versions.

Grant the service account `roles/cloudkms.cryptoKeyDecrypter` on the wrapping key and, on the key ring, a custom role with `cloudkms.keyRings.get`, `cloudkms.cryptoKeys.create`, `cloudkms.cryptoKeyVersions.get`, `cloudkms.cryptoKeyVersions.viewPublicKey` and `cloudkms.cryptoKeyVersions.useToSign`.

#### Azure Key Vault

- `HALO_AZURE_KEY_VAULT_URL`: the vault, such as `https://acme-halo.vault.azure.net`, or a Managed HSM, such as `https://acme-halo.managedhsm.azure.net`. Halo asks for tokens for the vault's domain and sends them only to this address.
- `HALO_AZURE_KEY_VAULT_HSM=1`: create `RSA-HSM` and `EC-HSM` keys, on a Premium vault or a Managed HSM.
- Credentials: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID` and `AZURE_CLIENT_SECRET` for an app registration, or `AZURE_FEDERATED_TOKEN_FILE` instead of the secret with AKS workload identity. Without them Halo uses a managed identity: through `IDENTITY_ENDPOINT` and `IDENTITY_HEADER` on App Service and Container Apps, or the instance metadata service on virtual machines. `AZURE_CLIENT_ID` then selects a user-assigned identity. `AZURE_AUTHORITY_HOST` changes the sign-in address for national clouds.
- `HALO_AZURE_KEY_VAULT_WRAPPING_KEY`: an RSA key and its version, such as `halo-secret-key/0123456789abcdef0123456789abcdef`. `HALO_SECRET_KEY` then holds 32 random bytes encrypted with RSA-OAEP-256 to that key, base64 encoded: `az keyvault key download --id https://acme-halo.vault.azure.net/keys/halo-secret-key/0123456789abcdef0123456789abcdef --file halo-wrapping-key.pem && openssl rand 32 | openssl pkeyutl -encrypt -pubin -inkey halo-wrapping-key.pem -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256 | base64 -w0`. After rotating the key, keep the old version enabled until `HALO_SECRET_KEY` is wrapped to the new one.
- `HALO_AZURE_KEY_VAULT_SIGNING=1`: create new signing keys as RSA 2048 or EC P-256 keys that can only sign and verify, named `halo-key-` and an ID. Key Vault has no Ed25519, so EdDSA isn't available and the SSH certificate authorities use ECDSA P-256. Halo stores each key's identifier and refuses exportable keys.

With Azure RBAC, grant the identity `Key Vault Crypto User` on the vault for wrapping, or `Key Vault Crypto Officer`, which can also create keys, when Halo signs inside Key Vault. With access policies, grant the key permissions `get`, `list`, `create`, `sign` and `unwrapKey`.

#### PKCS #11

PKCS #11 needs cgo and a server built with `CGO_ENABLED=1 go build -tags pkcs11 ./cmd/halo`. The published images are built without it, and a server without it refuses to start with these settings.

- `HALO_PKCS11_MODULE`: the vendor's PKCS #11 library, such as `/usr/lib/softhsm/libsofthsm2.so`.
- `HALO_PKCS11_TOKEN_LABEL` and `HALO_PKCS11_PIN`: the token and its user PIN.
- `HALO_PKCS11_WRAPPING_KEY`: the label of an RSA private key on the token that may decrypt. `HALO_SECRET_KEY` then holds 32 random bytes encrypted with RSA-OAEP and SHA-256 to its public key, base64 encoded: `openssl rand 32 | openssl pkeyutl -encrypt -pubin -inkey halo-wrapping-key.pem -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256 | base64 -w0`. SoftHSM 2.6 can't decrypt RSA-OAEP with SHA-256.
- `HALO_PKCS11_SIGNING=1`: create new signing key pairs on the token, labelled `halo-key_` and an ID, with a sensitive private key that can't be extracted. Halo refuses keys the token lets be extracted. Halo uses one session, opens a new one after the token drops it, and signs one request at a time.

## Roadmap and limitations

- Each Halo installation serves one organization. You can run [several server instances](#running-several-instances) against the same database. Follow [multi-organization support](https://github.com/ScalaStudios/Halo/issues/25) for shared installations.
- FAPI 2.0 support is partial. Mutual TLS client authentication remains open in [#76](https://github.com/ScalaStudios/Halo/issues/76).
- The [conformance report](CONFORMANCE.md) records the OpenID Foundation test results. OpenID certification and the [independent security audit](https://github.com/ScalaStudios/Halo/issues/28) remain open.

[CHANGELOG.md](CHANGELOG.md) records what has shipped.

## Documentation

The documentation lives at [halo.scala.gg/docs](https://halo.scala.gg/docs).

- [Get started](https://halo.scala.gg/docs/getting-started): run Halo locally and sign in to an example application through it.
- [Concepts](https://halo.scala.gg/docs/concepts): users, groups, roles, applications, sessions and sign-in methods.
- [Configuration](https://halo.scala.gg/docs/configuration): every environment variable and console setting.
- [Access policies](https://halo.scala.gg/docs/policies) and [governance](https://halo.scala.gg/docs/governance): who gets in, under which conditions, and for how long.
- [Federation](https://halo.scala.gg/docs/federation): sign-in with Google, Microsoft Entra ID, GitHub or another OpenID Connect provider.
- [Provisioning](https://halo.scala.gg/docs/provisioning): SCIM in both directions, service accounts and API keys.
- [Webhooks](https://halo.scala.gg/docs/webhooks) and the [API](https://halo.scala.gg/docs/api): automate Halo and react to its events.
- [Infrastructure access](https://halo.scala.gg/docs/infrastructure-access): SSH certificates with `halo login` and `halo ssh-cert`.
- [Security model](https://halo.scala.gg/docs/security-model): how Halo protects sessions, secrets and tokens, and its known limitations.

## Contributing

Bug reports, fixes, documentation and features are welcome. Report bugs and propose features in [GitHub issues](https://github.com/ScalaStudios/Halo/issues), and open an issue before starting anything large, such as a new feature, a new dependency or a schema change. [CONTRIBUTING.md](CONTRIBUTING.md) covers the development setup, the checks CI runs and the code style, and [Architecture](https://halo.scala.gg/docs/architecture) maps the code. Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Security

Report vulnerabilities privately through [GitHub's private vulnerability reporting](https://github.com/ScalaStudios/Halo/security/advisories/new), never in a public issue. [SECURITY.md](SECURITY.md) lists what to include and the response times to expect.

Halo stores session tokens, client secrets, API keys and refresh tokens only as SHA-256 hashes, encrypts signing keys and other stored secrets with AES-256-GCM, can keep that key wrapped by and the signing keys inside [Vault, a cloud KMS or an HSM](#keys-in-an-external-key-manager), writes every administrative change to the audit log in the same transaction, and refuses to start without HTTPS outside development. The [security model](https://halo.scala.gg/docs/security-model) explains each of these and lists the known limitations.

## License

Halo is licensed under the [Apache License 2.0](LICENSE). Two React Bits components in `web/src/components/reactbits/` keep their own MIT + Commons Clause terms; [NOTICE](NOTICE) has the details.

Halo is a Scala Studios project.

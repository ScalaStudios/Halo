# Halo

Halo is a free and open-source identity platform: single sign-on, passkeys, access governance and zero-trust access policies that you can host yourself.

> **Status: pre-release.** Halo has not published a release yet. Sign-in, single sign-on over OpenID Connect and SAML, the console, the account portal, access policies, governance, federation, SCIM provisioning, webhooks and SSH certificates work on `main`; see [What works today](#what-works-today) and [CHANGELOG.md](CHANGELOG.md).

## What Halo is for

Most teams end up with sign-in spread across a dozen self-hosted and SaaS tools, each with its own accounts, its own MFA rules and its own idea of who has access. Halo is meant to be the one place that answers three questions:

- **Who is this?** One directory of people, groups, service accounts and devices.
- **How did they prove it?** Passkeys and security keys first, authenticator apps where you allow them. Every sign-in recorded with its method, result and risk.
- **What can they reach, and should they still?** Applications assigned through groups, conditional access policies, requestable access with approvers and expiry, and periodic access reviews.

The complete platform is meant to be self-hostable with no paid tier required for core features, no dependency on proprietary infrastructure, and full export of users, groups, applications and policies.

## What works today

| Area | Today |
| --- | --- |
| Authentication | Passkeys and security keys (WebAuthn), authenticator apps (TOTP), recovery codes, magic links by email, invite and reset links, and sign-in with Google, Microsoft Entra ID, GitHub or any OpenID Connect provider, with account linking and accounts created on first sign-in |
| Single sign-on | OpenID Connect provider: authorization code with PKCE, refresh tokens with rotation, client credentials, device flow for the Halo CLI, userinfo, introspection, revocation and RP-initiated logout. SAML 2.0 identity provider with metadata import and IdP-initiated launch |
| API protection | API resources with their own scopes and audiences, and JWT access tokens that APIs validate themselves |
| Directory | Users with profiles and managers, assigned and rule-based groups (`user.department == "Engineering"`), administrative roles, service accounts with scoped API keys |
| Provisioning | SCIM 2.0 server for Okta, Microsoft Entra ID and HR systems, and outbound SCIM that pushes assigned people to applications |
| Access policies | Conditional access by person, group, application, network zone, device trust, sign-in risk and method, with report-only mode, risk events and a simulator |
| Governance | Access packages with approvers and time limits, access reviews, and joiner, mover and leaver rules |
| Infrastructure access | SSH certificate authority, group-to-principal mappings, and `halo login`, `halo ssh-cert`, `halo whoami` and `halo logout` |
| Operations | Email through SMTP with an outbox and retries, signed webhooks for audit and sign-in events, organization settings and branding, verified domains, JSON export, and an OpenAPI 3.1 document at `/api/v1/openapi.yaml` |
| Console | Overview, users, groups, service accounts, devices, applications, API resources, provisioning, roles, policies, infrastructure access, governance, methods, identity providers, sessions, sign-in log, audit log, risk events, API keys, webhooks and settings |
| Account portal | Profile, passkeys, security keys, authenticator apps, recovery codes, linked accounts, sessions, applications, access requests and approvals, access reviews and activity |

Planned and not built yet: LDAP sync and pushing groups over outbound SCIM.

## Run it locally

Requirements: Go 1.27, [Bun](https://bun.sh) 1.3 (or Node.js 22+), Docker.

```bash
docker compose -f compose.dev.yml up -d
cp .env.example .env
```

Set `HALO_SECRET_KEY` in `.env` to the output of `openssl rand -base64 32`, then load the variables and create the first administrator:

```bash
set -a && . ./.env && set +a
go run ./cmd/halo bootstrap --email you@example.com --name "Your Name"
```

Start the server and the web interface in two terminals:

```bash
go run ./cmd/halo serve
```

```bash
cd web && bun install && bun run dev
```

Open the setup link that `bootstrap` printed to create your passkey. The console is at http://localhost:3200/admin and your account at http://localhost:3200/account.

To explore with sample data instead, run `go run ./cmd/halo seed-demo` on an empty database. It loads a 68-person organization called Fernway Systems. Its passkeys are placeholders, so sign in as one of its users with `HALO_DEV=1 go run ./cmd/halo dev-session --email luna@example.com` and set the printed `halo_session` cookie for localhost.

## Integrate your first application

1. In the console, open **Applications → Add application → OpenID Connect → Web**, enter the application's redirect URI and assign it to a group.
2. Copy the issuer, client ID and client secret from the last step. The client secret is shown once.
3. Point the application at the discovery document: `https://<your Halo address>/.well-known/openid-configuration`.

[`examples/go-web-client`](examples/go-web-client) is an 80-line Go app that signs in with Halo this way. Each application page in the console also shows a ready-made configuration snippet for that application.

## How it fits together

```
browser ──► web (Next.js) ──► halo serve (Go) ──► PostgreSQL
            /admin /account     /api/v1/*   /oauth2/*  /.well-known/*
            /sign-in /enroll    /saml/*     /scim/v2/*
            /device
```

The web interface is the public entry point and forwards API and protocol paths to the Go server, so cookies, the OpenID Connect issuer and the WebAuthn origin share one address. Halo stores sessions, client secrets, refresh tokens, enrollment and magic links and API keys as SHA-256 hashes, and recovery codes as HMAC-SHA256 with a key derived from `HALO_SECRET_KEY`; signing keys, the SSH certificate authority, authenticator seeds, identity provider and webhook secrets, SCIM tokens and queued email are encrypted with `HALO_SECRET_KEY`.

| Path | What it serves |
| --- | --- |
| `/api/v1/*` | The management and account API used by the console and account portal; reference at `/api/v1/openapi.yaml` |
| `/oauth2/*`, `/.well-known/openid-configuration` | The OpenID Connect provider |
| `/saml/*` | The SAML 2.0 identity provider |
| `/scim/v2/*` | The SCIM 2.0 server for inbound provisioning |
| `/device` | The confirmation page for `halo login` |
| `/healthz` | Health check on the Go server |

[documentation/api.md](documentation/api.md) lists the API's route groups.

## Tests

```bash
go test ./...
```

The Go tests create throwaway databases on the development Postgres. The end-to-end tests use a virtual authenticator in Chromium to enroll passkeys and sign in to the OpenID Connect and SAML examples, and cover magic links, access requests, access policies and every console page at desktop and phone widths. They need both servers running:

```bash
cd web && bun run e2e
```

## Repository layout

```
cmd/halo/            The halo command: serve, migrate, bootstrap, seed-demo, dev-session, rotate-secret-key, and the client commands login, whoami, ssh-cert, logout
internal/            Server packages: store, auth, oidc, saml, scim, api, policy, governance, provisioning, apikeys, settings, webhooks, sshca, mail, cli, httpx, config
api/                 The OpenAPI document, embedded in the binary
migrations/          SQL migrations, embedded in the binary
web/                 Next.js interface: console, sign-in, enrollment, device approval, account portal
examples/            Applications that sign in with Halo over OpenID Connect and SAML
documentation/       User and operator documentation
```

## License

Halo is licensed under the [Apache License 2.0](LICENSE). The two React Bits components in `web/src/components/reactbits/` are distributed under their own MIT + Commons Clause terms; see [NOTICE](NOTICE).

Developed by Scala Studios.

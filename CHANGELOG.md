# Changelog

All notable changes to Halo are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Halo has not published a release yet; versions will follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it does.

## Unreleased

### Added

- Sign-in without passwords: passkeys and security keys (WebAuthn discoverable credentials), authenticator apps (TOTP), and single-use recovery codes.
- Invite and reset links that work once and expire after the setup link lifetime, 7 days by default, and resetting a person's authentication from the console.
- Email over SMTP for invitations, resets, magic links and access request notices, with an outbox in the database, retries, and `GET /api/v1/dev/outbox` in development.
- Magic sign-in links that work once for 10 minutes, limited to 5 per person in 15 minutes.
- Federated sign-in with Google, Microsoft Entra ID, GitHub and generic OpenID Connect providers: PKCE, verified email matching, allowed email domains, accounts created on first sign-in with default groups, and linked accounts people can unlink in the account portal.
- OpenID Connect provider: authorization code flow with PKCE (required for single-page and native applications), refresh tokens with rotation, client credentials for service applications, userinfo, token introspection and revocation, RP-initiated logout that ends the Halo session, discovery and signing keys. ID tokens are signed with RS256 and carry a `groups` claim with the names of the person's groups.
- `prompt=login`, `prompt=none` and `max_age`, and a completed sign-in can only be redeemed in the browser session that completed it.
- SAML 2.0 identity provider: metadata, SP- and IdP-initiated sign-in, signed assertions, NameID formats, attribute names, and service provider metadata import, with guides for AWS IAM Identity Center and Slack and an example service provider in `examples/go-saml-sp`.
- API resources: your own APIs with scopes, identifiers and token lifetimes, grants to applications, the `resource` parameter, and JWT access tokens with the API in `aud`.
- Access policies: conditions on people, groups, applications, network zones, device trust, sign-in risk and sign-in method; allow, block and require-a-passkey effects; report-only mode with 24-hour activity; and a simulator.
- Network zones, device tracking with trust levels, sign-in risk signals (risky network, repeated failures, new device, new network), and risk events that security administrators resolve or dismiss.
- Turning sign-in methods on and off for the organization.
- Access packages with approvers, maximum durations and justifications; access requests with email notices, approval, denial, cancellation, expiry and revocation; access reviews with automatic removal and overdue tracking; and joiner, mover and leaver lifecycle rules with run history.
- Service accounts with roles and owners, and API keys with `api` and `scim` scopes and optional expiry.
- SCIM 2.0 server under `/scim/v2` for users and assigned groups, tested against the Okta and Microsoft Entra ID request shapes.
- Outbound SCIM provisioning that creates, updates and deactivates the people assigned to an application every 2 minutes or on demand.
- Webhooks for audit and sign-in events, signed with HMAC-SHA256, with retries and a delivery log.
- SSH certificate authority with group-to-principal mappings and a configurable certificate lifetime.
- The `halo` client commands `login` (device flow), `whoami`, `ssh-cert` and `logout`, and the built-in Halo CLI application.
- Organization settings: name, contact email, sign-in message, logo, session lifetime, lockout, setup link lifetime and token lifetime defaults.
- Domain verification with DNS TXT records, and a JSON export of users, groups, memberships, roles, applications, policies, access packages and access reviews.
- Administration console: overview, users with profile editing, assigned and rule-based groups with rule preview, service accounts, devices, applications with editable redirect URIs, scopes and token lifetimes, API resources, provisioning, roles, policies, infrastructure access, governance, methods, identity providers, sessions, sign-in log, audit log, risk events, API keys, webhooks and settings.
- Applications: client secrets shown once, rotation with a 24-hour grace period, revocation, enabling and disabling, group assignment, owners, and setup guides for Grafana, Forgejo, kubectl, generic OpenID Connect, generic SAML, AWS IAM Identity Center and Slack.
- Account portal: profile, passkeys, security keys, authenticator apps, recovery codes, linked accounts, sessions, applications, access requests, approvals, access reviews and activity.
- Management API under `/api/v1` with administrative roles: global, security, user, helpdesk and application administrator, and auditor. Every administrative change writes an audit event in the same transaction. The OpenAPI 3.1 document is served at `/api/v1/openapi.yaml`.
- `HALO_TRUSTED_PROXIES` for client IP addresses behind reverse proxies.
- Signing key rotation: new OpenID Connect keys sign immediately while previous keys stay in the JWKS for 7 days, and new SAML certificates are listed beside the previous one until it is removed.
- `email_verified` in ID tokens and userinfo, set when a person redeems an emailed link or signs in through a provider that verified the address, and cleared when the address changes.
- Sessions bound to the device that started them: blocking a device signs it out. Halo CLI sessions appear in the account portal and the console and can be signed out like browser sessions.
- A per-address limit on device authorization requests and `slow_down` for clients that poll too fast.
- The `halo` command: `serve`, `migrate`, `bootstrap`, `seed-demo`, `dev-session` and `rotate-secret-key`, which re-seals every stored secret under a new `HALO_SECRET_KEY`.
- The Fernway Systems demo organization, loaded with `halo seed-demo`, with data for every area of the console.
- An example Go web application that signs in with Halo, in `examples/go-web-client`.
- End-to-end tests with a virtual authenticator for passkey sign-in to the OpenID Connect and SAML examples, magic links, access requests, access policies, and every console page at desktop and phone widths.
- Docker images for the server and the web interface, and a Docker Compose deployment with PostgreSQL and Caddy, in `deploy/`.
- Container images for `linux/amd64` and `linux/arm64`, published to `ghcr.io/scalastudios/halo-server` and `halo-web` for every commit on `main` and every release tag. The Compose deployment uses them, and `deploy/compose.build.yml` builds from the checkout instead.
- An install script, `deploy/install.sh`, behind `curl -fsSL https://halo.scala.gg/install.sh | sh`, that sets up the Docker Compose deployment on a fresh host.
- User and operator documentation in `documentation/`, with guides for email, policies, governance, federation, provisioning, webhooks, API resources and infrastructure access, and integration guides for Grafana, Forgejo, Nextcloud, Kubernetes, Proxmox VE, Outline, Headscale, AWS IAM Identity Center and Slack.
- Continuous integration for `go vet`, `go test`, type checking and the web build.
- Contributing guide, security policy, code of conduct, and issue and pull request templates.

### Changed

- Recovery codes are 16 characters (80 bits) and stored as HMAC-SHA256 with a key derived from `HALO_SECRET_KEY`. Migration `0010_hardening` deletes codes generated by earlier development builds; generate new ones from the account portal.

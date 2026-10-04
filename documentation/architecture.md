# Architecture

Halo has three parts: a Go server, a Next.js web interface, and PostgreSQL. This page describes how they fit together, what each Go package does, and how a sign-in to an application flows through them.

## Topology

```
browser ──► web (Next.js) ──► halo serve (Go) ──► PostgreSQL
            /admin /account     /api/v1/*
            /sign-in /enroll    /oauth2/*  /.well-known/*
```

The web interface is the only public entry point. It serves the console (`/admin`), the account portal (`/account`), and the sign-in and enrollment pages, and it forwards `/api`, `/oauth2`, `/.well-known`, `/saml` and `/scim` to the Go server. Because everything is served from one address, the session cookie, the OpenID Connect issuer and the passkey origin are the same, which is what `HALO_PUBLIC_URL` describes.

The web interface talks to the Go server in two ways:

- **Server-rendered pages** call the Go server directly at `HALO_API_URL` and forward the person's `halo_session` cookie (`web/src/lib/api/server.ts`).
- **Browser code** calls `/api/v1/…` on Halo's own address, and the web interface forwards the request (`web/src/lib/api/client.ts`).

`web/src/proxy.ts` sends visitors to `/admin` and `/account` without a `halo_session` cookie to `/sign-in`. It only checks that the cookie exists; the Go server validates the session on every API call, and pages redirect to `/sign-in` when it answers 401.

In production, a TLS-terminating proxy sits in front of the web interface. The [Docker Compose deployment](../deploy/README.md) uses Caddy.

## Go packages

| Package | Responsibility |
| --- | --- |
| `cmd/halo` | The `halo` command. Server commands: `serve`, `migrate`, `bootstrap`, `seed-demo`, `dev-session` and `rotate-secret-key`. Client commands, run by people on their own computers: `login`, `whoami`, `ssh-cert` and `logout`. |
| `api` | Embeds `openapi.yaml`, the API description served at `/api/v1/openapi.yaml`. |
| `internal/config` | Reads and validates the environment variables. |
| `internal/db` | Opens the PostgreSQL pool and applies the SQL files in `migrations/`, which are embedded in the binary. Each migration runs in its own transaction, in filename order, and is recorded in `schema_migrations`. |
| `internal/store` | All SQL, one file per area, with `Store.Tx` for transactions. |
| `internal/id` | Ids made of a prefix and a 26-character ULID, such as `usr_01J…`. |
| `internal/secret` | Random tokens, SHA-256 hashing, HKDF key derivation, HMAC-SHA256, constant-time comparison, and the AES-256-GCM sealer. |
| `internal/httpx` | JSON responses, the error format, request logging, session resolution, the same-origin check and the role guard. |
| `internal/auth` | Passkeys and security keys, authenticator apps, recovery codes, magic links, enrollment links, federated sign-in and the identity provider API, sessions, sign-out, the account portal's API (`/api/v1/me`), and completing sign-in requests from applications. |
| `internal/oidc` | The OpenID Connect provider, built on [zitadel/oidc](https://github.com/zitadel/oidc) with storage in PostgreSQL, RSA signing keys and the `groups` claim; the device flow for `halo login` and its `/api/v1/device` confirmation API; and API resources. |
| `internal/api` | The management API for the console: users, groups, applications, sessions, sign-in and audit logs, the overview, and signing key rotation. |
| `internal/access` | The types an access policy engine works with: the sign-in being decided and the decision. |
| `internal/policy` | The access policy engine: access policies, network zones, devices and sign-in risk, with the console's policy API. |
| `internal/mail` | Email: an outbox in the database, delivery over SMTP, and the invite, reset and magic link messages. |
| `internal/governance` | Access packages, access requests, access reviews and lifecycle rules. |
| `internal/saml` | The SAML 2.0 identity provider under `/saml`. |
| `internal/scim` | The SCIM 2.0 server under `/scim/v2`, for directories that provision people into Halo. |
| `internal/provisioning` | Pushes the people assigned to an application to that application over SCIM. |
| `internal/apikeys` | Service accounts and the API keys they authenticate with. |
| `internal/settings` | Organization settings, branding, domain verification, the data export, `/api/v1/organization` and `/api/v1/openapi.yaml`. |
| `internal/webhooks` | Webhook endpoints and signed delivery of audit and sign-in events. |
| `internal/sshca` | The SSH certificate authority, principal mappings and issued certificates. |
| `internal/cli` | The client commands of `halo`: `login`, `whoami`, `ssh-cert` and `logout`. |
| `internal/jobs` | The background job scheduler that `halo serve` runs. |
| `internal/server` | Builds the handlers and routes. |
| `internal/testdb` | Creates a throwaway database for each test. |
| `internal/mail/mailtest` | A local SMTP server for tests. |

### Routes in the Go server

| Path | Handled by |
| --- | --- |
| `/api/…` | Session resolution, then the same-origin check, then the auth and management handlers. Unknown routes answer `404` with `ERR_NO_ROUTE`. |
| `/oauth2/…` | Session resolution, then the OpenID Connect provider. |
| `/.well-known/…` | The OpenID Connect provider (discovery). |
| `/saml/…` | Session resolution, then the SAML identity provider. |
| `/scim/…` | Session and API key resolution, then the SCIM server. API keys need the `scim` scope here. |
| `GET /healthz` | Answers `200 ok` when the database responds, `503` when it does not. |

Every request is logged with its method, path, status and duration, and a panic in a handler becomes a `500` with `ERR_INTERNAL` instead of a dropped connection.

Session resolution reads the `halo_session` cookie, hashes it, and looks up a session that is neither revoked nor expired. It ignores sessions that belong to suspended or deprovisioned accounts. Without a session, it accepts an API key sent as `Authorization: Bearer hlk_…`, but only on routes the key's scope covers: `api` for `/api`, `scim` for `/scim`. It also sets a `halo_device` cookie that identifies the browser to the access policy engine.

### Background jobs

`halo serve` runs background jobs on a schedule:

| Job | Every |
| --- | --- |
| Delete expired records: setup steps, sign-in requests a day after they expire, tokens 7 days after they expire, sessions 30 days after they end, and enrollment links 30 days after they expire | Hour |
| Retry undelivered email | Minute |
| Expire access grants, mark overdue access reviews, run lifecycle rules | Minute |
| Push assigned people to applications over SCIM | 2 minutes |
| Deliver webhooks | 10 seconds |
| Retire OpenID Connect signing keys 7 days after a newer key replaced them | Hour |
| Delete expired SAML requests | Hour |
| Delete old access policy evaluations | Day |

Sign-in and audit events are kept.

## Signing in to an application

This is the path of an OpenID Connect sign-in with the authorization code flow. The application is Grafana at `https://grafana.example.com` and Halo runs at `https://auth.example.com`.

```
Grafana            Browser                       Halo web            Halo server
   │ redirect to /oauth2/authorize ─────────────────►│────forward────────►│ 1–2 store sign-in request
   │                  │◄──────── redirect to /sign-in?authRequest=are_… ──│
   │                  │── GET /sign-in ─────────────►│── GET /api/v1/auth/request/{id} ─► 3
   │                  │── sign in (passkey, code) or continue session ────►│ 4–5 finish
   │                  │◄──────── {"redirect": "/oauth2/authorize/callback?id=are_…"}
   │                  │── GET /oauth2/authorize/callback ───────────────────►│ 6 session guard
   │◄── redirect to the redirect URI with code and state ─────────────────────│ 7
   │── POST /oauth2/token ───────────────────────────────────────────────────►│ 8 tokens
   │── GET /oauth2/userinfo ─────────────────────────────────────────────────►│ 9 claims
```

1. Grafana redirects the browser to `https://auth.example.com/oauth2/authorize` with its `client_id`, `redirect_uri`, `response_type=code`, the requested `scope`, a `state`, and with PKCE a `code_challenge` and `code_challenge_method=S256`.
2. The web interface forwards the request to the Go server. The provider checks that the application exists, is active and uses OpenID Connect; that `redirect_uri` exactly matches a registered address; and that single-page and native applications sent a PKCE challenge. Halo stores the request as a sign-in request with an `are_` id that expires after 15 minutes, and redirects the browser to `/sign-in?authRequest=are_…`. With `prompt=none`, Halo skips the sign-in page instead: it checks the browser's Halo session, the assignment and the access policy right away, and either issues a code or redirects back with `login_required` or `interaction_required`.
3. The sign-in page loads the application's name with `GET /api/v1/auth/request/{id}` and shows "Sign in to continue to Grafana".
4. If the browser already has a Halo session, the page calls `POST /api/v1/auth/continue`. Halo answers `ERR_REAUTH_REQUIRED`, and asks the person to sign in again, when the request carried `prompt=login` or a `max_age` shorter than the session's age. Without a session, the person signs in with a passkey or security key (`POST /api/v1/auth/passkey/begin`, the browser's passkey prompt, then `POST /api/v1/auth/passkey/finish`), or with their email address and an authenticator code or recovery code (`POST /api/v1/auth/identify`, then `POST /api/v1/auth/totp` or `POST /api/v1/auth/recovery`). Every call carries the sign-in request id.
5. Every way of signing in ends in the same server function, which:
   - refuses suspended and deprovisioned accounts,
   - refuses people who are not in a group assigned to Grafana, or when Grafana is disabled,
   - asks the access policy engine whether to allow the sign-in, block it, or require a passkey or security key,
   - creates a session and sets the `halo_session` cookie if the browser had none,
   - marks the sign-in request as completed by this person and this session, with the `amr` values and the time of authentication,
   - records a successful sign-in event.

   The last three steps happen in one database transaction. Every refusal is recorded as a failed sign-in event with its reason. The server answers with `{"redirect": "/oauth2/authorize/callback?id=are_…"}`.
6. The browser follows the redirect. Before the provider issues a code, Halo checks that the sign-in request was completed by the session in this browser. If the request is completed but the browser has no Halo session, or a different one, Halo answers `403` with "This sign-in was completed in a different browser session. Start again from the application." This stops a sign-in completed in one browser from being redeemed in another.
7. The provider redirects the browser to Grafana's redirect URI with `code` and `state`. The code is the sign-in request id, encrypted with a key derived from `HALO_SECRET_KEY`.
8. Grafana exchanges the code at `POST /oauth2/token`, authenticating with its client secret, or with the PKCE verifier for public clients. Halo checks again that the person is active and assigned, then returns an ID token signed with RS256, an access token, and a refresh token if Grafana asked for `offline_access`. The sign-in request is deleted, so the code works once.
9. Grafana may call `GET /oauth2/userinfo` with the access token. Halo checks again that the person is active and still assigned on every userinfo, refresh and introspection request, so suspending someone or removing them from the group takes effect at the next one.

### Services and client credentials

A service application calls `POST /oauth2/token` with `grant_type=client_credentials` and its client secret. Halo issues a JWT access token whose subject is the client ID, carrying only the scopes registered on the application. No person or session is involved.

### Signing out of an application

When an application sends someone to `/oauth2/logout` with their ID token as `id_token_hint`, Halo revokes the tokens that application holds for the person and ends the Halo session named in the token's `sid` claim, so other applications ask them to sign in again too. Halo then shows a signed-out page and returns the person to the application's post-logout redirect URI if it is registered. People can also end their Halo session with **Sign out** in the account portal or the console.

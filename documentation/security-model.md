# Security model

This page describes how Halo authenticates people, protects sessions and secrets, issues tokens to applications, and limits what administrators can do. It describes the code as it is today, including its known limitations. To report a vulnerability, follow [SECURITY.md](../SECURITY.md).

## Authentication

Halo has no passwords. People sign in with a passkey or security key, with their email address and a code from an authenticator app or a recovery code, with an account at a configured identity provider, or, when email is configured, with a magic link sent to their address. Security administrators can turn each method off for the organization, and a sign-in with a method that is off is refused.

### Passkeys and security keys

- Credentials are bound to the hostname of `HALO_PUBLIC_URL`, which is the WebAuthn relying party ID, and Halo accepts responses only from the origin of `HALO_PUBLIC_URL`.
- Halo registers discoverable credentials, so people sign in without typing their email address. It asks the authenticator for user verification (fingerprint, face, screen lock or PIN) where it supports one.
- Halo stores the credential's public key, its id, its signature counter, its transports and its backup flags. It never receives a private key or biometric data.
- If a credential's signature counter goes backwards, which suggests it was cloned, Halo rejects the sign-in.
- Each registration or sign-in starts a challenge that works once and expires after 5 minutes.

### Authenticator apps

- Codes are 6-digit time-based one-time passwords (SHA-1, 30-second steps). Halo accepts the current code and the codes either side of it, to allow for clock drift.
- Each code works once: Halo records the last step used and refuses a code from that step or earlier.
- The secret shared with the app is encrypted with `HALO_SECRET_KEY`.

### Recovery codes

- Halo generates 10 codes at a time and shows them once. Each code is 16 random base32 characters (80 bits), shown as `xxxx-xxxx-xxxx-xxxx`.
- Halo stores only an HMAC-SHA256 of each code, keyed with a key derived from `HALO_SECRET_KEY` (HKDF-SHA256 with a fixed label), so guesses cannot be checked against the stored values without the key.
- Each code works once. Generating new codes replaces every old one.
- Case, dashes and spaces are ignored when people type a code.

### Magic links

- Halo sends a magic link only to an active account and only while magic links are turned on, and answers the request the same way whether or not the account exists or magic links are on.
- A link works once and expires after 10 minutes. Halo stores only its SHA-256 hash.
- Halo sends at most 5 links per person in 15 minutes.

### Identity providers

- Halo starts every federated sign-in with PKCE (S256) and a random state bound to the browser by an HttpOnly `halo_federation` cookie, and for OpenID Connect providers a nonce. The state works once and expires after 10 minutes; Halo stores only its SHA-256 hash.
- Halo links a provider account to an existing Halo account by email address only when the provider confirms the address (`email_verified`, Entra ID's `xms_edov`, or a verified primary address on GitHub) and its domain is allowed for that provider. After linking, Halo identifies the provider account by its subject.
- Creating accounts on first sign-in requires at least one allowed email domain. Service accounts are never linked.
- Client secrets for providers are encrypted with `HALO_SECRET_KEY`. Provider issuers must use https.
- A federated sign-in carries `amr: ["fed"]` and does not satisfy a policy that requires a passkey or security key.

[Federation](federation.md) describes the flow.

### Lockout

After 5 failed authenticator or recovery codes within 15 minutes, Halo refuses further codes for that account, answering `429` with `ERR_TOO_MANY_ATTEMPTS`. A security administrator can set the threshold between 3 and 20 and the window between 5 minutes and 24 hours under **Security defaults**. Refused attempts count as failures too, so the lockout lasts until fewer failures than the threshold fall within the window. The count also applies to addresses that have no account. Passkey sign-in is not affected. Anyone who knows a person's email address can trigger the lockout, which blocks that person's code sign-in, but not their passkeys, for as long as the attempts continue.

### Enrollment links

Invite and reset links contain 32 random bytes. Halo stores only their SHA-256 hash. A link works once, expires after the setup link lifetime (7 days by default), and stops working when a newer link is created for the same person. Halo marks the link used in the same transaction that saves the new passkey.

### Account status

Suspended and deprovisioned accounts cannot sign in. Halo stops honouring their existing sessions immediately, and suspending an account also revokes its sessions and the tokens issued during them.

### Access policy

Every sign-in, including continuing an existing session into an application, silent sign-in with `prompt=none`, and approving a `halo login` device code, asks the access policy engine for a decision: allow, block, or require a passkey or security key. The engine blocks devices an administrator blocked and methods that are turned off, then applies the strictest matching enforced policy. Refused sign-ins are recorded with the policy that refused them. [Policies](policies.md) describes the engine, network zones, devices and risk signals.

## Sessions

| Cookie | Contents | Attributes |
| --- | --- | --- |
| `halo_session` | 32 random bytes; Halo stores only the SHA-256 hash | HttpOnly, SameSite=Lax, Secure, path `/`, expires after the session lifetime (12 hours by default) |
| `halo_device` | 32 random bytes that identify the browser to the access policy engine. It does not authenticate anyone. | HttpOnly, SameSite=Lax, Secure, path `/`, expires after 2 years |
| `halo_federation` | The state of a federated sign-in in progress | HttpOnly, SameSite=Lax, Secure, path `/api/v1/auth/federated`, expires after 10 minutes |

`HALO_DEV=1` removes the Secure flag from these cookies, for local development over http.

Sessions are checked on the server on every request. Signing out, revoking a session in the account portal or the console, suspending the account, resetting its authentication, and blocking the device a session started on all end sessions immediately. Ending a session revokes the access and refresh tokens that applications received during it. Each `halo login` appears as a Halo CLI session, and ending it revokes the CLI's tokens.

## Same-origin protection

Every request to `/api/` with a method other than GET, HEAD or OPTIONS goes through a same-origin check before any handler runs:

1. If the request has a `Sec-Fetch-Site` header, it passes only when the value is `same-origin` or `none`.
2. Otherwise, it passes only when it has no `Origin` header or the `Origin` equals the origin of `HALO_PUBLIC_URL`.

Requests that fail get `403` with `ERR_CROSS_SITE`. Browsers send `Sec-Fetch-Site` and `Origin` themselves, so a page on another site cannot make a signed-in person's browser change anything in Halo. Scripts and command-line tools such as curl send neither header, so they pass the check and are authenticated by the session cookie, API key or token they present.

## API keys and service accounts

- API keys belong to service accounts, start with `hlk_` followed by 32 random bytes, and are shown once. Halo stores only their SHA-256 hash.
- A key carries scopes: `api` for the management API and `scim` for the SCIM server. A key without the matching scope is treated as no credentials. Keys are ignored on `/api/v1/me` and `/api/v1/auth` routes.
- A request made with a key has exactly the roles of its service account, checked like a person's. An administrator can give a service account only roles they hold, and must hold all of its roles to change it or create keys for it.
- A key stops working when it is revoked, expires, or its service account is disabled.
- The SCIM server requires the user administrator role, and the global administrator role to change people who hold an administrator role.

The API also refuses request bodies larger than 1 MiB, bodies that are not valid JSON, and JSON with fields it does not expect.

## Secrets at rest

| Value | How Halo stores it |
| --- | --- |
| Session tokens | SHA-256 hash |
| Client secrets | SHA-256 hash. Shown once; the console shows only the last 4 characters afterwards. |
| Refresh tokens | SHA-256 hash |
| Invite and reset link tokens | SHA-256 hash |
| Recovery codes | HMAC-SHA256 with a key derived from `HALO_SECRET_KEY` |
| OpenID Connect and SAML signing keys | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Authenticator-app secrets | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Tokens for provisioning applications over SCIM | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Email in the outbox | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Magic link tokens, API keys and federated sign-in state | SHA-256 hash |
| SSH certificate authority key | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Identity provider client secrets and webhook signing secrets | Encrypted with AES-256-GCM under `HALO_SECRET_KEY` |
| Opaque access tokens | Not stored. The token is its id encrypted with a key derived from `HALO_SECRET_KEY`; Halo stores the id. |
| JWT access tokens | Not stored. The token is signed; Halo stores its id (`jti`) to check revocation on userinfo and introspection. |
| Authorization codes | The sign-in request id encrypted with the same derived key. Stored with the request, which is deleted when the code is exchanged; an unused code stops working when the request expires after 15 minutes. |
| Passkey public keys | As they are; they are public |

Halo compares hashes in constant time. Client IDs, session ids and other identifiers are not secrets.

`halo rotate-secret-key` replaces `HALO_SECRET_KEY`: it re-encrypts every value above in one transaction and removes recovery codes, which cannot be re-derived; see [Rotate HALO_SECRET_KEY](../deploy/README.md#rotate-halo_secret_key). Losing the key makes the signing keys and authenticator-app secrets unreadable, and a backup is only usable with the key that was in use when it was taken.

## OpenID Connect

### Requests

- The issuer is `HALO_PUBLIC_URL`. Discovery is at `/.well-known/openid-configuration` and the signing keys at `/oauth2/keys`.
- Applications must use the authorization code flow (`response_type=code`). The discovery document lists more response and grant types than Halo's applications accept.
- PKCE uses S256 only. Single-page and native applications must send a challenge, and web applications may.
- Redirect URIs must exactly match an address registered on the application. Halo registers only https addresses without a fragment, except `http://localhost` and `http://127.0.0.1`. For native applications, Halo matches loopback redirect URIs on path and query and accepts any port, so command-line tools can pick a free port.
- Sign-in requests expire after 15 minutes. An authorization code works once.
- `prompt=login`, and a `max_age` shorter than the age of the Halo session, make the person sign in again.
- With `prompt=none`, Halo never shows a page. It issues a code when the browser has a Halo session that is within `max_age`, belongs to the person named in `id_token_hint`, is assigned to the application and passes the access policy, and answers `login_required` or `interaction_required` otherwise.
- Before issuing a code, Halo checks that the sign-in was completed by the Halo session in the same browser, so a completed sign-in cannot be redeemed elsewhere.

### Clients

- Web applications and services authenticate with their client secret, sent with HTTP Basic (`client_secret_basic`) or as form fields (`client_secret_post`). Token introspection accepts HTTP Basic only.
- Single-page and native applications are public clients: they have no secret and rely on PKCE.

### Tokens

- ID tokens are signed with RS256 using a 2048-bit RSA key that Halo generates the first time it needs one. A global administrator rotates it under **Security defaults** → **Signing keys**: the new key signs from then on, and previous keys stay in `/oauth2/keys` for 7 days so the tokens they signed still verify. The SAML signing certificate rotates the same way and stays listed until you remove it; SAML applications need the new certificate.
- Access tokens are opaque by default. Service applications, the Halo CLI, and applications granted scopes on an [API resource](api-resources.md) receive JWT access tokens signed with the same key as ID tokens. Each application has its own access token lifetime, 15 minutes by default; tokens for an API resource use that API's lifetime.
- Refresh tokens are issued only when the application requests `offline_access`. They last 8 hours by default and rotate on every use unless rotation is turned off for the application; a refresh token that was already used is rejected.
- The device flow is available only to the built-in Halo CLI application. A device code expires after 10 minutes and is approved only on Halo's `/device` page by a signed-in person, after the access policy check. Halo CLI access tokens are also accepted on the `/api/v1/me/` routes, and stop working when the person is suspended.
- Halo checks that the person is still active and still assigned to the application whenever it issues tokens, refreshes them, or answers userinfo and introspection. Disabling an application revokes every token issued to it.

### Claims

- `sub` is the person's Halo id, which never changes.
- `email_verified` is `true` only after the person redeemed a magic link, finished setup from a link Halo emailed (not one an administrator copied), or signed in through an identity provider that verified the same address. Changing the address sets it back to `false`.
- `amr` is `["hwk", "mfa"]` after a passkey or security key and `["otp"]` after an authenticator or recovery code. Applications can use it to require a phishing-resistant sign-in.
- `groups` lists the names of every group the person belongs to.
- `sid` identifies the Halo session. When an application sends the person to `/oauth2/logout` with the ID token as `id_token_hint`, Halo ends that session as well as revoking the application's tokens.

## Administration

- Every administrative endpoint checks the actor's role. Global administrators pass every check. [Concepts](concepts.md#roles) lists what each role allows.
- Only a global administrator can change an account that holds an administrator role, nobody can suspend their own account, the last active global administrator cannot be suspended, and a global administrator cannot remove their own role.
- Every administrative change, and every change people make to their own sign-in methods, writes an audit event with the actor and IP address in the same transaction as the change.
- Nobody can decide their own access request, global administrators included. Reviewers see only the reviews they are named on, unless they hold an administrator role.
- `halo bootstrap` creates a global administrator only when none exists. `halo seed-demo` and `halo dev-session` refuse to run unless `HALO_DEV=1`.

## Webhooks

- Halo signs every delivery with HMAC-SHA256 over a timestamp and the raw body, keyed with a per-endpoint secret that is shown once; see [Webhooks](webhooks.md#verify-the-signature). Receivers should reject timestamps older than 5 minutes.
- Endpoints must use https, except loopback addresses. Halo does not follow redirects and gives up on a request after 10 seconds.
- Event bodies contain the audit and sign-in events as administrators see them, including email and IP addresses. Send them only to systems allowed to hold that data.

## SSH certificates

- The SSH certificate authority's Ed25519 key is encrypted with `HALO_SECRET_KEY`. Its public key is public at `/api/v1/ssh/ca.pub`.
- Halo signs a key only for a signed-in person, only for the principals their groups map to, and for at most 24 hours (8 hours by default), starting 5 minutes in the past to allow for clock differences. Each certificate gets a unique serial and the key ID `halo:<user id>:<serial>`, and writes an audit event.
- sshd cannot check whether a certificate was revoked, so a certificate stays valid until it expires even after the person is suspended. Keep the lifetime short.

## Transport and deployment

- Halo refuses to start when `HALO_PUBLIC_URL` does not use https, unless `HALO_DEV=1`.
- Only the web interface needs to be reachable from outside. Keep the Go server's port private; the [Docker Compose deployment](../deploy/README.md) publishes only Caddy's ports.
- Halo records client IP addresses on sign-in events, sessions and audit events. It reads `X-Forwarded-For` only from proxies listed in `HALO_TRUSTED_PROXIES`, from right to left, and uses the first address that is not a trusted proxy. The outermost proxy must set the header and discard any value the client sent, as Caddy does.
- Unexpected errors are logged on the server and answered with `ERR_INTERNAL`, without internal details. Halo's request log records the method, path, status and duration of each request, and leaves out query strings, headers and bodies.

## Known limitations

- Rotating `HALO_SECRET_KEY` removes every recovery code, and opaque access tokens and unredeemed authorization codes stop working.
- Halo sends no separate verification email, so an address stays unverified until it is used in one of the ways listed under [Claims](#claims).
- The code lockout is per account, so it can be triggered by anyone who knows the address.
- APIs that validate JWT access tokens themselves do not see revocations until the token expires, and SSH certificates cannot be revoked by Halo.
- Webhook signing secrets cannot be rotated; replace the endpoint instead.

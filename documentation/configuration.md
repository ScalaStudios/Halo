# Configuration

Halo reads its configuration from environment variables when a `halo` command starts. If a required variable is missing or invalid, the command lists every problem and exits without starting, for example:

```text
halo: configuration is incomplete:
  HALO_SECRET_KEY must be 32 random bytes, base64 encoded. Generate one with: openssl rand -base64 32
```

## Server

The Go server (`halo serve` and the other `halo` commands) reads these variables.

| Variable | Required | Default | Example |
| --- | --- | --- | --- |
| `HALO_PUBLIC_URL` | Yes | | `https://auth.example.com` |
| `HALO_DATABASE_URL` | Yes | | `postgres://halo:secret@postgres:5432/halo` |
| `HALO_SECRET_KEY` | Yes | | Output of `openssl rand -base64 32` |
| `HALO_LISTEN` | No | `:8080` | `127.0.0.1:8080` |
| `HALO_ORGANIZATION` | No | `Halo` | `Fernway Systems` |
| `HALO_DEV` | No | Off | `1` |
| `HALO_TRUSTED_PROXIES` | No | `127.0.0.1/32,::1/128` | `172.31.250.0/24` |
| `HALO_SMTP_HOST` | No | | `smtp.example.com` |
| `HALO_SMTP_PORT` | No | `587` | `465` |
| `HALO_SMTP_USERNAME` | No | | `halo@example.com` |
| `HALO_SMTP_PASSWORD` | No | | |
| `HALO_SMTP_FROM` | When `HALO_SMTP_HOST` is set | | `Halo <halo@example.com>` |

### HALO_PUBLIC_URL

The address people type to reach Halo: the web interface, not the Go server. Halo uses it in three ways:

- It is the OpenID Connect **issuer**. Applications must use exactly this value, and it appears as `iss` in every token.
- Its hostname is the passkey (WebAuthn) **relying party ID**. Passkeys only work on the hostname they were created for, so changing the hostname later means everyone needs a new passkey.
- Its origin is the only one the [same-origin check](security-model.md#same-origin-protection) accepts.

It must be an absolute URL, and it must use https unless `HALO_DEV=1`. Halo removes a trailing slash.

### HALO_DATABASE_URL

The PostgreSQL connection URL. Halo is developed and tested against PostgreSQL 17. Every `halo` command applies pending migrations when it connects, so `halo serve` upgrades the schema on start; `halo migrate` applies migrations and exits.

### HALO_SECRET_KEY

32 random bytes, base64 encoded with the standard alphabet. Generate one with `openssl rand -base64 32`.

Halo encrypts these with this key using AES-256-GCM: the OpenID Connect and SAML signing keys, the SSH certificate authority's key, authenticator-app secrets, identity provider client secrets, webhook signing secrets, the tokens Halo uses to provision applications over SCIM, and email waiting in the outbox. It also derives from it the key that encrypts opaque access tokens and authorization codes, and the key that recovery codes are hashed with (HMAC-SHA256). Starting Halo with a different key without rotating makes those values unreadable, so nobody can sign in to applications and authenticator-app codes stop working until the original key is back. Back it up together with the database.

To replace the key, stop Halo and run `halo rotate-secret-key` with the current key in `HALO_SECRET_KEY` and the new one in `HALO_NEW_SECRET_KEY`. It re-encrypts every stored secret in one transaction and removes recovery codes, which cannot be re-derived, then prints what to change in your deployment. [Rotate HALO_SECRET_KEY](../deploy/README.md#rotate-halo_secret_key) walks through it.

### HALO_LISTEN

The address the Go server listens on, as `host:port` or `:port`.

### HALO_ORGANIZATION

Your organization's name, shown in the console, on the sign-in page and in email. A global administrator can override it in the console under **Organization**; this variable is the name Halo uses while that setting is empty. The API returns the name in use from `GET /api/v1/organization`.

### HALO_DEV

Development mode. It is on only when the value is exactly `1`. Development mode:

- allows `HALO_PUBLIC_URL` to use plain http,
- removes the Secure flag from Halo's cookies,
- lets the OpenID Connect provider run on plain http,
- enables `halo seed-demo` and `halo dev-session`,
- adds `GET /api/v1/dev/outbox`, which shows global administrators the 50 most recent emails,
- accepts `http://localhost` and `http://127.0.0.1` as identity provider issuers.

Never set it in production.

### HALO_TRUSTED_PROXIES

A comma-separated list of CIDR ranges, such as `10.0.0.0/8,172.31.250.0/24`, for the reverse proxies in front of the Go server. Halo refuses to start if an entry is not a valid CIDR range. The default trusts only the local machine.

Halo records a client IP address on every sign-in, session and audit event. When a request arrives from an address in these ranges, Halo reads `X-Forwarded-For` from right to left and uses the first address that is not itself in these ranges. Requests from any other address use the connecting address and ignore the header, so a client cannot forge its own address.

In the [Docker Compose deployment](../deploy/README.md), the Go server sits behind the web container, so set this to the subnet of the Compose network: `172.31.250.0/24` in the provided files. Without it, Halo records the web container's address for everyone.

### SMTP

`HALO_SMTP_HOST`, `HALO_SMTP_PORT`, `HALO_SMTP_USERNAME`, `HALO_SMTP_PASSWORD` and `HALO_SMTP_FROM` describe the mail server Halo sends email through. Email is configured when `HALO_SMTP_HOST` is set, and then `HALO_SMTP_FROM` is required.

Halo emails invite links, reset links, magic sign-in links and access request notices. Every message goes into an outbox in the database first; Halo retries failed deliveries with growing gaps, up to 5 attempts within a day. Without SMTP, messages are queued but not sent, and administrators pass invite and reset links on themselves. [Email](email.md) explains the outbox and magic links.

- Port `465` uses TLS from the start of the connection. On any other port, Halo requires STARTTLS, except when the host is `localhost` or a loopback address.
- With `HALO_SMTP_USERNAME` set, Halo authenticates with PLAIN using `HALO_SMTP_USERNAME` and `HALO_SMTP_PASSWORD`.

## Web interface

The Next.js web interface reads these variables.

| Variable | Default | Purpose |
| --- | --- | --- |
| `HALO_API_URL` | `http://localhost:8080` | Address of the Go server. Server-rendered pages call it at runtime, and the forwarding rules for `/api`, `/oauth2`, `/.well-known`, `/saml` and `/scim` point at it. `next build` writes those rules into the build, so set the variable both when you build and when you run. |
| `HALO_WEB_DIST` | `.next` | Directory for the Next.js build output. Lets you run two copies of the web interface from one checkout. |
| `PORT` | `3200` in the Docker image | Port of the production server in the Docker image. `bun run dev` and `bun run start` use port 3200. |
| `HOSTNAME` | `0.0.0.0` in the Docker image | Interface the production server binds to in the Docker image. |

## Docker Compose deployment

`deploy/.env` holds the server variables above plus two that only the Compose files use:

| Variable | Purpose |
| --- | --- |
| `HALO_DOMAIN` | The domain Caddy obtains a certificate for and serves Halo on. `HALO_PUBLIC_URL` is built from it. |
| `POSTGRES_PASSWORD` | The password of the `halo` database user. `HALO_DATABASE_URL` is built from it. |

## Development and tests

| Variable | Default | Purpose |
| --- | --- | --- |
| `HALO_TEST_DATABASE_URL` | `postgres://halo:halo@localhost:5436/halo` | The server where `go test` creates a throwaway database for each test. The user needs permission to create databases. |
| `HALO_URL` | `http://localhost:3200` | The Halo address the end-to-end test runs against. |

The example application in `examples/go-web-client` reads `CLIENT_ID`, `CLIENT_SECRET`, `HALO_ISSUER`, `REDIRECT_URI` and `LISTEN`; its [README](../examples/go-web-client/README.md) describes them.

## Organization settings

Some settings live in the database rather than in environment variables, so you change them in the console without restarting Halo. Halo reads them again within 30 seconds of a change.

| Setting | Console page | Default | Range | Who can change it |
| --- | --- | --- | --- | --- |
| Organization name | **Organization** | `HALO_ORGANIZATION` | Up to 100 characters | Global administrator |
| Contact email | **Organization** | Empty | | Global administrator |
| Sign-in message | **Branding** | Empty | Up to 500 characters | Global administrator |
| Logo | **Branding** | None | PNG, JPEG or WebP, up to 256 KB | Global administrator |
| Session lifetime | **Security defaults** | 12 hours | 1 to 72 hours | Security administrator |
| Lockout threshold | **Security defaults** | 5 failed codes | 3 to 20 | Security administrator |
| Lockout window | **Security defaults** | 15 minutes | 5 minutes to 24 hours | Security administrator |
| Setup link lifetime | **Security defaults** | 7 days | 1 to 30 days | Security administrator |
| Access token lifetime for new applications | **Security defaults** | 15 minutes | 5 minutes to 24 hours | Security administrator |
| ID token lifetime for new applications | **Security defaults** | 15 minutes | 5 minutes to 24 hours | Security administrator |
| Refresh token lifetime for new applications | **Security defaults** | 8 hours | 1 hour to 90 days | Security administrator |
| SSH certificate lifetime | **Infrastructure access** | 8 hours | 5 minutes to 24 hours | Security administrator |

A new session lifetime applies to sessions that start afterwards. The token lifetimes are defaults for applications created afterwards; change an existing application's lifetimes on its own page.

The API exposes these settings at `GET` and `PATCH /api/v1/settings`.

## Fixed values

These values cannot be changed in this version:

| What | Value |
| --- | --- |
| Magic sign-in links | Expire after 10 minutes; at most 5 per person in 15 minutes |
| Passkey and authenticator setup steps | Expire after 5 minutes |
| Sign-in requests from applications | Expire after 15 minutes |
| Federated sign-in attempts | Expire after 10 minutes |
| Device codes for `halo login` | Expire after 10 minutes |
| Client secrets | Valid for 365 days |
| Old secrets after rotation | Stop working after 24 hours |
| Previous OpenID Connect signing keys after rotation | Stay published in `/oauth2/keys` for 7 days |
| Email delivery | Up to 5 attempts within 1 day |
| Webhook delivery | Up to 8 attempts over about an hour |

# Self-host Halo with Docker Compose

This directory runs Halo in production on a single host: PostgreSQL, the Halo server, the Halo web interface, and Caddy for TLS. Caddy obtains and renews the certificate for your domain automatically.

```
internet ──► caddy :80 :443 ──► halo-web :3200 ──► halo-server :8080 ──► postgres :5432
```

| Service | Image | What it does |
| --- | --- | --- |
| `postgres` | `postgres:17-alpine` | Stores everything in the `postgres` volume. |
| `halo-server` | Built from `Dockerfile.server` | Runs `halo serve`: applies database migrations on every start, then serves the API and the OpenID Connect endpoints. |
| `halo-web` | Built from `Dockerfile.web` | Serves the console, sign-in pages and account portal, and forwards `/api`, `/oauth2`, `/.well-known`, `/saml` and `/scim` to `halo-server`. |
| `caddy` | `caddy:2` | Terminates TLS for `HALO_DOMAIN` and sends every request to `halo-web`. |

Only Caddy publishes ports. The other services are reachable only on the internal `halo` network.

## Before you start

- A Linux host with Docker Engine and the Docker Compose plugin. These files were tested with Docker 29.8 and Docker Compose 5.5.
- A domain name for Halo, such as `auth.example.com`. Halo's address becomes the OpenID Connect issuer and the passkey domain, so choose one you intend to keep: passkeys only work on the hostname they were created for.
- Ports 80 and 443 on the host reachable from the internet, so Caddy can obtain and renew the certificate.
- A copy of the Halo repository on the host. The Compose file builds the Halo images from it.

## Install

1. Point an `A` record (and an `AAAA` record if the host has IPv6) for your domain at the host, and wait until it resolves.

2. Create the environment file:

   ```bash
   cd deploy
   cp .env.example .env
   ```

3. Edit `.env` and set these values:

   | Variable | Set it to |
   | --- | --- |
   | `HALO_DOMAIN` | Your domain, for example `auth.example.com`. |
   | `POSTGRES_PASSWORD` | The output of `openssl rand -hex 32`. Use hex: the password is also part of `HALO_DATABASE_URL`, where some symbols would break the URL. |
   | `HALO_SECRET_KEY` | The output of `openssl rand -base64 32`. Store a copy outside the host, for example in your password manager. |
   | `HALO_ORGANIZATION` | Your organization's name, shown in the console. |

   Leave `HALO_PUBLIC_URL` and `HALO_DATABASE_URL` as they are: Compose fills them in from `HALO_DOMAIN` and `POSTGRES_PASSWORD`. Leave `HALO_DEV` empty. To have Halo email invite, reset and magic sign-in links, fill in the `HALO_SMTP_*` variables. [Configuration](../documentation/configuration.md) describes every variable.

4. Build the images and start Halo:

   ```bash
   docker compose up -d --build
   ```

5. Check that every service is running:

   ```bash
   docker compose ps
   ```

   `postgres` and `halo-web` show `healthy` once they are ready, and then Caddy starts. If Caddy cannot obtain a certificate, `docker compose logs caddy` says why; the usual causes are DNS that does not point at the host yet, or ports 80 and 443 blocked by a firewall.

6. Confirm Halo answers on your domain. The `issuer` in the response must be `https://` followed by your domain:

   ```bash
   curl https://auth.example.com/.well-known/openid-configuration
   ```

7. Create the first global administrator:

   ```bash
   docker compose exec halo-server halo bootstrap --email you@example.com --name "Your Name"
   ```

   Open the link it prints and create a passkey. The link works once and expires after 7 days. `bootstrap` refuses to run once a global administrator exists; invite everyone else from the console at `https://auth.example.com/admin`.

## Back up

Back up two things: the database, and `HALO_SECRET_KEY`. Signing keys and authenticator-app secrets in the database are encrypted with that key, so a backup restored without the same key cannot sign anyone in. `deploy/.env` holds both the key and the database password.

Create a dump:

```bash
docker compose exec -T postgres pg_dump -U halo -d halo --format=custom > halo-$(date +%F).dump
```

Copy the dump off the host. To restore it, stop Halo, restore, and start Halo again with the same `HALO_SECRET_KEY` that was in use when the dump was taken:

```bash
docker compose stop halo-server halo-web
docker compose exec -T postgres pg_restore -U halo -d halo --clean --if-exists < halo-2026-10-04.dump
docker compose start halo-server halo-web
```

Caddy keeps its certificates in the `caddy-data` volume. If you lose it, Caddy requests new certificates.

## Upgrade

Take a backup first. Migrations only run forward: to go back to an older version, restore the backup you took before upgrading, then check out and rebuild that version.

```bash
git pull
docker compose pull --ignore-buildable
docker compose build --pull
docker compose up -d
```

`pull` updates PostgreSQL and Caddy within their pinned versions, `build --pull` rebuilds Halo on updated base images, and `halo-server` applies any new migrations when it starts.

Always build after you update the repository. Compose reuses an existing `halo-server:local` or `halo-web:local` image instead of rebuilding it, so `docker compose up -d` without a build keeps running the old version.

## Rotate HALO_SECRET_KEY

`HALO_SECRET_KEY` encrypts the OpenID Connect and SAML signing keys, authenticator-app secrets, the tokens Halo uses to provision applications over SCIM, identity provider client secrets, webhook signing secrets, the SSH certificate authority and email waiting in the outbox. Recovery codes are stored as HMACs keyed from it. Starting Halo with a different key without rotating makes all of that unreadable: `/oauth2/keys` fails with status 500 and authenticator-app codes stop working. Sessions and passkeys do not depend on the key.

`halo rotate-secret-key` re-encrypts every stored secret from the current key to a new one in a single transaction, so either everything moves to the new key or nothing changes. Halo must not be running while it does this, or it could write values with the old key afterwards.

1. Take a backup.

2. Generate the new key and keep it in your password manager:

   ```bash
   openssl rand -base64 32
   ```

3. Stop Halo and run the rotation with the new key. `HALO_SECRET_KEY` still holds the current key from `deploy/.env`:

   ```bash
   docker compose stop halo-web halo-server
   docker compose run --rm -e HALO_NEW_SECRET_KEY='<new key>' halo-server rotate-secret-key
   ```

   The command prints how many values it re-encrypted in each table. If any value cannot be opened with the current key, it stops and changes nothing.

4. In `deploy/.env`, replace the value of `HALO_SECRET_KEY` with the new key, then start Halo:

   ```bash
   docker compose up -d
   ```

What the rotation cannot carry over:

- **Recovery codes.** Halo only stores an HMAC of each code, so they cannot be re-derived under the new key. The command removes them and lists the people who had them; ask those people to generate new codes on their account's Security page.
- **Opaque access tokens and unredeemed authorization codes.** These are encrypted with a key derived from `HALO_SECRET_KEY` and stop working once Halo runs with the new key. Applications get new access tokens with their refresh tokens, and anyone who was halfway through signing in starts again.

Backups taken before the rotation still need the old key. Keep the old key until those backups have expired.

Treat the key like the database itself: keep it out of version control, back it up with every database backup, and restrict who can read `deploy/.env`.

## Health checks

The `halo-server` image is distroless: it has no shell and no `wget` or `curl`, so a health check cannot run inside it. Instead, `halo-web` runs one against it over the internal network:

```bash
wget -q -O /dev/null http://halo-server:8080/healthz
```

`/healthz` returns `200 ok` when the database answers and `503` when it does not. Caddy waits for this check before it starts. On Kubernetes or another orchestrator, point an HTTP probe at port 8080, path `/healthz`.

## Client IP addresses

Halo records the client's IP address on every sign-in, session and audit event. Caddy sets `X-Forwarded-For` to the client's address and ignores any value the client sent, and `halo-web` passes the header through unchanged. `halo-server` reads the header only from addresses in `HALO_TRUSTED_PROXIES`, and uses the right-most address in it that is not itself a trusted proxy.

`HALO_TRUSTED_PROXIES` in `.env` is set to `172.31.250.0/24`, the subnet that `compose.yml` assigns to the `halo` network. Without it, Halo records the address of the `halo-web` container for everyone. If that subnet overlaps a network that already exists on your host, `docker compose up` fails with a "Pool overlaps" error: pick another private `/24` and change it in both `compose.yml` and `.env`.

## Build the images yourself

The Compose file builds both images from the repository root. To build them without Compose:

```bash
docker build -f deploy/Dockerfile.server -t halo-server .
docker build -f deploy/Dockerfile.web --build-arg HALO_API_URL=http://halo-server:8080 -t halo-web .
```

`halo-web` reads `HALO_API_URL`, the address of `halo-server`, twice. Next.js writes the forwarding rules for `/api`, `/oauth2`, `/.well-known`, `/saml` and `/scim` into the image at build time, so the build argument decides where those requests go. Server-rendered pages read the environment variable at runtime. Set both to the same address. The web container listens on port 3200 on all interfaces (`PORT` and `HOSTNAME`).

`halo-server` listens on `HALO_LISTEN`, which defaults to `:8080`. If you change it, change `HALO_API_URL` and the health check to match.

## Use your own reverse proxy

To put Halo behind a proxy you already run, remove the `caddy` service, publish `halo-web` on a local port, and forward every path on your domain to it. Your proxy must terminate TLS, and it must set `X-Forwarded-For` to the client's address after discarding any value the client sent.

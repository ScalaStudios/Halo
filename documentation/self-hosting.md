# Self-hosting

Halo runs on your own infrastructure with no external services: a Go server, a Next.js web interface, PostgreSQL, and a reverse proxy that terminates TLS. The repository ships everything needed to run it on one host with Docker Compose.

**Follow [deploy/README.md](../deploy/README.md) for the step-by-step guide.** It covers DNS, the environment file, starting the services, creating the first administrator, backups, upgrades, health checks and rotating `HALO_SECRET_KEY`.

## What runs

```
internet ──► caddy :80 :443 ──► halo-web :3200 ──► halo-server :8080 ──► postgres :5432
```

- **Caddy** obtains a certificate for your domain and forwards every request to the web interface.
- **halo-web** serves the console, sign-in pages and account portal, and forwards `/api`, `/oauth2`, `/.well-known`, `/saml` and `/scim` to the Go server, so everything shares one address.
- **halo-server** runs `halo serve`. It applies database migrations every time it starts.
- **PostgreSQL 17** stores all data.

Only Caddy is reachable from outside. The images are built from the repository: `deploy/Dockerfile.server` produces a static Go binary on a distroless base that runs as a non-root user, and `deploy/Dockerfile.web` produces a Next.js standalone server on Node.js 22.

## Decide before you install

- **The domain.** Halo's address is the OpenID Connect issuer that every application stores, and passkeys only work on the hostname they were created for. Moving Halo to a new hostname later means reconfiguring every application and giving everyone a new setup link. Pick a name you intend to keep, such as `auth.example.com`.
- **The secret key.** `HALO_SECRET_KEY` encrypts the token signing keys, authenticator-app secrets and other stored secrets, and a database backup is useless without the key that was in use when it was taken. Store it in your password manager when you create it. To replace it, stop Halo and run `halo rotate-secret-key` as described in [Rotate HALO_SECRET_KEY](../deploy/README.md#rotate-halo_secret_key); recovery codes do not survive a rotation, so the people who had them generate new ones.
- **Email.** Halo works without email: invite and reset links are shown to the administrator who creates them, to pass on. With an SMTP server in the `HALO_SMTP_*` variables, Halo also emails those links and people can sign in with magic links.

## Other ways to run Halo

The Compose files are one way to run the same three programs. To run them another way, such as on Kubernetes:

- Build the two images with the commands in [deploy/README.md](../deploy/README.md#build-the-images-yourself), and pass the Go server's address as `HALO_API_URL` both when you build the web image and when you run it.
- Expose only the web interface, behind a proxy that terminates TLS and sets `X-Forwarded-For`.
- Point liveness and readiness probes at port 8080, path `/healthz`, on the Go server. It answers `200` when the database responds.
- Give the Go server the variables in [Configuration](configuration.md), with `HALO_PUBLIC_URL` set to the public https address.

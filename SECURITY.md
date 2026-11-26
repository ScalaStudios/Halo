# Security policy

Halo is an identity platform, so its security matters to everything that trusts it for sign-in. Thank you for reporting problems responsibly.

## Supported versions

Halo is in early development and has not published a release. Until version 1.0, only the latest commit on the `main` branch receives security fixes. Older commits and forks are not patched.

| Version | Supported |
| --- | --- |
| Latest `main` | Yes |
| Anything older | No |

## Report a vulnerability

Report vulnerabilities privately through GitHub's private vulnerability reporting: open the Halo repository, go to **Security**, and choose **Report a vulnerability**, or go straight to [the report form](https://github.com/ScalaStudios/Halo/security/advisories/new). Only you and the maintainers can read the report.

Do not report a vulnerability in a public issue, pull request, discussion or chat. If you are unsure whether something is a security problem, report it privately anyway.

### What to include

- The affected part: a Go package such as `internal/auth`, a web route such as `/sign-in`, or the deployment files in `deploy/`.
- The commit you tested.
- Steps to reproduce against a local instance, or a proof of concept. The demo organization from `halo seed-demo` is a good test bed.
- The impact: what an attacker gains, and what they need first, such as an account, an administrator role or a network position.
- Relevant configuration, for example whether `HALO_DEV` was set or which reverse proxy sits in front of Halo.
- Whether you would like to be credited, and under which name.

Test only against instances you run yourself, and do not access or change other people's data.

### What happens next

These are the maintainers' targets, counted from the day the report arrives:

| Step | Target |
| --- | --- |
| Acknowledge the report | 3 working days |
| Confirm or rule out the vulnerability, with a severity | 10 working days |
| Fix or mitigate critical and high severity issues on `main` | 30 days |
| Publish an advisory and credit the reporter | When the fix is on `main`, and no later than 90 days after the report unless agreed otherwise |

You receive updates in the private report until the advisory is published.

## Security model in brief

The [security model](https://halo.scala.gg/docs/security-model) documents these in detail.

- **No passwords by default.** People sign in with passkeys and security keys (WebAuthn) first, with authenticator-app codes (TOTP), single-use recovery codes, or emailed single-use magic links as alternatives. Passkeys are bound to the hostname of `HALO_PUBLIC_URL`. A passkey whose signature counter goes backwards is rejected as possibly cloned. After 5 failed passwords, authenticator or recovery codes within 15 minutes, Halo refuses further passwords and codes for that account until the window passes. To slow password spraying, where one guess is tried against many accounts, Halo also refuses passwords from a client IP address with 4 times the lockout threshold of failed passwords (20 with the defaults) within the same window, whichever accounts they were for. The count is kept in the sign-in log in PostgreSQL, so it holds across replicas, and the IP address is read from `X-Forwarded-For` only when the request comes from `HALO_TRUSTED_PROXIES`.
- **Client certificates come only from a trusted proxy.** Halo doesn't terminate TLS, so client certificate sign-in reads the certificate from the header `HALO_CLIENT_CERT_HEADER` names, and only on requests from `HALO_TRUSTED_PROXIES`. With it unset, Halo reads no certificates. The proxy must check the certificate during the TLS handshake and replace that header when a browser sends it, because a certificate on its own is public. Halo then checks the chain against the trusted certificate authorities, the validity period, client authentication usage, and revocation with the OCSP responder or CRL the certificate names over HTTP. When the certificate names a responder or CRL and none answers over HTTP, the sign-in is refused. A header sent more than once is ignored. Certificates match people by email address, so they never sign in an account that holds or is eligible for an administrator role or is an emergency access account. The same applies to the first sign-in with an identity provider, and changes to identity providers, certificate authorities and Windows sign-in share one approval setting.
- **Kerberos tickets must be for Halo's own key.** Windows sign-in decrypts the ticket with the keytab for Halo's own service principal, accepts it within 5 minutes of clock skew only, refuses tickets for any other service principal, and refuses an authenticator any Halo server already accepted, with the replay cache in PostgreSQL. It refuses NTLM and never reads the PAC. A principal signs in only the person it is linked to, or the person whose email address its realm maps it to, never someone matched by display name. Halo never links a principal by email address to an account that holds or is eligible for an administrator role or is an emergency access account. The sign-in page sends the Negotiate challenge without a click only to addresses in trusted network zones an administrator chose.
- **Passwords, when turned on, are hashed with Argon2id.** Passwords are off until a security administrator turns them on, optionally for chosen groups only, and by default they also need an authenticator-app code. Each password is stored as an Argon2id hash (64 MiB, 3 passes, 4 lanes, a 16-byte random salt) with its parameters, after NFKC normalization, and compared in constant time. New passwords need 15 to 256 characters and no other composition rules. Halo refuses passwords that the Have I Been Pwned range API reports as breached, sending only the first 5 characters of the password's SHA-1 hash; when the API is unreachable Halo accepts the password and logs a warning, and the check can be turned off for installations without internet access.
- **Secrets are hashed.** Session tokens, client secrets, API keys, refresh tokens, and invite, reset and magic link tokens are stored only as SHA-256 hashes and compared in constant time. Recovery codes are stored as HMAC-SHA256 with a key derived from `HALO_SECRET_KEY`. Client secrets and API keys are shown once, when they are created.
- **Keys are sealed with `HALO_SECRET_KEY`.** OpenID Connect and SAML signing keys, authenticator-app seeds, SCIM provisioning tokens, identity provider client secrets, webhook signing secrets, the SSH certificate authority, the Kerberos keytab and queued email are encrypted with AES-256-GCM under `HALO_SECRET_KEY`, and opaque access tokens and authorization codes are encrypted with a key derived from it. Keep the key secret and back it up. `halo rotate-secret-key` replaces it, as [Rotate HALO_SECRET_KEY](https://halo.scala.gg/docs/self-hosting#rotate-halo_secret_key) describes.
- **Same-origin protection.** Every request to `/api/` that changes state (any method other than GET, HEAD and OPTIONS) must come from Halo's own pages. A browser request passes when `Sec-Fetch-Site` is `same-origin` or `none`. Without that header, the request passes only when `Origin` is absent or equals the origin of `HALO_PUBLIC_URL`. Anything else fails with `ERR_CROSS_SITE`.
- **Rate limits.** Halo counts requests to `/api/` per API key and per session, and requests to `/scim/` per API key, in one-minute windows: by default 600 a minute for each API key, 1,200 for each session and 3,000 for each key on SCIM. Each service account has its own limit for the management API. Requests over the limit fail with `429` and `Retry-After`. The counts are kept in PostgreSQL, so they hold across replicas.
- **Sessions.** The `halo_session` cookie is HttpOnly, SameSite=Lax and Secure, and lasts 12 hours. Signing out or revoking a session ends it on the server and revokes the OpenID Connect tokens issued under it.
- **HTTPS outside development.** Halo refuses to start when `HALO_PUBLIC_URL` does not use https, unless `HALO_DEV=1`. Never set `HALO_DEV` in production: it also drops the Secure flag from cookies and allows plain http.
- **Administration is role-checked and audited.** Every administrative change checks the actor's role and writes an audit event in the same database transaction. Only a global administrator can change an account that holds or is eligible for an administrator role. An LDAP sync that a global administrator neither started nor last configured doesn't suspend, restore or update the profile of those accounts, a lifecycle rule doesn't suspend them or end their sessions, and granting the first role invalidates setup links, access passes and temporary passwords issued before it. A group-limited administrator can't make a change that puts someone in a rule-based group outside their groups. The last active person with the standing global administrator role can't be suspended, deprovisioned, deleted or lose the role, on any path including SCIM, and emergency access accounts can't be suspended through SCIM.
- **OpenID Connect hygiene.** Single-page and native applications must use PKCE with S256. Redirect URIs must match a registered address exactly and use https, except for `localhost` and `127.0.0.1`. Refresh tokens rotate by default, and a used refresh token is rejected. Disabling an application revokes its tokens, and Halo refuses refresh and userinfo requests for people who are no longer active or no longer assigned to the application.

# Security policy

Halo is an identity platform, so its security matters to everything that trusts it for sign-in. Thank you for reporting problems responsibly.

## Supported versions

Halo is in early development and has not published a release. Until version 1.0, only the latest commit on the `main` branch receives security fixes. Older commits and forks are not patched.

| Version | Supported |
| --- | --- |
| Latest `main` | Yes |
| Anything older | No |

## Report a vulnerability

Report vulnerabilities privately through the repository host's private vulnerability reporting: open the Halo repository, go to **Security**, and choose **Report a vulnerability**. Only you and the maintainers can read the report.

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

The [security model](documentation/security-model.md) documents these in detail.

- **No passwords.** People sign in with passkeys and security keys (WebAuthn) first, with authenticator-app codes (TOTP), single-use recovery codes, or emailed single-use magic links as alternatives. Passkeys are bound to the hostname of `HALO_PUBLIC_URL`. A passkey whose signature counter goes backwards is rejected as possibly cloned. After 5 failed authenticator or recovery codes within 15 minutes, Halo refuses further codes for that account until the window passes.
- **Secrets are hashed.** Session tokens, client secrets, API keys, refresh tokens, invite, reset and magic link tokens, and recovery codes are stored only as SHA-256 hashes and compared in constant time. Client secrets and API keys are shown once, when they are created.
- **Keys are sealed with `HALO_SECRET_KEY`.** OpenID Connect and SAML signing keys, authenticator-app seeds, provisioning tokens and queued email are encrypted with AES-256-GCM under `HALO_SECRET_KEY`, and opaque access tokens and authorization codes are encrypted with a key derived from it. Keep the key secret and back it up: Halo cannot rotate it yet.
- **Same-origin protection.** Every request to `/api/` that changes state (any method other than GET, HEAD and OPTIONS) must come from Halo's own pages. A browser request passes when `Sec-Fetch-Site` is `same-origin` or `none`. Without that header, the request passes only when `Origin` is absent or equals the origin of `HALO_PUBLIC_URL`. Anything else fails with `ERR_CROSS_SITE`.
- **Sessions.** The `halo_session` cookie is HttpOnly, SameSite=Lax and Secure, and lasts 12 hours. Signing out or revoking a session ends it on the server and revokes the OpenID Connect tokens issued under it.
- **HTTPS outside development.** Halo refuses to start when `HALO_PUBLIC_URL` does not use https, unless `HALO_DEV=1`. Never set `HALO_DEV` in production: it also drops the Secure flag from cookies and allows plain http.
- **Administration is role-checked and audited.** Every administrative change checks the actor's role and writes an audit event in the same database transaction. Only a global administrator can change an account that holds an administrator role, and the last active global administrator cannot be suspended.
- **OpenID Connect hygiene.** Single-page and native applications must use PKCE with S256. Redirect URIs must match a registered address exactly and use https, except for `localhost` and `127.0.0.1`. Refresh tokens rotate by default, and a used refresh token is rejected. Disabling an application revokes its tokens, and Halo refuses refresh and userinfo requests for people who are no longer active or no longer assigned to the application.

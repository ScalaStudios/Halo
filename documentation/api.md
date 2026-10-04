# API

The Halo console and account portal are built entirely on the JSON API described here, so anything they do, you can do with a script.

**The reference is the OpenAPI 3.1 document at `/api/v1/openapi.yaml`** on your Halo server, for example `https://auth.example.com/api/v1/openapi.yaml`, and in the repository at [`api/openapi.yaml`](../api/openapi.yaml). It lists every route with its roles, request and response schemas, and the error codes it returns. The console links to it under **SDKs and reference**. This page explains what the routes have in common.

## Base URL

```text
https://auth.example.com/api/v1
```

The web interface forwards `/api` to the Go server, so use Halo's public address. Inside your own network you can also call the Go server directly on `HALO_LISTEN`, for example `http://halo-server:8080/api/v1` in the [Docker Compose deployment](../deploy/README.md).

## Authentication

The API accepts three kinds of credentials.

### Session cookie

The `halo_session` cookie is the one the browser gets when a person signs in. To call the API as yourself from a script:

1. Sign in to Halo in your browser.
2. Copy the value of the `halo_session` cookie from the browser's developer tools. The cookie is HttpOnly, so scripts on the page cannot read it.
3. Send it with every request:

   ```bash
   curl https://auth.example.com/api/v1/me -H "Cookie: halo_session=$HALO_SESSION"
   ```

The session lasts as long as the session lifetime, 12 hours by default, and signing out ends it. In development, `halo dev-session --email you@example.com` prints a session for any user (it requires `HALO_DEV=1`).

### API keys

Automation uses API keys of [service accounts](provisioning.md#service-accounts). Send the key as a bearer token:

```bash
curl https://auth.example.com/api/v1/users -H "Authorization: Bearer hlk_…"
```

- A key works on `/api` only when it has the `api` scope, and requests made with it have the roles of its service account.
- Keys are ignored on the personal routes under `/api/v1/me` and on the sign-in routes under `/api/v1/auth`, which act on a signed-in person.
- Halo stores only a hash of each key. A key stops working when it is revoked, when it expires, or when its service account is disabled.

### Halo CLI access tokens

The personal routes under `/api/v1/me/` also accept the access token that [`halo login`](infrastructure-access.md#use-the-halo-command) obtained, as a bearer token. `halo ssh-cert` uses this to request certificates. `GET /api/v1/me` itself requires a session.

### Roles

What a request may do depends on the roles of the person or service account behind it; see [Concepts](concepts.md#roles). Each operation in the OpenAPI document states the roles it requires. Requests without valid credentials get `401` with `ERR_UNAUTHENTICATED`, and requests your roles do not allow get `403` with `ERR_FORBIDDEN`.

## Same-origin rule

Requests that change something (any method other than GET, HEAD and OPTIONS) must not come from another website. Halo blocks them with `403` and `ERR_CROSS_SITE` when the browser marks them as cross-site with `Sec-Fetch-Site`, or when their `Origin` header names an origin other than `HALO_PUBLIC_URL`. Command-line tools such as curl send neither header and pass. The [security model](security-model.md#same-origin-protection) has the exact rule.

## Requests and responses

- Request and response bodies are JSON with camelCase field names. Send `Content-Type: application/json`.
- Request bodies may be at most 1 MiB. Halo rejects invalid JSON and fields it does not recognise with `400` and `ERR_INVALID_JSON`.
- `PATCH` routes change only the fields you send. `PUT` routes replace the whole object or list; send every field.
- Times are RFC 3339 strings, such as `2026-10-04T10:00:10Z`.
- Ids are a prefix and a 26-character ULID, such as `usr_01J9Z8Q4M2N5P7R9T1V3X5Z7B9`. Common prefixes: `usr_` users and service accounts, `grp_` groups, `app_` applications, `crd_` client secrets, `ses_` sessions, `evt_` sign-in events, `aud_` audit events, `pkg_` access packages, `req_` access requests, `rev_` access reviews, `lcr_` lifecycle rules, `pol_` policies, `nz_` network zones, `dev_` devices, `rsk_` risk events, `key_` API keys, `idp_` identity providers, `whk_` webhooks, `api_` API resources, `spm_` SSH principal mappings and `dom_` domains.
- Actions that return nothing answer `204 No Content`.
- Every JSON response carries `Cache-Control: no-store`.

## Errors

Every error has the same shape:

```json
{"error": {"code": "ERR_NOT_FOUND", "message": "This user was not found. It may have been deleted."}}
```

Match on `code`, which is stable. Show `message` to people: it says what happened and what to do next.

| Status | Code | Meaning |
| --- | --- | --- |
| 400 | `ERR_INVALID_JSON` | The body is not valid JSON, is larger than 1 MiB, or has an unknown field. |
| 401 | `ERR_UNAUTHENTICATED` | No valid session, key or token. |
| 403 | `ERR_FORBIDDEN` | Your roles do not allow this change. |
| 403 | `ERR_CROSS_SITE` | The request came from another website. |
| 404 | `ERR_NOT_FOUND` | The item does not exist. |
| 404 | `ERR_NO_ROUTE` | No API route matches the path and method. |
| 409 | `ERR_CONFLICT` | The change conflicts with the current state, such as a name that already exists or a group that is still in use. |
| 422 | `ERR_INVALID` | A field is missing or invalid. The message names the field and the expected format. |
| 500 | `ERR_INTERNAL` | Halo could not complete the request. The server log has the details. |

The sign-in routes add their own codes:

| Status | Code | Meaning |
| --- | --- | --- |
| 400 | `ERR_CEREMONY_EXPIRED` | The passkey or authenticator setup step expired. Start again. |
| 400 | `ERR_REGISTRATION_FAILED` | Halo could not verify the new passkey. |
| 401 | `ERR_PASSKEY_REJECTED` | Halo could not verify the passkey. |
| 401 | `ERR_CODE_MISMATCH` | The authenticator or recovery code did not match, or was already used. |
| 401 | `ERR_REAUTH_REQUIRED` | The application asks the person to sign in again. |
| 403 | `ERR_ACCOUNT_SUSPENDED` | The account is suspended. |
| 403 | `ERR_NOT_ASSIGNED` | The person is not in a group assigned to the application. |
| 403 | `ERR_BLOCKED_BY_POLICY` | An access policy blocked the sign-in. |
| 403 | `ERR_STRONGER_AUTH_REQUIRED` | An access policy requires a passkey or security key. |
| 404 | `ERR_AUTH_REQUEST_EXPIRED` | The sign-in request from the application expired or was already used. |
| 404 | `ERR_LINK_EXPIRED` | The invite, reset or magic link expired or was already used. |
| 404 | `ERR_DEVICE_CODE` | The `halo login` code is wrong, expired or already used. |
| 409 | `ERR_LAST_METHOD` | The sign-in method or linked account is the person's last one and cannot be removed. |
| 429 | `ERR_TOO_MANY_ATTEMPTS` | Too many failed codes. Wait for the lockout window to pass. |

Federated sign-in redirects to the sign-in page with its codes instead of answering JSON; [Federation](federation.md#errors) lists them.

Other areas return:

| Status | Code | Meaning |
| --- | --- | --- |
| 403 | `ERR_SELF_APPROVAL` | You cannot approve or deny your own access request. |
| 403 | `ERR_NO_PRINCIPALS` | None of your groups maps to an SSH principal. |
| 404 | `ERR_NOT_CONFIGURED` | Outbound provisioning is not set up for the application. |
| 409 | `ERR_BUILT_IN` | The Halo CLI application cannot be deleted. |
| 413 | `ERR_TOO_LARGE` | The logo is larger than 256 KB. |
| 422 | `ERR_NOT_VERIFIED` | Halo did not find the domain's verification TXT record. |
| 422 | `ERR_TOKEN_UNREADABLE` | A saved SCIM token cannot be decrypted with the current `HALO_SECRET_KEY`. |
| 502 | `ERR_SCIM_CONNECTION` | The application's SCIM endpoint could not be reached or answered with an error. |

## Route groups

| Routes | Covers | Guide |
| --- | --- | --- |
| `/organization`, `/branding/logo` (GET), `/auth/methods`, `/auth/providers`, `/ssh/ca.pub`, `/openapi.yaml` | Public information. No authentication. | |
| `/auth/…` | Passkey, code, magic link, federated and enrollment sign-in, and sign-out | [Security model](security-model.md#authentication) |
| `/device/{code}` | Approving `halo login` in the browser | [Infrastructure access](infrastructure-access.md) |
| `/me`, `/me/…` | The signed-in person's profile, methods, sessions, applications, groups, sign-ins, linked accounts, devices, access requests, approvals, reviews and SSH certificates | [Concepts](concepts.md) |
| `/users`, `/groups` | The directory | [Concepts](concepts.md) |
| `/applications` | Applications, secrets, group assignments and SAML settings | [Concepts](concepts.md#applications) |
| `/sessions`, `/sign-ins`, `/audit`, `/overview` | Activity | [Concepts](concepts.md#sign-in-log-and-audit-log) |
| `/access-packages`, `/access-requests`, `/access-reviews`, `/lifecycle/…` | Governance | [Governance](governance.md) |
| `/policies`, `/network-zones`, `/methods`, `/devices`, `/risk-events` | Access policies | [Policies](policies.md) |
| `/service-accounts`, `/api-keys`, `/provisioning/applications` | Automation and outbound SCIM | [Provisioning](provisioning.md) |
| `/identity-providers` | Federation | [Federation](federation.md) |
| `/settings`, `/branding/logo`, `/domains`, `/export` | Organization settings, branding, domains and data export | [Configuration](configuration.md#organization-settings) |
| `/signing-keys`, `/saml/certificate/rotate`, `/saml/certificates/{id}` | OpenID Connect signing keys and SAML certificates, and rotating them | [Security model](security-model.md#tokens) |
| `/webhooks` | Webhook endpoints and deliveries | [Webhooks](webhooks.md) |
| `/api-resources` | Your APIs and their grants | [API resources](api-resources.md) |
| `/ssh/…` | The SSH certificate authority | [Infrastructure access](infrastructure-access.md) |
| `/dev/outbox` | The email outbox, only with `HALO_DEV=1` | [Email](email.md#read-the-outbox-in-development) |

## Protocol endpoints

These endpoints follow their specifications rather than the conventions above.

| Method | Path | Description |
| --- | --- | --- |
| GET | `/.well-known/openid-configuration` | OpenID Connect discovery document. |
| GET | `/oauth2/authorize` | Authorization endpoint. |
| POST | `/oauth2/token` | Token endpoint: authorization code, refresh token, client credentials for service applications, and the device code grant for the Halo CLI. |
| POST | `/oauth2/device_authorization` | Device authorization endpoint. Only the built-in Halo CLI application may use the device flow. |
| GET, POST | `/oauth2/userinfo` | Claims about the person, with the access token as a bearer token. |
| GET | `/oauth2/keys` | The public keys that verify ID tokens and JWT access tokens (JWKS). |
| POST | `/oauth2/introspect` | Token introspection, authenticated with the application's client secret over HTTP Basic. |
| POST | `/oauth2/revoke` | Token revocation. |
| GET | `/oauth2/logout` | End-session endpoint. Revokes the application's tokens for the person and, with an `id_token_hint`, ends the Halo session it names. |
| GET, POST | `/saml/…` | The SAML 2.0 identity provider: `/saml/metadata`, `/saml/sso`, `/saml/certificate` and `/saml/launch/{appId}`. |
| | `/scim/v2/…` | The SCIM 2.0 server, authenticated with an API key that has the `scim` scope. |
| GET | `/healthz` | On the Go server only, not forwarded by the web interface. `200 ok` when the database answers. |

[Generic OpenID Connect](integrations/generic-oidc.md), [SAML 2.0](integrations/generic-saml.md) and [Provisioning](provisioning.md#inbound-scim) explain how to use them.

## Examples

### Invite a person

```bash
curl -X POST https://auth.example.com/api/v1/users \
  -H "Authorization: Bearer $HALO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email": "sam@example.com", "name": "Sam Lee", "department": "Engineering"}'
```

The response contains the new `user` and the `enrollUrl` to send to Sam, and `emailed` tells you whether Halo emailed it already. The link works once and expires at `expiresAt`, 7 days later by default. The key's service account needs the user administrator role.

### Assign roles

Only a global administrator can assign roles, in the console under **Roles** or with the API. Send the complete list of roles; an empty list removes every role.

```bash
curl -X PUT https://auth.example.com/api/v1/users/usr_01J9Z…/roles \
  -H "Cookie: halo_session=$HALO_SESSION" \
  -H "Content-Type: application/json" \
  -d '{"roles": ["user_admin", "auditor"]}'
```

The role keys are `global_admin`, `security_admin`, `user_admin`, `helpdesk_admin`, `app_admin` and `auditor`. `GET /api/v1/users/{id}` returns the role names, such as `User administrator`, in `roles`.

### Register an application with two redirect URIs

```bash
curl -X POST https://auth.example.com/api/v1/applications \
  -H "Authorization: Bearer $HALO_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "Wiki", "protocol": "oidc", "type": "web",
       "redirectUris": ["https://wiki.example.com/auth/oidc.callback", "https://wiki.internal.example.com/auth/oidc.callback"],
       "groupIds": ["grp_01J9Z…"]}'
```

Store the `clientSecret` from the response: Halo does not show it again. Change the redirect URIs later with `PATCH /api/v1/applications/{id}`.

### Approve every pending request you can decide

```bash
curl -s https://auth.example.com/api/v1/me/approvals -H "Cookie: halo_session=$HALO_SESSION" |
  jq -r '.[].id' |
  while read -r id; do
    curl -s -X POST "https://auth.example.com/api/v1/access-requests/$id/approve" -H "Cookie: halo_session=$HALO_SESSION"
  done
```

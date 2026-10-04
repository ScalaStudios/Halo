# Generic OpenID Connect

Connect any application that supports OpenID Connect sign-in. The pages for [Grafana](grafana.md), [Forgejo](forgejo.md), [Nextcloud](nextcloud.md), [Kubernetes](kubernetes.md), [Proxmox VE](proxmox.md), [Outline](outline.md) and [Headscale](headscale.md) apply the same steps to specific applications.

The examples use `https://auth.example.com` as Halo's address. Replace it with your `HALO_PUBLIC_URL`.

## Register the application in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose where the application runs:
   - **Web application** if it runs on a server and can keep a secret. Pick the **Custom** template, or a template for a known application.
   - **Single-page app** if it runs entirely in the browser.
   - **Native or CLI** for desktop, mobile and command-line tools.
4. Choose **Continue**, enter the application's name and its redirect URI, and choose **Create application**.
5. Copy the values Halo shows: the issuer, the client ID, and for web applications the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may sign in. Until you assign a group, nobody can sign in.

For a service that calls an API with its own credentials, choose **OAuth 2.0 service** in step 2 instead; see [client credentials](#client-credentials).

## Values for the application

Most applications need only the discovery URL, the client ID and the client secret. Use the rest when an application asks for each endpoint separately.

| Setting | Value |
| --- | --- |
| Issuer | `https://auth.example.com` |
| Discovery | `https://auth.example.com/.well-known/openid-configuration` |
| Authorization endpoint | `https://auth.example.com/oauth2/authorize` |
| Token endpoint | `https://auth.example.com/oauth2/token` |
| Userinfo endpoint | `https://auth.example.com/oauth2/userinfo` |
| JWKS (signing keys) | `https://auth.example.com/oauth2/keys` |
| Introspection endpoint | `https://auth.example.com/oauth2/introspect` |
| Revocation endpoint | `https://auth.example.com/oauth2/revoke` |
| End-session endpoint | `https://auth.example.com/oauth2/logout` |
| Client ID | Starts with `hl_` |
| Client secret | Starts with `hls_`; web applications and services only |
| Scopes | `openid profile email groups`, plus `offline_access` for a refresh token |
| Response type | `code` |
| PKCE | `S256` |
| Signing algorithm | `RS256` |

The issuer must match exactly, without a trailing slash. Every token's `iss` claim carries it.

## Client authentication

- **Web applications** authenticate to the token endpoint with their client secret, either with HTTP Basic (`client_secret_basic`) or as `client_id` and `client_secret` form fields (`client_secret_post`). Use PKCE as well if the application supports it.
- **Single-page and native applications** are public clients without a secret. They must send a PKCE challenge with `code_challenge_method=S256`; Halo refuses their sign-in requests otherwise.
- **Token introspection** accepts the client secret over HTTP Basic only.

## Redirect URIs

Halo sends people back only to redirect URIs registered on the application, and the URI in the request must match one exactly: scheme, host, port, path and trailing slash. Halo registers only `https://` addresses without a `#fragment`, except `http://localhost` and `http://127.0.0.1`.

For native applications, Halo ignores the port of a loopback redirect URI: registering `http://localhost:8000` also accepts `http://localhost:18000`. Path and query must still match.

The console's form registers one redirect URI when you create the application. Add more, and post-logout redirect URIs, on the application's page afterwards, or create the application through the [API](../api.md#register-an-application-with-two-redirect-uris) with several.

## Claims

| Claim | Scope | Value |
| --- | --- | --- |
| `sub` | `openid` | The person's Halo id, such as `usr_01J…`. Stable for the life of the account. |
| `email` | `email` | The person's email address. |
| `email_verified` | `email` | `true` once Halo has verified the address; see [Security model](../security-model.md#claims). |
| `name` | `profile` | The person's name. |
| `preferred_username` | `profile` | The person's email address. |
| `groups` | `groups` | The names of every group the person belongs to, assigned and rule-based. |
| `amr` | `openid` | `["hwk", "mfa"]` after a passkey or security key, `["fed"]` after an identity provider, `["otp"]` after an authenticator app, recovery code or magic link. |
| `sid` | `openid` | The id of the Halo session, in ID tokens. |

Halo includes the requested claims in the ID token as well as in the userinfo response, so applications that read only the ID token still get `groups`.

Halo's `groups` claim carries group **names**, such as `"Grafana editors"`, not ids. When you map groups to roles in an application, use the names exactly as they appear in the console.

## Tokens

| Token | Default lifetime | Notes |
| --- | --- | --- |
| ID token | 15 minutes | Signed with RS256. Verify it with the keys at `/oauth2/keys`. |
| Access token | 15 minutes | Opaque. Send it to `/oauth2/userinfo` or check it with `/oauth2/introspect`. Applications granted scopes on an [API resource](../api-resources.md) receive a JWT instead. |
| Refresh token | 8 hours | Only with the `offline_access` scope. Each use returns a new refresh token, and the old one stops working, unless rotation is turned off for the application. |

Change the lifetimes and refresh token rotation on the application's page. New applications start with the defaults from **Security defaults**.

Halo checks that the person is still active and still assigned to the application on every token, refresh, userinfo and introspection request.

## Re-authentication and sign-out

- Send `prompt=login`, or a `max_age` in seconds, to make Halo ask the person to sign in again even when they have a Halo session.
- Send `prompt=none` to check for a Halo session without showing any page. Halo issues a code when the browser has a session it accepts for this application, and otherwise redirects back with `login_required` (no usable session) or `interaction_required` (not assigned, or blocked by an access policy).
- Send the person to `/oauth2/logout` with the ID token as `id_token_hint` to sign them out. Halo revokes the application's tokens and ends the Halo session named in the token's `sid` claim, so other applications need a new sign-in too. Add `post_logout_redirect_uri` to send the person back afterwards; it must be registered on the application as a post-logout redirect URI, which you add on the application's page or send as `postLogoutUris` to the [API](../api.md).

## Client credentials

A service application gets a client secret and no redirect URI. Request a token with the client credentials grant:

```bash
curl -u "hl_…:hls_…" https://auth.example.com/oauth2/token \
  -d grant_type=client_credentials -d scope="billing:read"
```

Halo returns a JWT access token whose `sub` is the client ID, with only the requested scopes that are registered on the application or granted to it on an [API resource](../api-resources.md). Prefer API resources for your own APIs: the token then carries the API's identifier in `aud`.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| "The requested redirect_uri is missing in the client configuration." | The redirect URI in the request does not match a registered one exactly. Compare scheme, host, port, path and trailing slash with the application's **Authentication** tab in the console. |
| "You don't have access to this application. Ask your administrator to assign it to you." | The person is not in a group assigned to the application. Assign a group under **Users & groups**, or add the person to an assigned group. |
| `invalid_client` with "The application does not exist, is disabled, or does not use OpenID Connect." | Check the client ID, and enable the application if it is disabled. |
| "This application must use PKCE: send code_challenge with code_challenge_method=S256." | The application is registered as a single-page or native app. Enable PKCE with S256 in the application, or register it as a web application with a secret. |
| "This sign-in was completed in a different browser session. Start again from the application." | The sign-in finished in a different browser or after the person signed out. Start again from the application in one browser. |
| `invalid_grant` with "The user is no longer active or no longer assigned to this application." | The person was suspended or removed from the application's groups after signing in. |
| No `groups` claim | Add `groups` to the scopes the application requests. |
| The application rejects the issuer | Use `https://auth.example.com` exactly as `HALO_PUBLIC_URL` sets it, without a trailing slash. |

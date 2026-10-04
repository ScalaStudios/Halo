# Federation

Halo can let people sign in with an account they already have at another identity provider: Google, Microsoft Entra ID, GitHub, or any OpenID Connect provider. Halo stays the identity provider for your applications; the external provider only proves who someone is when they sign in to Halo.

Security administrators manage identity providers; every administrator can read them.

## How a federated sign-in works

1. The person chooses **Continue with** *provider* on the sign-in page.
2. Halo sends them to the provider with PKCE (S256), a random state that a `halo_federation` cookie binds to their browser for 10 minutes, and, for every kind except GitHub, a nonce.
3. The provider sends them back to Halo's callback URL, and Halo exchanges the code and verifies the response.
4. Halo finds the Halo account, as described in the next section.
5. The sign-in then goes through the same checks as any other: suspended and deprovisioned accounts are refused, the application must be assigned to one of the person's groups, and the [access policies](policies.md) decide with the sign-in method `federated`.

The session's method is `federated`, and the `amr` claim Halo sends applications is `["fed"]`. A federated sign-in is only as strong as the provider's own sign-in, so policies that require a passkey or security key refuse it.

## How Halo finds the account

1. **An existing link.** If the provider account was linked to a Halo account before, Halo uses that account. Halo identifies the provider account by its subject, not by its email address, so a changed address at the provider does not matter.
2. **A matching email address.** Otherwise, the provider must confirm the email address:
   - OpenID Connect providers, including Google, must send `email_verified: true`. For Microsoft Entra ID, Halo also accepts the `xms_edov` claim.
   - For GitHub, Halo uses the account's primary email address when GitHub marks it verified.

   The address's domain must also be in the provider's **Allowed email domains**, when the list is not empty. If a Halo person has that address, Halo links the provider account to them and records `user.identity.link` in the audit log. Service accounts are never linked.
3. **A new account.** If no Halo account has the address and **Create accounts on first sign-in** is on, Halo creates an active account with the name from the provider, adds it to the groups under **Add new people to**, and records `user.create`. Creating accounts requires at least one allowed domain, so only people from your organization get one.
4. Otherwise the sign-in fails with `ERR_FEDERATION_NO_ACCOUNT`.

Invited people who have not signed in yet can sign in this way; their first sign-in makes them active. This is also how people provisioned over [SCIM](provisioning.md#inbound-scim) usually sign in for the first time.

## Add a provider

Every provider needs Halo's callback URL, which the **Identity providers** page shows:

```text
https://auth.example.com/api/v1/auth/federated/callback
```

Then:

1. Register Halo with the provider, as described for each kind below, and note the client ID and client secret.
2. In the console, open **Identity providers** and choose **Add identity provider**.
3. Pick the kind and enter a **Name**. People see it on the button, as in "Continue with Google".
4. Enter the **Client ID**, the **Client secret**, and the kind-specific address.
5. Optionally change the **Scopes**. The defaults are `openid email profile`, and `read:user user:email` for GitHub. Halo always adds `openid` for every kind except GitHub.
6. Enter the **Allowed email domains**, such as `example.com`. Leave the list empty to accept every domain for linking; creating accounts needs at least one.
7. Choose whether to **Create accounts on first sign-in** and which assigned groups new people join.
8. Turn on **Show on the sign-in page** and **Turned on**, and choose **Save provider**.

Halo encrypts the client secret with `HALO_SECRET_KEY` and never shows it again. To change other settings later, leave the secret empty to keep it.

### Google

1. In the Google Cloud console, create an OAuth client ID of type **Web application**.
2. Add the callback URL as an authorized redirect URI.
3. In Halo, choose the Google kind. The issuer is always `https://accounts.google.com`.

To limit sign-in to your Google Workspace, add your domain to the allowed email domains.

### Microsoft Entra ID

1. In the Entra admin center, register an application with a **Web** redirect URI set to the callback URL, and create a client secret.
2. Under **Token configuration**, add the optional claims `email` and `xms_edov` to the ID token. Entra ID does not send `email_verified`, so without `xms_edov` Halo cannot match people by email address on their first sign-in.
3. In Halo, choose the Microsoft kind and enter the **Directory (tenant) ID**, a GUID such as `72f988bf-86f1-41af-91ab-2d7cd011db47`. Halo builds the issuer `https://login.microsoftonline.com/<tenant id>/v2.0`. Domain names and `common` are not accepted, so only accounts from your tenant can sign in.

### GitHub

1. On GitHub, create an OAuth app under **Settings → Developer settings → OAuth Apps**, with the callback URL as the **Authorization callback URL**, and generate a client secret.
2. In Halo, choose the GitHub kind. For GitHub Enterprise Server, enter its address as the **GitHub URL**; Halo calls its API under `/api/v3`.

GitHub uses OAuth rather than OpenID Connect, so Halo reads the account from GitHub's `/user` and `/user/emails` API and identifies it by its numeric id.

### Generic OpenID Connect

1. Register Halo as a confidential web client, or as a public client with PKCE, at the provider, with the callback URL as its redirect URI.
2. In Halo, choose the OpenID Connect kind and enter the **Issuer URL**. Halo reads `/.well-known/openid-configuration` from it.
3. Enter the client secret, or leave it empty for a public client.

The issuer must use https. In development with `HALO_DEV=1`, `http://localhost` and `http://127.0.0.1` also work.

## Linked accounts

People see the provider accounts linked to them in the account portal under **Security**, and can unlink them. Halo refuses to unlink the last way someone can sign in: they need another linked account at an enabled provider, or a passkey, security key or authenticator app. Recovery codes do not count.

Turning a provider off stops sign-in with it and hides its button; links stay, and work again when you turn it back on. Deleting a provider removes every link to it.

## Errors

When a federated sign-in fails, Halo sends the person back to the sign-in page with one of these codes. The last five are also recorded in the sign-in log.

| Code | Meaning |
| --- | --- |
| `ERR_PROVIDER_UNAVAILABLE` | The provider is turned off or was deleted. |
| `ERR_PROVIDER_UNREACHABLE` | Halo could not load the provider's discovery document. |
| `ERR_FEDERATION_STATE` | The sign-in attempt expired after 10 minutes, was already used, or came back in a different browser. |
| `ERR_FEDERATION_CANCELLED` | The provider returned an error, for example because the person cancelled. |
| `ERR_FEDERATION_FAILED` | Halo could not exchange the code or verify the ID token. Check the client ID, secret and issuer. |
| `ERR_FEDERATION_EMAIL` | The provider did not confirm the email address. |
| `ERR_FEDERATION_DOMAIN` | The email domain is not in the allowed domains. |
| `ERR_FEDERATION_NO_ACCOUNT` | No Halo account has the address, and the provider does not create accounts. |

## API

Identity providers are at `/api/v1/identity-providers`, linked accounts at `/api/v1/me/identities`, and the providers shown on the sign-in page at `GET /api/v1/auth/providers`; see the [API](api.md).

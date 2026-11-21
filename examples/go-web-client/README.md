# Example: a Go web app that signs in with Halo

An 80-line web app that delegates sign-in to Halo with OpenID Connect (authorization code + PKCE) and shows the ID token claims it receives.

1. In the Halo console, open **Applications → Add application → OpenID Connect → Web** and use the redirect URI `http://localhost:9000/callback`. Assign it to a group you belong to. Copy the client ID and the client secret from the last step.
2. Run the app:

   ```bash
   CLIENT_ID=hl_… CLIENT_SECRET=hls_… go run ./examples/go-web-client
   ```

3. Open http://localhost:9000 and choose **Sign in with Halo**.

Settings: `HALO_ISSUER` (default `http://localhost:3200`), `REDIRECT_URI` (default `http://localhost:9000/callback`), `LISTEN` (default `localhost:9000`), `ACR_VALUES` (optional, see below).

## Step-up sign-in

Add an authentication context under **Policies → Contexts**, for example `passkey-recent` with the methods Passkey and Security key and a maximum sign-in age of 5 minutes. Then run the app with `ACR_VALUES=passkey-recent`. If your Halo session was a password sign-in, or is older than 5 minutes, Halo asks you to sign in again with a passkey or security key and then returns to the app. The claims show `"acr": "passkey-recent"` next to `amr` and `auth_time`.

An API that needs a stronger sign-in for one action can tell its client so with RFC 9470:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer error="insufficient_user_authentication",
  error_description="A passkey sign-in in the last 5 minutes is required",
  acr_values="passkey-recent"
```

The client sends the person to Halo again with `acr_values=passkey-recent`, checks `acr` in the new ID token, and repeats the call with the new access token. Halo puts `acr` in ID tokens only, not in access tokens or introspection, so an API that checks the token itself should ask with `max_age=300` instead and compare it with `auth_time` from introspection.

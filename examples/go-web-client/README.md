# Example: a Go web app that signs in with Halo

An 80-line web app that delegates sign-in to Halo with OpenID Connect (authorization code + PKCE) and shows the ID token claims it receives.

1. In the Halo console, open **Applications → Add application → OpenID Connect → Web** and use the redirect URI `http://localhost:9000/callback`. Assign it to a group you belong to. Copy the client ID and the client secret from the last step.
2. Run the app:

   ```bash
   CLIENT_ID=hl_… CLIENT_SECRET=hls_… go run ./examples/go-web-client
   ```

3. Open http://localhost:9000 and choose **Sign in with Halo**.

Settings: `HALO_ISSUER` (default `http://localhost:3200`), `REDIRECT_URI` (default `http://localhost:9000/callback`), `LISTEN` (default `localhost:9000`).

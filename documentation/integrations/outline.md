# Outline

Sign in to the Outline wiki with Halo through Outline's OpenID Connect support.

The examples use `https://auth.example.com` for Halo and `https://wiki.example.com` for Outline.

## Register Outline in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Outline** template, then **Continue**.
4. Check the redirect URI and change the host to your Outline address:

   ```text
   https://wiki.example.com/auth/oidc.callback
   ```

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use Outline.

## Configure Outline

Add these variables to Outline's environment, for example its `.env` file, with your client ID and secret:

```bash
OIDC_CLIENT_ID=hl_…
OIDC_CLIENT_SECRET=hls_…
OIDC_ISSUER_URL=https://auth.example.com
OIDC_SCOPES=openid profile email
OIDC_USERNAME_CLAIM=preferred_username
OIDC_DISPLAY_NAME=Halo
```

Outline discovers Halo's endpoints from `OIDC_ISSUER_URL`. To set the endpoints explicitly instead, replace that line with:

```bash
OIDC_AUTH_URI=https://auth.example.com/oauth2/authorize
OIDC_TOKEN_URI=https://auth.example.com/oauth2/token
OIDC_USERINFO_URI=https://auth.example.com/oauth2/userinfo
```

Restart Outline. Its sign-in page now offers **Halo**, the value of `OIDC_DISPLAY_NAME`.

`OIDC_USERNAME_CLAIM=preferred_username` is Outline's default, written out here because Halo's `preferred_username` is the person's email address. Outline takes the email address and name from the `email` and `profile` scopes.

## Who can sign in

Halo decides who can sign in to Outline: only members of the groups assigned to the application. To give someone access, add them to one of those groups in Halo; to remove it, take them out or suspend their account.

## Check it

Open Outline in a private window, choose **Halo**, and sign in. Outline opens with your name and email address from Halo. For errors, see [troubleshooting](generic-oidc.md#troubleshooting).

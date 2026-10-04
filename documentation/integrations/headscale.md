# Headscale

Register users and nodes in Headscale by signing in with Halo, and limit access to members of a Halo group.

The examples use `https://auth.example.com` for Halo and `https://hs.example.com` for Headscale.

## Register Headscale in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Headscale** template, then **Continue**.
4. Check the redirect URI and change the host to your Headscale address:

   ```text
   https://hs.example.com/oidc/callback
   ```

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may join the network.

## Configure Headscale

1. Save the client secret to a file that only Headscale can read, for example `/etc/headscale/halo-client-secret`.

2. Add an `oidc` section to Headscale's `config.yaml`, with your client ID:

   ```yaml
   oidc:
     issuer: "https://auth.example.com"
     client_id: "hl_…"
     client_secret_path: "/etc/headscale/halo-client-secret"
     scope: ["openid", "profile", "email", "groups"]
     pkce:
       enabled: true
       method: S256
   ```

3. Restart Headscale.

People now register a node with `tailscale up --login-server https://hs.example.com`, which opens a sign-in link; signing in to Halo there creates their Headscale user from their Halo identity.

## Limit access by group

Halo already refuses sign-in to anyone who is not in a group assigned to the application. Headscale can check groups as well: Halo's `groups` claim lists the names of all of the person's groups, and Headscale reads it with `allowed_groups`:

```yaml
oidc:
  allowed_groups:
    - "Tailnet users"
```

Add `groups` to `scope`, as in the configuration above, or Halo leaves the claim out and Headscale refuses everyone. Use the group names exactly as they appear in Halo's console.

Headscale's `email_verified_required` setting is on by default, so it uses an email address only when the provider marks it as verified. Halo sends `email_verified: true` only for people whose address it has verified; [Claims](../security-model.md#claims) lists how an address becomes verified.

## Check it

On a device with Tailscale installed, run `tailscale up --login-server https://hs.example.com`, open the link it prints, and sign in to Halo. The node appears in `headscale nodes list` under your user. For errors, see [troubleshooting](generic-oidc.md#troubleshooting).

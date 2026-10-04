# Grafana

Sign in to Grafana with Halo through Grafana's generic OAuth support, and give people Grafana roles based on their Halo groups.

The examples use `https://auth.example.com` for Halo and `https://grafana.example.com` for Grafana.

## Register Grafana in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Grafana** template, then **Continue**.
4. Check the redirect URI and change the host to your Grafana address:

   ```text
   https://grafana.example.com/login/generic_oauth
   ```

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use Grafana.

The application page in the console shows the same configuration as below with your client ID filled in, under **View setup guide** right after creation and on the application's overview.

## Configure Grafana

1. Save the client secret to a file that only Grafana can read:

   ```bash
   printf '%s' 'hls_…' | sudo tee /etc/grafana/halo-client-secret > /dev/null
   sudo chown grafana:grafana /etc/grafana/halo-client-secret
   sudo chmod 600 /etc/grafana/halo-client-secret
   ```

2. Add this block to `grafana.ini`, with your client ID:

   ```ini
   [auth.generic_oauth]
   enabled = true
   name = Halo
   allow_sign_up = true
   client_id = hl_…
   client_secret = $__file{/etc/grafana/halo-client-secret}
   scopes = openid profile email groups
   auth_url = https://auth.example.com/oauth2/authorize
   token_url = https://auth.example.com/oauth2/token
   api_url = https://auth.example.com/oauth2/userinfo
   use_pkce = true
   groups_attribute_path = groups
   role_attribute_path = contains(groups[*], 'Grafana editors') && 'Editor' || 'Viewer'
   ```

3. Make sure `root_url` in the `[server]` section is Grafana's public address. Grafana builds the redirect URI from it, and it must match the one registered in Halo:

   ```ini
   [server]
   root_url = https://grafana.example.com
   ```

4. Restart Grafana. The sign-in page now shows **Sign in with Halo**.

`$__file{…}` makes Grafana read the secret from the file instead of keeping it in `grafana.ini`.

## Map groups to Grafana roles

Halo's `groups` claim lists the names of all of the person's groups. `role_attribute_path` is a JMESPath expression over the claims: in the block above, members of the Halo group **Grafana editors** become editors and everyone else becomes a viewer.

To make another group administrators, put it first:

```ini
role_attribute_path = contains(groups[*], 'Grafana admins') && 'Admin' || contains(groups[*], 'Grafana editors') && 'Editor' || 'Viewer'
```

Use the group names exactly as they appear in Halo's console. Grafana updates the role every time the person signs in.

## Check it

Open Grafana in a private window and choose **Sign in with Halo**. After you sign in to Halo, Grafana opens with your name and the role from your groups. The sign-in appears in Halo's console under **Sign-in logs**.

If Halo shows "You don't have access to this application", assign one of your groups to the application. For other errors, see [troubleshooting](generic-oidc.md#troubleshooting).

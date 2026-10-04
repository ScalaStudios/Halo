# Proxmox VE

Add Halo to Proxmox VE as an OpenID Connect realm, create Proxmox users on their first sign-in, and bring their Halo groups along.

The examples use `https://auth.example.com` for Halo and `https://pve.example.com:8006` for the Proxmox VE web interface.

## Register Proxmox VE in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Proxmox VE** template, then **Continue**.
4. Change the redirect URI to the address you open the Proxmox VE web interface at, including the port and without a trailing slash:

   ```text
   https://pve.example.com:8006
   ```

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use Proxmox VE.

Halo matches redirect URIs exactly. If you reach Proxmox VE at more than one address, such as each node's own hostname, register every address; the console's form takes one, and the [API](../api.md#register-an-application-with-two-redirect-uris) takes several.

## Add the realm in Proxmox VE

In the web interface, open **Datacenter → Permissions → Realms**, choose **Add → OpenID Connect Server**, and fill in:

| Field | Value |
| --- | --- |
| Issuer URL | `https://auth.example.com` |
| Realm | `halo` |
| Client ID | Your client ID, `hl_…` |
| Client Key | Your client secret, `hls_…` |
| Autocreate Users | On |
| Username Claim | `email` |
| Scopes | `openid email profile groups` |
| Groups Claim | `groups` |
| Autocreate Groups | On |

Or create the realm from a shell on a node:

```bash
pveum realm add halo --type openid \
  --issuer-url https://auth.example.com \
  --client-id hl_… \
  --client-key hls_… \
  --username-claim email \
  --autocreate 1 \
  --scopes "openid email profile groups" \
  --groups-claim groups \
  --groups-autocreate 1
```

With `--username-claim email`, Proxmox VE usernames look like `sam@example.com@halo`. Halo's `preferred_username` is also the email address, so `username` gives the same result. Avoid `subject`: it produces usernames from Halo ids such as `usr_01J…@halo`.

## Map groups and grant permissions

Halo's `groups` claim lists the names of all of the person's groups. Proxmox VE adds the realm name to each one, so the Halo group `pve-admins` becomes the Proxmox VE group `pve-admins-halo`. With **Autocreate Groups** on, Proxmox VE creates missing groups at sign-in; with it off, it ignores groups that do not exist yet.

Proxmox VE skips groups whose names contain characters it does not allow in group names, so give the Halo groups you use in Proxmox VE names such as `pve-admins`.

Signing in does not grant any permissions by itself. After the first sign-in creates the groups, grant them roles under **Datacenter → Permissions**, for example the `Administrator` role on `/` for `pve-admins-halo`.

## Check it

Open the Proxmox VE web interface, choose the **halo** realm on the sign-in screen, and sign in to Halo. Proxmox VE opens with your new user. If Halo reports that the redirect URI is missing from the client configuration, register the exact address in your browser's address bar, port included. For other errors, see [troubleshooting](generic-oidc.md#troubleshooting).

# Nextcloud

Sign in to Nextcloud with Halo through Nextcloud's OpenID Connect user backend app, `user_oidc`, and create Nextcloud groups from Halo groups.

The examples use `https://auth.example.com` for Halo and `https://cloud.example.com` for Nextcloud.

## Register Nextcloud in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Nextcloud** template, then **Continue**.
4. Check the redirect URI and change the host to your Nextcloud address:

   ```text
   https://cloud.example.com/apps/user_oidc/code
   ```

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use Nextcloud.

## Configure Nextcloud

Run these commands on the Nextcloud host. They use `occ` from `/var/www/nextcloud` as the `www-data` user; adjust both for your installation.

1. Install the OpenID Connect user backend:

   ```bash
   sudo -u www-data php /var/www/nextcloud/occ app:install user_oidc
   ```

2. Add Halo as a provider, with your client ID and secret:

   ```bash
   sudo -u www-data php /var/www/nextcloud/occ user_oidc:provider Halo \
     --clientid="hl_…" \
     --clientsecret="hls_…" \
     --discoveryuri="https://auth.example.com/.well-known/openid-configuration" \
     --scope="openid email profile groups" \
     --mapping-groups="groups" \
     --group-provisioning=1
   ```

Nextcloud's sign-in page now offers **Halo**. `user_oidc` uses PKCE automatically when the provider supports it, as Halo does.

## Map groups

Halo's `groups` claim lists the names of all of the person's groups. With `--group-provisioning=1`, Nextcloud reads the claim named in `--mapping-groups` at every sign-in, creates any group that does not exist yet, and puts the person in it. The Nextcloud groups carry the Halo group names, so share folders with them or make one of them an administrator group in Nextcloud.

The `groups` scope in `--scope` is required: without it, Halo leaves the claim out.

## Send everyone to Halo

With one provider configured, you can skip Nextcloud's own sign-in form and send people straight to Halo:

```bash
sudo -u www-data php /var/www/nextcloud/occ config:app:set --type=string --value=0 user_oidc allow_multiple_user_backends
```

Keep a local Nextcloud administrator account with a strong password for emergencies before you do this.

## Check it

Open Nextcloud in a private window, choose **Halo**, and sign in. Nextcloud creates your account on the first sign-in, with your name and email address from Halo and your groups. For errors, see [troubleshooting](generic-oidc.md#troubleshooting).

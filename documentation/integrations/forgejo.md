# Forgejo

Add Halo to Forgejo as an OpenID Connect authentication source, and make members of a Halo group Forgejo administrators.

The examples use `https://auth.example.com` for Halo and `https://git.example.com` for Forgejo.

## Register Forgejo in Halo

1. In the console, open **Applications** and choose **Add application**.
2. Choose **OpenID Connect** and **Continue**.
3. Choose **Web application** and the **Forgejo** template, then **Continue**.
4. Check the redirect URI and change the host to your Forgejo address:

   ```text
   https://git.example.com/user/oauth2/halo/callback
   ```

   `halo` in the path is the name of the authentication source you create in Forgejo below. If you choose another name, use it here too; the name is case-sensitive.

5. Choose **Create application**, and copy the client ID and the client secret. Halo shows the secret only once.
6. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick the groups whose members may use Forgejo.

The application page in the console shows the command below with your client ID filled in.

## Configure Forgejo

Run this on the Forgejo host as the user that runs Forgejo, with the client secret in `HALO_CLIENT_SECRET` and your client ID after `--key`:

```bash
forgejo admin auth add-oauth \
  --name halo \
  --provider openidConnect \
  --key hl_… \
  --secret "$HALO_CLIENT_SECRET" \
  --auto-discover-url https://auth.example.com/.well-known/openid-configuration \
  --scopes openid --scopes profile --scopes email --scopes groups \
  --group-claim-name groups \
  --admin-group "Forgejo maintainers"
```

Forgejo's sign-in page now offers Halo. To check or change the source later, find it among the authentication sources in Forgejo's site administration.

## Map groups

Halo's `groups` claim lists the names of all of the person's groups, and `--group-claim-name groups` tells Forgejo to read it:

- `--admin-group "Forgejo maintainers"` makes members of the Halo group **Forgejo maintainers** Forgejo administrators.
- `--restricted-group` takes a group name in the same way and makes its members restricted users.
- `--group-team-map` takes a JSON mapping from group names to organization teams.

Use the group names exactly as they appear in Halo's console.

## New accounts

By default, Forgejo asks someone who signs in with Halo for the first time to sign in to an existing Forgejo account to link, or to register a new one. To create accounts automatically instead, set `ENABLE_AUTO_REGISTRATION = true` in the `[oauth2_client]` section of `app.ini`. The `USERNAME` setting in the same section decides where new usernames come from (`userid`, `nickname`, `email` or `preferred_username`). Halo does not send a `nickname` claim, and its `preferred_username` is the person's email address.

## Check it

Sign out of Forgejo, choose the Halo option on Forgejo's sign-in page, and sign in to Halo. Forgejo opens signed in, and the sign-in appears in Halo's console under **Sign-in logs**. For errors, see [troubleshooting](generic-oidc.md#troubleshooting).

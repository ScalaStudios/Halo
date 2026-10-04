# Infrastructure access

Halo can sign short-lived SSH certificates, so people reach servers with the same identity, groups and offboarding as every other application, and servers never hold a list of personal keys. People get certificates from the `halo` command-line tool after signing in with the device flow.

## How it works

1. Halo runs an SSH certificate authority. It creates an Ed25519 key pair the first time anything needs it and stores the private key encrypted with `HALO_SECRET_KEY`.
2. Your servers trust that authority for user certificates.
3. A security administrator maps groups to **principals**, the unix account names their members may log in as.
4. A person runs `halo login` once, then `halo ssh-cert` whenever they need a certificate. Halo signs their public key for every principal their groups map to, valid for a few hours.
5. `ssh` presents the certificate automatically, and `sshd` checks it against the authority and the account name.

## Set up a server

1. Download the authority's public key. The route needs no authentication:

   ```bash
   curl -fsS https://auth.example.com/api/v1/ssh/ca.pub | sudo tee /etc/ssh/halo_user_ca.pub
   ```

2. Add this line to `/etc/ssh/sshd_config`:

   ```text
   TrustedUserCAKeys /etc/ssh/halo_user_ca.pub
   ```

3. Create the unix accounts that your principal mappings name, such as `ops` or `deploy`, if they do not exist.
4. Check the configuration and reload sshd:

   ```bash
   sudo sshd -t && sudo systemctl reload sshd
   ```

Without an `AuthorizedPrincipalsFile`, sshd accepts a certificate for an account only when the account's name is one of the certificate's principals. Keep it that way unless you need a different mapping on a particular server.

The authority's key does not change, so you can bake the file into your server images. The console shows its fingerprint under **Infrastructure access**.

## Map groups to principals

1. In the console, open **Infrastructure access** and choose **Add principal mapping**.
2. Pick a group. Assigned and rule-based groups both work, and each group has at most one mapping.
3. Enter between 1 and 32 unix account names, such as `ops`. Names start with a lowercase letter or `_` and hold up to 32 lowercase letters, digits, `_`, `.` and `-`.
4. Choose **Save mapping**.

A person's certificate lists the principals of every mapped group they belong to. Someone in no mapped group gets `ERR_NO_PRINCIPALS` instead of a certificate.

## Set the certificate lifetime

Certificates last 8 hours by default. A security administrator sets the lifetime between 5 minutes and 24 hours under **Infrastructure access**. The lifetime is the main control: sshd cannot ask Halo whether a certificate is still wanted, so a certificate works until it expires.

## Use the halo command

The `halo` binary is both the server and the client. Build it from the repository with `go build ./cmd/halo` and put it on each person's path.

### Sign in

```bash
halo login --server https://auth.example.com
```

1. `halo` asks Halo for a device code and prints a link to Halo's `/device` page and a code such as `BCDF-GHJK`.
2. Open the link, sign in to Halo if you are not signed in, and check that the page shows the same code. The page also shows the address the request came from and the program that made it.
3. Choose **Approve sign-in**. Halo checks the [access policies](policies.md) against your browser session for the built-in **Halo CLI** application, and records the sign-in in the sign-in log.
4. `halo` receives its tokens and prints who you are signed in as.

The code expires after 10 minutes. `halo` stores the server address, an access token and a refresh token in `credentials.json` under your user configuration directory (`~/.config/halo/` on Linux), readable only by you. It refreshes the access token on its own when it is about to expire.

Every active person can use the Halo CLI application; group assignment does not apply to it. It cannot be deleted. Disabling it in the console under **Applications** revokes every CLI token and stops `halo login`.

### Get a certificate

```bash
halo ssh-cert
```

`halo ssh-cert` sends `~/.ssh/id_ed25519.pub` to Halo, writes the certificate to `~/.ssh/id_ed25519-cert.pub`, where `ssh` finds it next to the key, and prints the principals and when it expires:

```text
Wrote certificate 42 to /home/sam/.ssh/id_ed25519-cert.pub.
Log in as ops, deploy until 4 October 18:02 CEST.
```

Then connect as usual:

```bash
ssh ops@server.example.com
```

Options:

- `--key` picks another public key, such as `--key ~/.ssh/work_ed25519.pub`.
- `--out` writes the certificate somewhere else.

Halo accepts any OpenSSH public key except a certificate; RSA keys need at least 2048 bits. Your private key never leaves your computer.

### Other commands

| Command | What it does |
| --- | --- |
| `halo whoami` | Shows your name, email address, user id, groups and server. |
| `halo logout` | Revokes the refresh token at Halo and deletes the stored credentials. |

## The certificates Halo issues

| Field | Value |
| --- | --- |
| Type | User certificate |
| Key ID | `halo:<user id>:<serial>`, which sshd writes to its log on every login |
| Serial | A number that increases with every certificate |
| Principals | The account names mapped to the person's groups |
| Valid | From 5 minutes before issue, to allow for clock differences, until the lifetime ends |
| Extensions | `permit-pty`, `permit-port-forwarding`, `permit-agent-forwarding`, `permit-X11-forwarding`, `permit-user-rc` |

Every certificate writes an `ssh.certificate.issue` audit event. **Infrastructure access** lists the 200 most recent certificates with the person, principals, key fingerprint, IP address and validity.

## Take access away

- Removing someone from a mapped group, or removing the mapping, stops new certificates for those principals at once.
- Suspending the person stops `halo` working for them: Halo refuses their CLI tokens.
- Certificates already issued stay valid until they expire. To end one sooner, add the public key to an sshd `RevokedKeys` file on your servers.

Halo checks the access policies when someone approves `halo login`, not each time it issues a certificate.

## API

`GET /api/v1/ssh/ca.pub` returns the authority's public key. People request certificates with `POST /api/v1/me/ssh/certificates`, using a browser session or a Halo CLI access token. Administrators manage `/api/v1/ssh/authority`, `/api/v1/ssh/principal-mappings` and `/api/v1/ssh/certificates`; see the [API](api.md).

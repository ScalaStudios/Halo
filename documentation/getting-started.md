# Get started

Run Halo on your computer, create the first administrator with a passkey, and sign in to an example application through Halo. To run Halo for other people, follow [Self-hosting](self-hosting.md) instead.

## Requirements

- Go 1.27
- [Bun](https://bun.sh) 1.3, or Node.js 22 or later
- Docker
- OpenSSL, to generate the secret key
- A browser and device that support passkeys, such as a phone, a laptop with a fingerprint reader or screen lock, or a password manager that stores passkeys

## 1. Start PostgreSQL

From the repository root, start PostgreSQL 17 on port 5436:

```bash
docker compose -f compose.dev.yml up -d
```

## 2. Configure Halo

Copy the example environment file:

```bash
cp .env.example .env
```

Generate a secret key and paste it after `HALO_SECRET_KEY=` in `.env`:

```bash
openssl rand -base64 32
```

Load the variables into your shell. Do this in every terminal where you run a `halo` command:

```bash
set -a && . ./.env && set +a
```

`.env.example` sets `HALO_DEV=1`, which lets Halo run on plain http at `http://localhost:3200`. [Configuration](configuration.md) describes every variable.

## 3. Create the first administrator

```bash
go run ./cmd/halo bootstrap --email you@example.com --name "Your Name"
```

Halo applies its database migrations, creates you as a global administrator, and prints a setup link such as `http://localhost:3200/enroll?token=…`. The link works once and expires after 7 days.

## 4. Start Halo

Start the server in one terminal:

```bash
go run ./cmd/halo serve
```

Start the web interface in a second terminal:

```bash
cd web && bun install && bun run dev
```

## 5. Create your passkey

Open the setup link from step 3. Follow the prompts to create a passkey; your browser asks you to confirm with your fingerprint, face, screen lock or security key. Halo signs you in and opens your account at http://localhost:3200/account.

The console is at http://localhost:3200/admin.

## 6. Sign in to an application through Halo

The repository includes an 80-line Go web application that signs in with Halo. Connect it:

1. In the console, open **Groups**, choose **Create group**, name it `Example users`, keep **Assigned** membership, and choose **Create group**.
2. Open **Users**, open your own account, go to **Groups**, and add yourself to **Example users** with **Add to group**.
3. Open **Applications** and choose **Add application**. Choose **OpenID Connect**, then **Web application** and the **Custom** template. Name it `Example app`, enter the redirect URI `http://localhost:9000/callback`, and choose **Create application**.
4. Copy the client ID and the client secret. Halo shows the secret only now.
5. Choose **Open application**, go to **Users & groups**, choose **Assign group**, and pick **Example users**. Until a group is assigned, nobody can sign in to the application.
6. In a third terminal, start the example application with your client ID and secret:

   ```bash
   CLIENT_ID=hl_… CLIENT_SECRET=hls_… go run ./examples/go-web-client
   ```

7. Open http://localhost:9000 and choose **Sign in with Halo**. Because you are already signed in to Halo, it sends you straight back, and the example application shows the claims from your ID token, including `groups`.

The application, your sign-in to it, and every change you made appear in the console under **Applications**, **Sign-in logs** and **Audit log**.

## Explore with demo data

To look around a populated organization instead, load the Fernway Systems demo, with 68 people and their groups, applications, sessions and history, into an empty database:

```bash
docker compose -f compose.dev.yml down -v && docker compose -f compose.dev.yml up -d
go run ./cmd/halo seed-demo
```

The demo passkeys are placeholders, so sign in by creating a session for one of the demo people:

```bash
go run ./cmd/halo dev-session --email luna@example.com
```

In your browser's developer tools, add a cookie for `localhost` named `halo_session` with the printed value, then open http://localhost:3200/admin. Luna is a global administrator. `seed-demo` and `dev-session` only run with `HALO_DEV=1`.

## Next steps

- [Concepts](concepts.md) explains users, groups, roles, applications and sessions.
- [Integrations](README.md#integrations) connects Grafana, Forgejo, Nextcloud, Kubernetes and more.
- [Email](email.md) connects a mail server, so Halo sends invitations and magic links itself.
- [Access policies](policies.md) and [governance](governance.md) decide who gets in and for how long.
- [Self-hosting](self-hosting.md) puts Halo on a server with HTTPS.

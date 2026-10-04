# Contributing to Halo

Halo welcomes bug reports, fixes, documentation and features. This guide covers setting up a development environment, running the checks, the code style, and how to get a change merged.

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md). To report a security vulnerability, follow [SECURITY.md](SECURITY.md) instead of opening an issue.

## Before you start

Open an issue before you write code for anything large: a new feature, a new dependency, a database schema change, a change to the API, or a change to how people sign in. Describe the problem first and the solution second. Small, contained fixes such as a typo or a bug with an obvious cause can go straight to a pull request.

## Set up a development environment

You need Go 1.27, [Bun](https://bun.sh) 1.3 (or Node.js 22 or later), Docker and OpenSSL.

1. Start PostgreSQL 17. It listens on `127.0.0.1:5436` with user, password and database `halo`:

   ```bash
   docker compose -f compose.dev.yml up -d
   ```

2. Create your environment file and set `HALO_SECRET_KEY` in it to the output of `openssl rand -base64 32`:

   ```bash
   cp .env.example .env
   ```

3. Load the variables in every terminal where you run `halo` commands:

   ```bash
   set -a && . ./.env && set +a
   ```

4. Load data. Either load the demo organization, Fernway Systems, with 68 people, groups, applications and activity:

   ```bash
   go run ./cmd/halo seed-demo
   ```

   or create a single administrator and enroll a real passkey with the link it prints:

   ```bash
   go run ./cmd/halo bootstrap --email you@example.com --name "Your Name"
   ```

   `seed-demo` needs `HALO_DEV=1`, which `.env.example` sets, and an empty database.

5. Start the server. It applies migrations, then listens on `:8080`:

   ```bash
   go run ./cmd/halo serve
   ```

6. In a second terminal, start the web interface on http://localhost:3200:

   ```bash
   cd web && bun install && bun run dev
   ```

7. With the demo data, sign in by creating a session directly. The demo passkeys are placeholders, so this is the only way in:

   ```bash
   go run ./cmd/halo dev-session --email luna@example.com
   ```

   Set the printed `halo_session` cookie for `localhost` in your browser's developer tools. Luna is a global administrator. `dev-session` only runs with `HALO_DEV=1`.

Restart `halo serve` after you change Go code. The web interface reloads by itself.

To start over with an empty database:

```bash
docker compose -f compose.dev.yml down -v && docker compose -f compose.dev.yml up -d
```

## Run the checks

Run these before you open a pull request. CI runs the same four on every pull request and on every push to `main` (`.github/workflows/ci.yml`).

```bash
go vet ./...
go test ./...
cd web && npx tsc --noEmit
cd web && bun run build
```

`go test ./...` creates a throwaway database for each test on the development PostgreSQL and drops it afterwards. To use another server, set `HALO_TEST_DATABASE_URL`, for example `postgres://halo:halo@localhost:5436/halo`; the user needs permission to create databases. When PostgreSQL is unreachable, the database tests are skipped rather than failed, so check the output for `SKIP`.

The end-to-end tests use a virtual authenticator in Chromium to enroll passkeys and sign in through Halo to [`examples/go-web-client`](examples/go-web-client) over OpenID Connect and [`examples/go-saml-sp`](examples/go-saml-sp) over SAML. They also cover magic links, access requests, access policies, and every console page at desktop and phone widths:

```bash
cd web && bunx playwright install chromium
cd web && bun run e2e
```

They need both servers running with the demo data loaded (they sign in as `luna@example.com` through `dev-session` with the repository's `.env`), and ports 9000 and 9100 free, because they start the example applications there. Set `HALO_URL` to test an address other than http://localhost:3200.

## Code style

These rules apply everywhere:

- **No code comments.** Names carry the meaning. If a block needs a comment to be understood, rename or restructure it instead. `//go:embed` directives are not comments.
- Make the smallest correct change. Reuse an existing helper, component or pattern before you write a new one, and do not add abstractions for needs that do not exist yet.

### Go

- Use the standard library first: `net/http` with method patterns (`mux.Handle("GET /api/v1/things/{id}", httpx.Handle(fn))` and `r.PathValue("id")`), `log/slog`, `crypto/*`. Propose a new dependency in an issue before you add it.
- Format with `gofmt`.
- Keep all SQL in `internal/store`. Handlers call store methods and never query the database themselves.
- Put schema changes in a new numbered file in `migrations/`. Migrations run in filename order and only forward, so never edit one that has shipped.
- Return errors with a stable code and an actionable message through `httpx.Fail(status, "ERR_…", message)`, `httpx.Invalid(message)` or `httpx.NotFound(what)`. The message says what happened, why, and what to do next, for example: "This is your last way to sign in, so it can't be removed. Add another passkey or authenticator app first."
- Write JSON in camelCase. Create ids with `internal/id`: a prefix such as `usr_`, `grp_` or `app_` followed by a 26-character ULID.
- Store secrets hashed with `secret.Hash` or sealed with `Store.Sealer`, and compare them with `secret.Equal`, which runs in constant time.
- Check roles in admin endpoints with `httpx.RequireRole`, and record every admin change with `store.RecordAudit` in the same transaction as the change.
- Write tests with the standard `testing` package against a real database from `internal/testdb`.

### Web

- The interface is Next.js (App Router) with React 19, Tailwind CSS 4 and strict TypeScript.
- Use brand tokens only. `web/src/app/globals.css` defines the colours, type scale and radii, and Tailwind's default palette, type scale and radii are removed, so classes such as `text-gray-500` or `rounded-2xl` produce nothing. Use tokens such as `bg-surface`, `text-fg-3`, `text-body-sm` and `rounded-md`.
- Reuse the primitives in `web/src/components/ui/`, such as `Button`, `Card`, `DataTable`, `Dialog` and `Field`. Console mutations go through `web/src/components/console/use-mutation.ts`.
- Space on the 4-point scale: Tailwind steps 1, 2, 3, 4, 6, 8, 12, 16, 24 and 32.
- Use Lucide icons at 16, 20 or 24 pixels with a stroke width of 1.75. No emoji.
- Write copy in sentence case. Be specific and factual, leave out exclamation marks, and never write "simply", "just" or "easily". Buttons are a verb and an object, such as "Create group". The product is called Halo.
- Every control works. Async buttons show a loading state and empty lists show an empty state.

## Propose a change

1. Open or find the issue for anything large and agree on the approach there.
2. Branch from `main` and keep each pull request to one change.
3. Add or update tests for any change in behaviour.
4. Update [`documentation/`](documentation) when something users or operators see changes, and add a line under **Unreleased** in [`CHANGELOG.md`](CHANGELOG.md).
5. Run the checks above.
6. In the pull request, explain what changed, why, and how you verified it. Include screenshots for interface changes.

## Commit messages

Write the whole message in lowercase, starting with `feat:`, `fix:` or `chore:`, and describe what the change does. A type or function name keeps its case.

```text
feat: rotate client secrets with a 24-hour grace period
fix: refuse refresh tokens once the user is suspended
chore: update zitadel/oidc to v3.51.11
fix: PeekEnrollmentToken ignores links that were already used
```

## License

Halo is licensed under the [Apache License 2.0](LICENSE). Contributions are accepted under the same license: inbound equals outbound. Under section 5 of the license, any contribution you intentionally submit for inclusion in Halo is licensed under the Apache License 2.0, without additional terms or conditions. Submit only work that you have the right to license this way.

### The React Bits boundary

Two files in `web/src/components/reactbits/`, `CodeSlots.tsx` and `HoldButton.tsx`, come from [React Bits](https://github.com/DavidHDev/react-bits) and are **not** covered by the Apache License 2.0. They are distributed under MIT plus the Commons Clause License Condition v1.0, which is not an OSI-approved license; [NOTICE](NOTICE) lists the details. Keep the boundary intact:

- Use the two components only through their wrappers, `web/src/components/ui/code-input.tsx` and `web/src/components/ui/hold-to-confirm.tsx`, so a redistributor who needs a fully OSI-licensed tree can replace them.
- Do not copy more React Bits code into Halo without discussing it in an issue first.
- Record any other third-party code you bring in, with its license, in [NOTICE](NOTICE).

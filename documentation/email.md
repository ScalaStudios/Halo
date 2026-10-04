# Email

Halo sends email for invitations, authentication resets, magic sign-in links and access requests. This page explains how to connect Halo to a mail server, how the outbox delivers and retries messages, and how magic links work.

Halo runs without email. Without it, the console shows invite and reset links to the administrator who creates them, and you pass them on yourself.

## What Halo sends

| Message | Sent to | When |
| --- | --- | --- |
| Set up your Halo account | The invited person | An administrator invites someone. The link works once and expires after the setup link lifetime, 7 days by default. |
| Reset how you sign in to Halo | The person | An administrator resets their authentication. |
| Your Halo sign-in link | The person | Someone asks for a magic link on the sign-in page. |
| *Name* requests *package* | Each approver of the access package | Someone requests an access package. |
| Your request for *package* was approved or denied | The requester | An approver decides the request. |

Every message is plain text and ends with "Halo ·" and your organization name.

## Connect a mail server

1. Get the SMTP host, port, user name and password from your mail provider. Any provider that accepts SMTP submission works.
2. Set the variables for the Go server:

   ```bash
   HALO_SMTP_HOST=smtp.example.com
   HALO_SMTP_PORT=587
   HALO_SMTP_USERNAME=halo@example.com
   HALO_SMTP_PASSWORD=…
   HALO_SMTP_FROM="Halo <halo@example.com>"
   ```

3. Restart `halo serve`. Halo refuses to start when `HALO_SMTP_HOST` is set without `HALO_SMTP_FROM`.
4. Invite yourself with a second address, or open the sign-in page and choose **Email me a sign-in link**, and check that the message arrives.

How Halo connects:

- On port `465`, Halo uses TLS from the start of the connection.
- On any other port, Halo requires STARTTLS. It sends without TLS only when the host is `localhost` or a loopback address, such as a local relay.
- With `HALO_SMTP_USERNAME` set, Halo authenticates with PLAIN. Without it, Halo sends without authenticating.
- `HALO_SMTP_PORT` defaults to `587`.

[Configuration](configuration.md#smtp) lists the variables with their defaults.

## The outbox

Halo writes every message to an outbox table in PostgreSQL before it tries to send it, so a mail server outage does not lose messages.

- The message body is encrypted with `HALO_SECRET_KEY`. The recipient and subject are stored as they are.
- With SMTP configured, Halo sends the message right away. If that fails, a background job retries it every minute that it is due, with growing gaps of about 1, 2, 4 and 8 minutes, up to 5 attempts in total.
- Halo retries a message only within 1 day of creating it.
- Without SMTP, Halo queues the message and logs `email queued but not sent: SMTP is not configured`. When you configure SMTP and restart Halo, it sends the queued messages that are less than 1 day old.
- Halo deletes sent messages, and messages that used up their attempts, 30 days after creating them.

The API reports whether an invitation or reset went out: `POST /api/v1/users` and `POST /api/v1/users/{id}/reset-authentication` answer with `"emailed": true` when SMTP is configured and the first attempt succeeded. When it is `false`, pass on the `enrollUrl` from the same response yourself.

### Read the outbox in development

With `HALO_DEV=1`, a global administrator can read the 50 most recent messages, decrypted, at `GET /api/v1/dev/outbox`. Use it to open magic links and invitations on a computer without a mail server:

```bash
curl http://localhost:3200/api/v1/dev/outbox -H "Cookie: halo_session=$HALO_SESSION"
```

The route does not exist when `HALO_DEV` is off.

## Magic links

A magic link signs a person in with one click on a link Halo emails to them.

1. On the sign-in page, the person enters their email address and chooses **Email me a sign-in link**.
2. Halo answers the same way whether or not the address belongs to an account, so the page does not reveal who has one.
3. If the address belongs to an **active** account, Halo emails a link to `/sign-in/magic?token=…`. Invited people who have not signed in yet, and suspended accounts, get no email.
4. The person opens the link. Halo checks the token, runs the same access policy checks as every other sign-in, and starts a session.

Limits:

- A link works once and expires after 10 minutes.
- Halo sends at most 5 links per person in 15 minutes and ignores further requests without telling the requester.
- When the sign-in came from an application, open the link in the same browser you started in. Halo only redeems an application's sign-in request in the browser session that completed it.

Turn magic links off for everyone in the console under **Methods**. The sign-in page then stops offering them, and an access policy can also refuse them for some people or applications; see [Policies](policies.md#allowed-sign-in-methods).

Magic links prove that someone can read the person's mailbox, nothing more. They are not phishing-resistant, so a policy that requires a passkey or security key refuses them.

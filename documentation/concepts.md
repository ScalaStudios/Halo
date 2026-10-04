# Concepts

This page explains the objects you work with in Halo and how they relate: people sign in with **sign-in methods**, which creates a **session**. People belong to **groups**, groups are assigned to **applications**, and **roles** decide who may administer Halo. Halo records every sign-in attempt in the **sign-in log** and every change in the **audit log**.

Other guides cover the areas built on top of these: [access policies](policies.md), [governance](governance.md), [federation](federation.md), [provisioning and service accounts](provisioning.md), [webhooks](webhooks.md), [API resources](api-resources.md) and [infrastructure access](infrastructure-access.md).

## Users

A user is a person in Halo's directory. Each user has an email address, a name, optional title, department, location and manager, a status, and a source: `Halo` for people created in Halo, `SCIM · ` and the connector's name for people provisioned over SCIM, or the identity provider's name for people created on their first federated sign-in. Email addresses are unique, ignoring case.

Automation uses [service accounts](provisioning.md#service-accounts) instead of users. They hold roles and API keys, and do not appear in the user list.

| Status | Meaning |
| --- | --- |
| Invited | The account exists but the person has not signed in yet. Their first successful sign-in makes them active. |
| Active | The person can sign in. |
| Suspended | The person cannot sign in. Suspending an account ends all of its sessions and revokes the tokens issued during them. |
| Deprovisioned | The person cannot sign in. A SCIM connector deleted the account; Halo keeps it with its history. |

### Invite people

In the console, open **Users** and choose **Invite user**. Enter the person's email address and name, and optionally pick assigned groups. Halo creates the account and shows a setup link. The link works once and expires after 7 days, or the setup link lifetime set under **Security defaults**. When [email is configured](email.md), Halo also emails the link to the person; otherwise, send it to them yourself. Opening it lets them create their first passkey or security key, which also signs them in.

Creating a new link for a person, such as by resetting their authentication, invalidates any earlier link that was not used.

**Reset authentication**, on a user's page, removes all of the person's passkeys, security keys, authenticator apps and recovery codes, ends their sessions, and creates a new setup link. Use it when someone loses their devices.

Edit a person's name, email address, title, department, location and manager on their user page. Helpdesk administrators can change title, department and location only. A directory that [provisions people over SCIM](provisioning.md#inbound-scim) keeps these fields up to date for you.

### Authentication strength

The console labels each person by the strongest method they have:

| Strength | When |
| --- | --- |
| Phishing-resistant | The person has a passkey or a security key. |
| Multi-factor | The person has an authenticator app but no passkey or security key. |
| Single factor | Anything else. |

The console's overview lists administrators who have no phishing-resistant method.

## Groups

Access is granted to groups, not to individuals. An application is assigned to one or more groups, and a person can sign in to it when they belong to at least one of them.

Group names are unique, ignoring case. Halo has two kinds of group:

- **Assigned groups** hold the people an administrator adds. Add someone from their user page (**Groups → Add to group**) or pick groups when you invite them.
- **Rule-based groups**, which the API calls dynamic groups, contain everyone whose profile matches a rule. Halo evaluates the rule every time it reads the directory, so membership changes as soon as a profile does. You cannot add or remove members of a rule-based group by hand.

### Rule syntax

A rule compares one user attribute with one value:

```text
user.<attribute> == "<value>"
```

| Attribute | Holds |
| --- | --- |
| `status` | `active`, `invited`, `suspended` or `deprovisioned` |
| `department` | The person's department |
| `location` | The person's location |
| `title` | The person's job title |
| `source` | Where the account came from, such as `Halo` |

Examples:

```text
user.department == "Engineering"
user.location == "Amsterdam"
user.status == "active"
```

The comparison ignores case, so `user.department == "engineering"` also matches `Engineering`. A rule holds exactly one comparison: Halo has no `and`, `or`, `!=` or wildcards yet, and it rejects a rule it cannot parse when you save the group. When you edit a rule in the console, preview it to see who it would include before you save it.

You can rename a group, change its description, and change a rule-based group's rule. A group cannot switch between assigned and rule-based. Halo refuses to delete a group while an application, an access package, an access policy, an identity provider that adds new accounts to it, an open access review or a lifecycle rule still uses it, and names each one.

### Groups in tokens

When an application requests the `groups` scope, Halo adds a `groups` claim to the ID token, the userinfo response and token introspection. The claim lists the **names** of every group the person belongs to, assigned and rule-based alike, for example `["Engineering", "Grafana editors"]`. Applications match on these names, so renaming a group in a mapping means changing the application's configuration too.

## Roles

Roles decide who may administer Halo. People without a role can use the account portal but not the console. Every role can read every page of the console, except the data export.

| Role | What it allows besides reading |
| --- | --- |
| Global administrator | Everything. Only a global administrator can assign roles, change accounts that hold an administrator role, change the organization name, contact email and sign-in message, upload the logo, add and verify domains, download the data export, and decide any access request. |
| Security administrator | Suspend people and end their sessions. Manage access policies, network zones, allowed sign-in methods, device trust and risk events; identity providers; service accounts and API keys; webhooks; the SSH certificate lifetime and principal mappings; and the security defaults: session lifetime, lockout, setup link lifetime and token lifetimes. |
| User administrator | Invite, edit, suspend and restore people, end their sessions, reset their authentication and remove their sign-in methods. Create, edit and delete groups and manage their members. Manage access packages, revoke access grants, start and complete access reviews, and manage lifecycle rules. |
| Helpdesk administrator | Change people's title, department and location, end their sessions, reset their authentication, and remove their sign-in methods. |
| Application administrator | Register, change, enable, disable and delete applications, rotate and revoke client secrets, assign groups to applications, set up outbound provisioning, and manage API resources and their grants. |
| Auditor | Nothing: read only. |

Security and user administrators can also end individual sessions from the **Sessions** page.

Some permissions do not need a role at all:

- People named as **approvers** of an access package can approve and deny requests for it, and people named as **reviewers** of an access review can record its decisions and complete it.
- Everyone can request access packages, and get SSH certificates for the principals their groups map to.

Halo also enforces these safeguards:

- Only a global administrator can edit, suspend, restore, reset or remove methods from an account that holds any administrator role.
- Nobody can suspend their own account, and the last active global administrator cannot be suspended, by hand or by a lifecycle rule.
- A global administrator cannot remove their own global administrator role.
- An administrator can give a service account only roles they hold themselves.

A global administrator assigns roles in the console under **Roles**, or through the API with `PUT /api/v1/users/{id}/roles`; see [API](api.md#assign-roles). The keys are `global_admin`, `security_admin`, `user_admin`, `helpdesk_admin`, `app_admin` and `auditor`.

## Applications

An application is anything people sign in to through Halo, or a service that calls an API with its own credentials. Register one in the console under **Applications → Add application**.

| Protocol and type | Use it for | Credentials |
| --- | --- | --- |
| OpenID Connect, web application | Server-side apps such as Grafana, Forgejo and Nextcloud | Client secret |
| OpenID Connect, single-page app | Apps that run entirely in the browser | None. Must use PKCE. |
| OpenID Connect, native or CLI | Desktop, mobile and command-line tools such as kubectl | None. Must use PKCE. |
| OAuth 2.0 service | Machine-to-machine access. Nobody signs in. | Client secret, client credentials grant |
| SAML 2.0 | Applications that do not support OpenID Connect | None. Halo signs the assertion with its own key. |

Prefer OpenID Connect when an application supports both. [Connect an application with SAML 2.0](integrations/generic-saml.md) covers SAML; the rest of this section describes OpenID Connect and OAuth applications.

Each OpenID Connect and OAuth application gets a client ID that starts with `hl_`. Web applications and services also get a client secret that starts with `hls_`. Halo shows the secret once, when you create the application or rotate the secret, and stores only a hash of it. A secret is valid for 365 days. Rotating creates a new secret and makes every older one stop working 24 hours later, which gives you time to update the application. Revoking a secret stops it immediately.

Only members of the application's assigned groups can sign in to it. Nobody can sign in until you assign at least one group: open the application, go to **Users & groups**, and choose **Assign group**.

**Disabling** an application stops all sign-ins to it and revokes every token issued to it. **Deleting** an application removes it with its credentials and group assignments, and cannot be undone.

Halo creates one application itself: the **Halo CLI** (client ID `halo-cli`), which the `halo login` command uses. Every active person can use it without a group assignment. It cannot be deleted; disable it to stop command-line sign-in. See [Infrastructure access](infrastructure-access.md).

### Scopes and claims

An application created in the console may request the scopes `openid`, `profile`, `email` and `groups`, plus `offline_access`, which adds a refresh token to the token response, and the scopes of any [API resource](api-resources.md) it was granted. Halo drops any other scope from the request.

| Claim | Scope | Value |
| --- | --- | --- |
| `sub` | `openid` | The person's Halo id, such as `usr_01J…`. It never changes. |
| `email` | `email` | The person's email address. |
| `email_verified` | `email` | `true` once the person redeemed a magic link, finished setup from a link Halo emailed, or signed in through an identity provider that verified the same address. Changing the address sets it back to `false`. |
| `name` | `profile` | The person's name. |
| `preferred_username` | `profile` | The person's email address. |
| `groups` | `groups` | The names of every group the person belongs to. |
| `amr` | `openid` | How the person signed in: `["hwk", "mfa"]` for a passkey or security key, `["fed"]` for an identity provider, `["otp"]` for an authenticator app, recovery code or magic link. |

### Token lifetimes

New applications start with the token lifetimes set under **Security defaults**:

| Token | Default | Range for one application |
| --- | --- | --- |
| Access token | 15 minutes | 1 minute to 24 hours |
| ID token | 15 minutes | 1 minute to 24 hours |
| Refresh token | 8 hours, replaced by a new one every time it is used | Up to 90 days, or 0 to issue none |

Change an application's lifetimes, refresh token rotation, name, description, homepage, owner, redirect URIs, post-logout URIs and scopes on its page. Changing the defaults does not change existing applications. An access token for an [API resource](api-resources.md) lasts as long as that API's lifetime instead.

## Sessions

Signing in to Halo creates a session that lasts 12 hours by default; a security administrator sets the session lifetime between 1 and 72 hours under **Security defaults**. The session is what makes single sign-on work: while it lasts, opening another application signs you in without asking again, unless the application asks Halo to re-authenticate with `prompt=login` or with a `max_age` shorter than the session's age.

Each session records the device, browser, IP address, the method used to sign in, and when it was last active. People see and end their own sessions in the account portal under **Sessions**. Administrators see every session in the console under **Sessions** and on each user's page. Ending a session also revokes the access and refresh tokens that applications received during it.

## Sign-in methods

Halo has no passwords. People sign in with these methods:

| Method | How it works |
| --- | --- |
| Passkey | A credential stored on a phone, computer or password manager, unlocked with a fingerprint, face or screen lock. People choose **Continue with passkey** without typing their email address. |
| Security key | A hardware key over USB, NFC or Bluetooth. |
| Authenticator app | A 6-digit code that changes every 30 seconds, from an app such as 1Password or Google Authenticator. People enter their email address and the current code. |
| Recovery codes | 10 single-use codes such as `abcd-efgh`, for when no other method is at hand. Generating new codes replaces the old ones. |
| Magic link | A single-use sign-in link that Halo emails to the person and that expires after 10 minutes. Works only when [email is configured](email.md#magic-links). |
| Identity provider | Signing in with a Google, Microsoft Entra ID, GitHub or OpenID Connect account linked to the person. See [Federation](federation.md). |

The setup link from an invitation or reset always enrolls a passkey or security key. People add authenticator apps, more passkeys and recovery codes later in the account portal under **Security**. The account portal does not let people remove their own last sign-in method; recovery codes do not count as one.

Security administrators turn methods on and off for the whole organization under **Methods**, and [access policies](policies.md) can require a passkey or security key for some people or applications.

## Sign-in log and audit log

The **sign-in log** (console: **Sign-in logs**) records every attempt to sign in, successful or not: who, which application (or Halo itself), the method, the IP address, the browser and operating system, and for failures the reason, such as "The passkey signature could not be verified." Each event also carries a [risk level](policies.md#sign-in-risk): `none`, `low`, `medium` or `high`.

The **audit log** (console: **Audit log**) records every change an administrator or service account makes, every change people make to their own sign-in methods and access requests, and the changes Halo makes itself, such as lifecycle rules and expiring access: who made it, the action (for example `user.invite` or `application.secret.rotate`), a summary, the target and the IP address. Halo writes each audit event in the same database transaction as the change it describes, so a change cannot happen without its audit event.

[Webhooks](webhooks.md) send both logs to your own systems as events happen.

Halo keeps sign-in and audit events indefinitely; nothing deletes them automatically.

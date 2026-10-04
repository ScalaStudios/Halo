# Provisioning and automation

This page covers the non-human side of Halo:

- **Service accounts** and their **API keys**, for scripts and tools that call Halo.
- **Inbound SCIM**, where an HR system or another identity provider creates, updates and removes people and groups in Halo.
- **Outbound SCIM**, where Halo creates, updates and deactivates accounts in your applications.

## Service accounts

A service account is an identity for automation, such as Terraform, a CI pipeline or an HR connector. It holds administrator roles like a person does, but it never signs in through the sign-in page: it authenticates with API keys. Service accounts do not appear in the user list, and Halo gives each one an address of the form `terraform@service.halo.invalid`.

Security administrators manage service accounts; every administrator can read them.

### Create a service account

1. In the console, open **Service accounts** and choose **Create service account**.
2. Name it after what it automates, such as `Terraform`, and add a description.
3. Pick an owner: the person responsible for it.
4. Pick its roles. Requests made with its keys can do exactly what these roles allow; [Concepts](concepts.md#roles) lists them.

Two rules keep service accounts from becoming a way around roles:

- You can give a service account only roles you hold yourself. Anyone may give the auditor role. A global administrator can give every role.
- To change a service account's name, owner or roles, or to create keys for it, you must hold every role it has. Any security administrator can disable one.

Disabling a service account stops all of its keys at once. Enabling it again makes them work again.

### Create an API key

1. Open the service account and choose **Create API key**.
2. Enter a label that says where the key is used, such as `Production CI`.
3. Choose its scopes:

   | Scope | Allows |
   | --- | --- |
   | `api` | The management API under `/api/v1`, within the service account's roles. Personal routes under `/api/v1/me` and the sign-in routes under `/api/v1/auth` ignore keys. |
   | `scim` | The SCIM server under `/scim/v2`. The service account needs the user administrator role. |

4. Optionally set an expiry date.
5. Copy the key. It starts with `hlk_` and Halo shows it only now.

Halo stores only a SHA-256 hash of the key and shows its first 12 characters, such as `hlk_x7Qe2mVr`, to tell keys apart. A key stops working when it is revoked, when it expires, or when its service account is disabled. The key list shows when each key was last used, to the minute.

Send the key as a bearer token:

```bash
curl https://auth.example.com/api/v1/users -H "Authorization: Bearer hlk_…"
```

Every change made with a key is recorded in the audit log with the service account as the actor.

## Inbound SCIM

Halo is a SCIM 2.0 server. A directory such as Okta or Microsoft Entra ID can keep Halo's people and groups in step with it.

### Connection details

| Setting | Value |
| --- | --- |
| Base URL | `https://auth.example.com/scim/v2` (your `HALO_PUBLIC_URL` followed by `/scim/v2`) |
| Authentication | Bearer token: an API key with the `scim` scope |
| Service account roles | User administrator. To change or remove people who hold an administrator role in Halo, the service account needs the global administrator role. |

The console's **Provisioning** page shows the base URL and lists the service accounts that have an active SCIM key.

### Set up a connector

1. Create a service account named after the source, such as `Okta`, with the **User administrator** role.
2. Create an API key for it with the `scim` scope.
3. Enter the base URL and the key in the source system's provisioning settings, as described below for Okta and Entra ID.
4. Push a test user and check that they appear in Halo under **Users** with the source `SCIM · Okta`.

### What Halo supports

- `GET /ServiceProviderConfig`, `/ResourceTypes` and `/Schemas` for discovery.
- `/Users` and `/Groups` with `GET`, `POST`, `PUT`, `PATCH` and `DELETE`.
- Filters with a single `eq` condition: `userName`, `emails.value` or `externalId` for users, and `displayName` or `externalId` for groups.
- Paging with `startIndex` and `count`, at most 200 results per page. `excludedAttributes=members` leaves group members out.
- `PATCH` operations `add`, `replace` and `remove`, with or without a path. `active` may be a boolean or the strings `"True"` and `"False"`.
- No bulk operations, sorting, ETags or password changes.

User attributes:

| SCIM attribute | Halo field |
| --- | --- |
| `userName` | Email address. It must be an email address; when it is not, Halo uses the primary entry of `emails`. |
| `displayName`, `name.formatted`, or `name.givenName` and `name.familyName` | Name |
| `title` | Title |
| enterprise `department` | Department |
| enterprise `manager.value` | Manager, as the Halo id of another person |
| `externalId` | Stored for matching |
| `active` | `false` suspends the person and ends their sessions; `true` restores them |

What each operation does in Halo:

- **Create** adds the person with the status `invited`, or `suspended` when `active` is false. Halo sends no email. The person signs in through an [identity provider](federation.md) that confirms the same email address, or an administrator sends them a setup link with **Reset authentication** on their user page.
- **Delete** sets the person's status to `deprovisioned` and ends their sessions. Halo keeps the account and its history; deprovisioned people cannot sign in.
- **Groups** created over SCIM are assigned groups. SCIM never sees rule-based groups. Deleting a group over SCIM follows the same rule as the console: while an application, access package, policy, identity provider, open access review or lifecycle rule uses the group, Halo answers `409` with `scimType` `mutability` and a `detail` that names each one.

Every SCIM change writes an audit event such as `scim.user.create` or `scim.user.suspend`, and [lifecycle rules](governance.md#lifecycle-rules) react to the joiners, movers and leavers it creates.

Errors use the SCIM error format, for example:

```json
{"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"], "status": "409", "scimType": "uniqueness", "detail": "A user with userName sam@example.com or the same externalId already exists in Halo. …"}
```

### Okta

1. In Okta, open the app integration you use for Halo, or create one, and turn on SCIM provisioning in its general settings.
2. On the **Provisioning** tab, set:
   - **SCIM connector base URL**: `https://auth.example.com/scim/v2`
   - **Unique identifier field for users**: `userName`
   - **Supported provisioning actions**: push new users, push profile updates and push groups
   - **Authentication Mode**: HTTP Header, with the `hlk_` key as the bearer token
3. Test the connector configuration and save.
4. Under **To App**, turn on creating users, updating user attributes and deactivating users.
5. Assign people and push groups to the app.

Okta deactivates people by setting `active` to false, which suspends them in Halo.

### Microsoft Entra ID

1. In the Entra admin center, open the enterprise application for Halo and go to **Provisioning**.
2. Set the provisioning mode to **Automatic**.
3. Under **Admin Credentials**, enter the base URL as **Tenant URL** and the `hlk_` key as **Secret Token**, then choose **Test Connection**.
4. Check the attribute mappings: `userPrincipalName` or `mail` must map to `userName` with an email address, because Halo identifies people by email.
5. Assign users and groups to the application and start provisioning.

Entra ID sends `active` as the string `"False"` when it disables someone; Halo accepts it and suspends the person.

## Outbound SCIM

Halo can create and update accounts in an application that has a SCIM 2.0 endpoint, so people have an account before their first sign-in and lose it when they lose access.

### Set it up

1. In the application, turn on SCIM provisioning and copy its SCIM base URL and bearer token.
2. In Halo, open **Provisioning**, find the application under outbound provisioning and choose **Set up**. Only OpenID Connect and SAML applications that people sign in to are listed.
3. Enter the **SCIM base URL** and the **Bearer token**. The URL must use https, except `http://localhost` and `http://127.0.0.1`.
4. Turn on **Push changes every 2 minutes** and save.
5. Choose **Test**. Halo reads the application's `/ServiceProviderConfig` with the token.
6. Choose **Sync now** to push everyone right away.

Halo encrypts the token with `HALO_SECRET_KEY`. If that key changes, the token becomes unreadable and Halo asks you to save it again.

### What Halo pushes

Every 2 minutes, and whenever you choose **Sync now**, Halo compares the people assigned to the application with what it pushed before:

- Each **active** person who is assigned to the application through a group is created with `POST /Users`, or updated with `PUT /Users/{id}` when their name, email, title or department changed. Halo sends the email address as `userName` and as the primary work email, the name, the title, the enterprise department, `active: true`, and the Halo user id as `externalId`.
- When a create answers `409 Conflict`, Halo finds the existing account with `filter=userName eq "…"` and updates it.
- Each person who was pushed before and is no longer assigned or no longer active is deactivated with a `PATCH` that sets `active` to false. Halo never deletes accounts in the application.

**Sync now** runs even while pushing is paused. A connection failure stops the run; an error answer for one person is recorded and the run continues. The **Provisioning** page shows each application's state, the time of the last run, the counts of created, updated and deactivated accounts, and the first error with the number of others.

Groups are not pushed yet.

## API

Service accounts and keys are at `/api/v1/service-accounts` and `/api/v1/api-keys`, and outbound provisioning at `/api/v1/provisioning/applications`; see the [API](api.md).

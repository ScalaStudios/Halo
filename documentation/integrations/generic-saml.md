# Connect an application with SAML 2.0

Halo is a SAML 2.0 identity provider. Use SAML for applications that don't support OpenID Connect; when an application supports both, prefer OpenID Connect.

## Halo's identity provider values

Replace `https://auth.example.com` with your `HALO_PUBLIC_URL`. Every SAML application uses the same values, and the application's **Overview** tab in the console shows them with copy buttons.

| Value | |
| --- | --- |
| Metadata URL | `https://auth.example.com/saml/metadata` |
| IdP entity ID (issuer) | `https://auth.example.com/saml/metadata`, the same as the metadata URL |
| SSO URL | `https://auth.example.com/saml/sso`, for the HTTP-Redirect and HTTP-POST bindings |
| Signing certificate | `https://auth.example.com/saml/certificate`, a PEM file |

Most applications import the metadata URL, or a file downloaded from it, and need nothing else.

Halo creates its signing key the first time anything asks for it: an RSA-2048 key with a self-signed certificate that is valid for 10 years and names your Halo host. The private key is stored encrypted with `HALO_SECRET_KEY`. The **Overview** tab shows the certificate's SHA-256 fingerprint and expiry date. Signatures use RSA-SHA256.

## Register the application

1. In the console, open **Applications**, choose **Add application**, then **SAML 2.0**.
2. Pick a template, or **Custom**, and choose how to add the service provider:
   - **Enter values**: the application's **entity ID** (also called issuer or audience) and its **ACS URL**, where Halo posts the SAML response. ACS URLs must use `https://`; plain `http://` is only allowed for `localhost` and `127.0.0.1`.
   - **Paste metadata**: the application's service provider metadata XML. Halo reads the entity ID and every ACS URL that uses the HTTP-POST binding.
3. Choose the **NameID format**, the identifier Halo sends for each person:

   | Format | Halo sends |
   | --- | --- |
   | Email address (default) | The person's email address, as `urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress` |
   | Persistent | The person's Halo user ID, which never changes, as `urn:oasis:names:tc:SAML:2.0:nameid-format:persistent` |
   | Unspecified | The person's email address, as `urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified` |

4. Open the new application's **Users & groups** tab and assign the groups whose members may sign in.

Each entity ID can belong to one application only. SAML applications have no client secret. You can change the entity ID, ACS URLs (up to 10), NameID format and signature on the **Authentication** tab later.

## Attributes

Halo sends these attributes in every assertion. Rename them on the **Attributes** tab to the names the application expects, or clear a name to stop sending that value.

| Value | Default attribute name | Source |
| --- | --- | --- |
| Email address | `email` | The person's email address |
| Full name | `name` | The person's name |
| Given name | `given_name` | The first word of the person's name |
| Family name | `family_name` | The rest of the person's name |
| Groups | `groups` | One value for each group the person belongs to |

Empty values, such as the family name of someone with a one-word name, are left out. Attribute names that contain a colon, such as `urn:oid:0.9.2342.19200300.100.1.3`, are sent with the `uri` name format; all others use `basic`.

## Signatures

By default Halo signs the assertion. Choose **Sign the whole response** on the **Authentication** tab when the application expects a signed response; Halo then signs the response and keeps the assertion inside it signed. Halo does not encrypt assertions.

## How sign-in works

**Started by the application.** The application sends an authentication request to the SSO URL. Halo accepts it only from a registered entity ID and only posts the response to one of that application's ACS URLs. If the person isn't signed in to Halo, Halo keeps the request for 15 minutes, sends them to the sign-in page, and continues automatically after they sign in. Each kept request works once.

**Started from Halo.** `https://auth.example.com/saml/launch/<application ID>` signs the person in to the application without a request from it, using the application's first ACS URL. People who aren't signed in go to the sign-in page first.

Before posting a response, Halo checks that the application is enabled, that the person is assigned to it through a group, and that the access policies allow the sign-in. A policy that requires a phishing-resistant method turns away a session that was created with an authenticator app, an email link or a recovery code. Every attempt, allowed or refused, appears in **Sign-ins** with the application and the reason.

## Management API

Create a SAML application:

```http
POST /api/v1/applications
Content-Type: application/json

{
  "name": "Team wiki",
  "protocol": "saml",
  "saml": {
    "entityId": "https://wiki.example.com/saml/metadata",
    "acsUrls": ["https://wiki.example.com/saml/acs"],
    "nameIdFormat": "email"
  }
}
```

Send `"metadataXml"` instead of `entityId` and `acsUrls` to have Halo read them from the service provider's metadata. `GET /api/v1/applications/{id}` returns the settings under `saml`, together with Halo's values under `saml.idp`: `entityId`, `metadataUrl`, `ssoUrl`, `certificateUrl`, `certificateFingerprint` and `certificateExpiresAt`.

Change settings with `PUT /api/v1/applications/{id}/saml`. Send only the fields you change: `entityId`, `acsUrls`, `nameIdFormat`, `signResponse` (`true` signs the whole response), `metadataXml`, or `attributes` with all five names (`email`, `name`, `givenName`, `familyName`, `groups`; an empty string stops sending that value). Both calls need the application administrator role and are recorded in the audit log.

## Not supported yet

- Single logout.
- Encrypted assertions.
- Verifying signatures on authentication requests. Halo accepts signed requests but relies on the registered entity ID and ACS URLs instead.
- `ForceAuthn` and `IsPassive`. Halo reuses an existing Halo session, and sends people without one to the sign-in page.

## Troubleshooting

| Halo shows | What to do |
| --- | --- |
| **Halo couldn't read this sign-in request** with `unknown service provider` | The entity ID the application sends doesn't match the one registered in Halo. Copy it exactly, including any trailing slash. |
| **Halo couldn't read this sign-in request** about the ACS URL | Add the exact ACS URL the application uses on the **Authentication** tab. |
| **This sign-in request is no longer valid** | The request was older than 15 minutes or was already used. Start the sign-in again from the application. |
| **You don't have access to …** | Assign one of the person's groups to the application. |
| **… is turned off** | Enable the application in its **Danger zone**. |

# Slack

Let people sign in to Slack with Halo over SAML 2.0. SAML single sign-on needs a Slack plan that includes it, and a Slack owner or administrator to turn it on. The general SAML setup is described in [Connect an application with SAML 2.0](generic-saml.md); this page covers what is specific to Slack.

Replace `https://auth.example.com` with your `HALO_PUBLIC_URL`, and `example.slack.com` with your workspace address.

## 1. Register Slack in Halo

1. In the Halo console, open **Applications**, choose **Add application**, then **SAML 2.0**.
2. Pick the **Slack** template. It fills in:

   | Field | Value |
   | --- | --- |
   | Entity ID | `https://slack.com`, Slack's default service provider issuer |
   | ACS URL | `https://example.slack.com/sso/saml` |

3. Replace `example` in the ACS URL with your workspace's subdomain. If your Slack configuration uses a different service provider issuer, use that as the entity ID.
4. Keep the NameID format **Email address**. Slack matches people by the email address in the NameID.
5. Create the application, then assign the groups that should reach Slack on its **Users & groups** tab.

## 2. Configure Slack

In Slack's workspace settings, start configuring SAML authentication. Slack's form asks for three values:

| Slack asks for | Value |
| --- | --- |
| SAML 2.0 Endpoint (HTTP) | `https://auth.example.com/saml/sso` |
| Identity Provider Issuer | `https://auth.example.com/saml/metadata` |
| Public Certificate | The contents of the file from `https://auth.example.com/saml/certificate`, including the `BEGIN CERTIFICATE` and `END CERTIFICATE` lines |

If Slack asks whether responses or assertions are signed, match the **Signature** setting on the application's **Authentication** tab in Halo. Halo signs the assertion by default; **Sign the whole response** signs the response as well.

Test the configuration from Slack before you require SAML for everyone, and keep an owner signed in until the test succeeds.

## Attributes

Slack reads the email address from the NameID. To fill in other profile fields, rename the attributes on the application's **Attributes** tab in Halo to the attribute names Slack's SAML documentation lists, and clear the ones Slack doesn't use.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| Halo shows **Halo couldn't read this sign-in request** with `unknown service provider` | The entity ID in Halo doesn't match Slack's service provider issuer. Copy it from Slack's SAML settings. |
| Halo shows **Halo couldn't read this sign-in request** about the ACS URL | Check that the ACS URL in Halo uses your workspace's address. |
| Halo shows **You don't have access to Slack** | Assign one of the person's groups to the application in Halo. |
| Slack rejects the response | Check that the certificate in Slack is the current one from Halo, and that Slack's signature options match Halo's **Signature** setting. |

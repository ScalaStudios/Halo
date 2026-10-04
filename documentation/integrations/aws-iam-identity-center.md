# AWS IAM Identity Center

Use Halo as the external identity provider for AWS IAM Identity Center, so people sign in to the AWS access portal, the console and the CLI with Halo. The general SAML setup is described in [Connect an application with SAML 2.0](generic-saml.md); this page covers what is specific to AWS.

Replace `https://auth.example.com` with your `HALO_PUBLIC_URL`.

## Before you start

- You need permission to change the identity source in IAM Identity Center.
- Changing the identity source affects everyone who signs in to IAM Identity Center. Plan the switch and keep an administrator session open until you have tested it.
- Halo sends the person's email address as the NameID. Each person needs a user in IAM Identity Center whose username is that email address. Create the users in IAM Identity Center, or provision them automatically.

## 1. Start the change in AWS

In the IAM Identity Center console, open **Settings** and change the identity source to an **External identity provider**. AWS shows its service provider metadata: an **ACS URL**, an **issuer URL**, and a metadata file you can download. Keep this page open.

## 2. Register AWS in Halo

1. In the Halo console, open **Applications**, choose **Add application**, then **SAML 2.0**.
2. Pick the **AWS IAM Identity Center** template.
3. Either choose **Enter values** and use the IAM Identity Center issuer URL as the **Entity ID** and the IAM Identity Center ACS URL as the **ACS URL**, or choose **Paste metadata** and paste the XML from the metadata file AWS offers.
4. Keep the NameID format **Email address**.
5. Create the application, then assign the groups that should reach AWS on its **Users & groups** tab.

The template fills in example values with zeros in place of your region and IDs. Replace them with the values AWS shows.

## 3. Give AWS Halo's values

Back in AWS, in the identity provider metadata section, either upload the file you download from `https://auth.example.com/saml/metadata`, or enter the values one by one:

| AWS asks for | Value |
| --- | --- |
| IdP sign-in URL | `https://auth.example.com/saml/sso` |
| IdP issuer URL | `https://auth.example.com/saml/metadata` |
| IdP certificate | The file from `https://auth.example.com/saml/certificate` |

Review the change and confirm it.

## 4. Test

Open your AWS access portal URL in a private window. AWS sends you to Halo; after you sign in, Halo posts the response back and the portal opens. The attempt appears in Halo under **Sign-ins**.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| AWS reports that the user doesn't exist | Create the user in IAM Identity Center with the same email address as in Halo, or provision the users. |
| Halo shows **You don't have access to AWS IAM Identity Center** | Assign one of the person's groups to the application in Halo. |
| Halo shows **Halo couldn't read this sign-in request** | Compare the entity ID and ACS URL in Halo with the issuer URL and ACS URL AWS shows. They must match exactly. |

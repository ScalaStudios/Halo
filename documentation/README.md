# Halo documentation

Halo is an open-source identity and access management (IAM) platform that you host yourself: single sign-on over OpenID Connect and SAML, passkeys, SCIM provisioning, conditional access policies and access governance. It has not published a release yet; [CHANGELOG.md](../CHANGELOG.md) lists what works today.

## Start here

- [Get started](getting-started.md): run Halo on your computer, create the first administrator, and sign in to an example application through Halo.
- [Concepts](concepts.md): users, groups and their rules, roles, applications, sessions, sign-in methods, and the sign-in and audit logs.

## Run Halo

- [Self-hosting](self-hosting.md): what a production installation looks like, with the step-by-step guide in [deploy/README.md](../deploy/README.md).
- [Configuration](configuration.md): every environment variable, with its default and an example, and the settings you change in the console.
- [Email](email.md): connect a mail server, the outbox, and magic sign-in links.

## Manage access

- [Access policies](policies.md): conditional access, report-only mode, network zones, devices, sign-in risk, allowed sign-in methods and the simulator.
- [Access governance](governance.md): access packages, requests and approvals, access reviews, and joiner, mover and leaver rules.
- [Federation](federation.md): sign-in with Google, Microsoft Entra ID, GitHub or any OpenID Connect provider, account linking and accounts created on first sign-in.
- [Infrastructure access](infrastructure-access.md): Halo's SSH certificate authority, `halo login` and `halo ssh-cert`.

## Automate and integrate

- [Provisioning and automation](provisioning.md): service accounts and API keys, SCIM from Okta or Microsoft Entra ID into Halo, and SCIM from Halo into your applications.
- [Webhooks](webhooks.md): signed delivery of audit and sign-in events, with a Go receiver.
- [API resources](api-resources.md): protect your own APIs with Halo access tokens and validate them.
- [API](api.md): the JSON API behind the console and the account portal, with its [OpenAPI document](../api/openapi.yaml).

## Understand Halo

- [Architecture](architecture.md): the components, the Go packages, and how a sign-in to an application flows through them.
- [Security model](security-model.md): how Halo authenticates people and protects sessions, secrets and tokens, and its known limitations.

## Integrations

Step-by-step guides for connecting applications to Halo:

- [Generic OpenID Connect](integrations/generic-oidc.md): endpoints, claims and tokens for any application that supports OpenID Connect.
- [Grafana](integrations/grafana.md)
- [Forgejo](integrations/forgejo.md)
- [Nextcloud](integrations/nextcloud.md)
- [Kubernetes with kubelogin](integrations/kubernetes.md)
- [Proxmox VE](integrations/proxmox.md)
- [Outline](integrations/outline.md)
- [Headscale](integrations/headscale.md)
- [SAML 2.0](integrations/generic-saml.md): for applications that do not support OpenID Connect, with guides for [AWS IAM Identity Center](integrations/aws-iam-identity-center.md) and [Slack](integrations/slack.md).

## Contribute

[CONTRIBUTING.md](../CONTRIBUTING.md) covers the development environment, tests and code style. Report security problems privately as [SECURITY.md](../SECURITY.md) describes.

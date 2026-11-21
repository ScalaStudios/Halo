# Conformance

This file records what happens when the [OpenID Foundation conformance suite](https://gitlab.com/openid/conformance-suite) runs against Halo. We update it with each major release.

Halo is not OpenID Certified. These are results from running the suite ourselves, not a certification: certification means submitting the logs to the OpenID Foundation, which hasn't happened. Halo hasn't had an independent security audit either.

## Last run

- **Date:** 21 November 2026
- **Halo:** server built from `main` at `v0.1.1-164-gf3ecdc3` with the fixes listed under [Fixes from these runs](#fixes-from-these-runs)
- **Conformance suite:** `release-v5.3.4`, from the suite's published images
- **How:** `scripts/conformance/run.sh`, with the earlier conformance web image. The `conformance` GitHub Actions workflow runs the same script on a nightly schedule and on demand

| Profile | Test plan and variant | Modules | Passed | Warning | Review | Skipped | Failed |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Basic OP, static clients | `oidcc-basic-certification-test-plan[server_metadata=discovery][client_registration=static_client]` | 35 | 20 | 7 | 4 | 4 | 0 |
| Config OP | `oidcc-config-certification-test-plan` | 1 | 1 | 0 | 0 | 0 | 0 |
| Form Post OP, basic | `oidcc-formpost-basic-certification-test-plan[server_metadata=discovery][client_registration=static_client]` | 35 | 20 | 7 | 4 | 4 | 0 |
| Basic OP, dynamic registration | `oidcc-basic-certification-test-plan[server_metadata=discovery][client_registration=dynamic_client]` | 38 | 21 | 7 | 4 | 6 | 0 |
| RP-Initiated Logout OP | `oidcc-rp-initiated-logout-certification-test-plan[response_type=code][client_registration=static_client]` | 11 | 3 | 0 | 8 | 0 | 0 |
| Back-Channel Logout OP | `oidcc-backchannel-rp-initiated-logout-certification-test-plan[response_type=code][client_registration=static_client]` | 2 | 2 | 0 | 0 | 0 | 0 |

The plans use `client_secret_basic`, with `client_secret_post` in its own module, and the authorization code flow with the default response mode except in the Form Post plan.

**Review** is the suite's status for modules that require visual evidence. The scheduled harness uses rendered HTTP transcripts. In a supplemental Chromium run on 21 November 2026, all 20 review modules had screenshots of the expected result. Six showed the sign-in screen for `prompt=login` or `max_age`. Eleven showed request errors on Halo. Three showed the signed-out screen, after confirmation where required. The six sign-in modules were rerun after fixing the sign-in page's handling of fresh authentication requests.

## Known warnings

Each of these appears in the Basic, Form Post and dynamic registration plans.

| Test | Condition | Why |
| --- | --- | --- |
| `oidcc-server` | `EnsureIdTokenDoesNotContainNonRequestedClaims` | ID tokens carry `client_id`, which the [zitadel/oidc](https://github.com/zitadel/oidc) library Halo is built on adds to every ID token. It always equals `azp`. |
| `oidcc-scope-profile` | `VerifyScopesReturnedInUserInfoClaims` | Halo returns the profile claims it stores, `name`, `preferred_username` and `picture` when the person has one, and has no given name, family name, birthdate, locale or the other profile claims to return. |
| `oidcc-scope-email`, `oidcc-alternate-happy-flow` | `EnsureIdTokenDoesNotContainEmailForScopeEmail` | Halo puts `email` and `email_verified` in the ID token when the `email` scope is granted, because many applications read them from there. OpenID Connect allows it. |
| `oidcc-ensure-request-with-acr-values-succeeds` | `ValidateIdTokenACRClaimAgainstAcrValuesRequest` | The suite asks for `acr_values` Halo doesn't define. Halo returns `acr` only for the authentication contexts an administrator creates. |
| `oidcc-codereuse-30seconds` | `EnsureHttpStatusCodeIs4xx` | Halo refuses an authorization code used a second time, but doesn't revoke the access token issued the first time, which RFC 6749 section 4.1.2 recommends. |
| `oidcc-claims-essential` | `EnsureUserInfoContainsName` | Halo doesn't support the `claims` request parameter (`claims_parameter_supported` is false), so a claim requested only through it isn't returned. |

## Skipped tests

The suite skips these because Halo doesn't offer the feature, which certification allows:

- `oidcc-scope-address`, `oidcc-scope-phone` and `oidcc-scope-all`: Halo holds no address or phone number.
- `oidcc-unsigned-request-object-supported-correctly-or-rejected-as-unsupported`: Halo doesn't accept request objects and answers `request_not_supported`.
- `oidcc-request-uri-unsigned-supported-correctly-or-rejected-as-unsupported`: Halo accepts `request_uri` only from its own pushed authorization request endpoint and answers `request_uri_not_supported` for any other.
- `oidcc-idtoken-unsigned`: Halo never issues unsigned ID tokens.

## Profiles Halo doesn't meet

- **Implicit OP and Hybrid OP**, and their Form Post variants: Halo issues only the `code` response type.
- **Dynamic OP** (`oidcc-dynamic-certification-test-plan[response_type=code]`): run once on 21 November 2026 with 1 module passed, 3 skipped, 2 failed and 17 stopped. The profile needs the implicit grant with the `id_token` and `id_token token` response types, and most of its modules register clients with `private_key_jwt`, which the tested revision did not support for applications. The pairwise subject modules are skipped because Halo uses public subject identifiers. `oidcc-server-rotate-keys` also waits for someone to rotate the signing keys by hand. `scripts/conformance/run.sh "oidcc-dynamic-certification-test-plan[response_type=code]" out/config/dynamic.json` runs it.
- **Front-Channel Logout OP and Session Management OP**: Halo implements neither.
- **Third-Party Initiated Login**: not implemented.
- **FAPI 2.0 Security Profile**: not run. The tested revision predates the application-level `private_key_jwt` and FAPI profile settings. Halo's pushed authorization requests and DPoP are covered by its own tests in `internal/oidc/fapi_test.go`.

## Fixes from these runs

| Test that failed | Rule | Fix |
| --- | --- | --- |
| `oidcc-refresh-token` | RFC 6749 section 5.1 | Every token endpoint response sends `Cache-Control: no-store` and `Pragma: no-cache`. |
| `oidcc-response-type-missing`, `oidcc-prompt-none-not-logged-in` in the Form Post plan | OAuth 2.0 Form Post Response Mode section 2 | An authorization error is posted to the redirect URI when the request asks for `response_mode=form_post`, instead of being sent in the query. |
| `oidcc-unsigned-request-object-supported-correctly-or-rejected-as-unsupported` in the Form Post plan | OpenID Connect Core section 6.1 | `request_not_supported` goes back with the response mode and state the request object carries. |
| `oidcc-request-uri-unsigned-supported-correctly-or-rejected-as-unsupported` | OpenID Connect Core section 6.2 | A `request_uri` that isn't one of Halo's pushed authorization requests gets `request_uri_not_supported` at the registered redirect URI, instead of an error page. |
| `oidcc-prompt-login` and `oidcc-max-age-1` | OpenID Connect Core section 3.1.2.1 | An application asking for fresh authentication opens the normal sign-in form. |
| `oidcc-rp-initiated-logout-no-params` and `-only-state` | OpenID Connect RP-Initiated Logout section 2 | A plain logout request without an `id_token_hint`, or a logout request with a hint from another session, asks the person before signing them out. |
| `oidcc-rp-initiated-logout-no-id-token-hint` | OpenID Connect RP-Initiated Logout sections 3 and 4 | A request with a post-logout redirect must identify the application with an ID-token hint or a client ID. An unidentified request returns an error on Halo. |

## How the harness runs

`scripts/conformance/run.sh` builds Halo from the checkout, starts PostgreSQL, Halo behind Caddy at `https://halo.test`, and the suite's MongoDB, server and nginx with Docker Compose, then runs the plans through the suite's own `run-test-plan.py`. It needs Docker with Compose, Go, `openssl` and `curl`, and about 3 GB for images. The results go to `scripts/conformance/out/results`: one zip of signed logs for each plan, `summary.txt` with the suite's report, and `browser.log`. The workflow uploads them as the `conformance-results` artifact.

A few things differ from a production deployment so the suite can run unattended:

- **Sign-in without a browser.** The suite's built-in browser can't run Halo's sign-in page, so `browser.py` follows each URL the suite asks a person to open and signs in with a password through the same `/api/v1/auth` calls the sign-in page makes. The harness turns on password sign-in without a second factor for its test installation.
- **Registered clients get access.** Halo signs nobody in to an application they aren't assigned to, so the harness assigns its test group to each client the suite registers, as an administrator would.
- **The registration limit is lifted.** Halo allows 20 client registrations from one network an hour, and the suite registers two for each test, so the harness clears that counter every 15 seconds.
- **A test network and certificate.** Halo only sends back-channel logout tokens to public addresses, so the Docker network uses `198.18.28.0/24`, a benchmarking range that isn't private. One self-signed certificate covers both `halo.test` and the suite, and Halo trusts it through `SSL_CERT_FILE`.

To run it:

```bash
scripts/conformance/run.sh
```

Set `HALO_WEB_IMAGE` to use a web image you already have instead of building `deploy/Dockerfile.web`, `KEEP=1` to leave the containers running, `SUITE_TAG` for another suite release, and pass test plan and configuration pairs to run other plans. `expected-failures.json` and `expected-skips.json` list the known warnings and skips above, so the run fails on anything new, and also when one of them stops happening.

## Security audit

Halo hasn't had an independent security review of its authentication, session, token and SCIM code. That is left to the maintainers to commission, and its summary and fixes will be published here.

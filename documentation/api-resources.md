# API resources

An API resource describes one of your own APIs, such as a billing service, to Halo. Applications that you grant its scopes receive access tokens meant for that API: signed JWTs with the API's identifier as the audience. The API checks them itself, without calling Halo on each request.

Application administrators manage API resources; every administrator can read them.

## Concepts

| Term | Meaning |
| --- | --- |
| Identifier | An absolute URI that names the API, such as `https://billing.example.com`. Access tokens for the API carry it in `aud`. It does not have to resolve. |
| Scope | A permission the API understands, such as `billing:read`. Scope names are unique across all APIs, so prefix them with the API. |
| Grant | The scopes of one API that one application may request. |
| Access token lifetime | How long tokens for this API last, between 1 minute and 24 hours. |

## Add an API

1. In the console, open **API resources** and choose **Add API**.
2. Enter a name, such as `Billing API`, and the identifier.
3. Set the access token lifetime. Keep it short: the API cannot see revocations until a token expires.
4. Add between 1 and 100 scopes, each with a description. A scope name uses lowercase letters, digits and `: . _ / -`, up to 100 characters. Halo refuses `openid`, `profile`, `email`, `phone`, `address`, `offline_access` and `groups`, which it handles itself.
5. Choose **Save API**.

## Grant scopes to an application

1. Open the API and choose **Grant access** under **Applications**.
2. Pick the application and the scopes it may request, and choose **Save access**. OpenID Connect and OAuth applications can be granted scopes; SAML applications cannot, because they receive no access tokens.

Removing every scope removes the grant.

An application that has a grant on any API receives JWT access tokens from then on, even when it requests no API scope. Service applications and the Halo CLI always receive JWTs. Other applications receive opaque access tokens that only Halo can read.

## Request a token

An application asks for API scopes in one of two ways:

- **By scope.** Add the scopes to the `scope` parameter of the authorization request, the device authorization request or a client credentials token request, for example `scope=openid billing:read`.
- **By resource.** Send `resource=https://billing.example.com` ([RFC 8707](https://www.rfc-editor.org/rfc/rfc8707)). Halo adds every scope the application was granted on that API. A `resource` the application has no grant for fails with `invalid_target`.

Scopes the application was not granted are dropped from the request rather than refused.

A service application gets a token for its own use with the client credentials grant:

```bash
curl -s https://auth.example.com/oauth2/token \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -d grant_type=client_credentials \
  -d resource=https://billing.example.com
```

In the token Halo issues:

- `aud` lists the identifier of every API whose granted scopes were requested. When no API scope was requested, `aud` is the application's client ID.
- The token lasts as long as the shortest lifetime among those APIs, or the application's own access token lifetime when there are none.

## What the access token contains

The token is a JWT signed with RS256. Its header carries `typ: JWT` and the `kid` of the signing key. Its claims:

| Claim | Value |
| --- | --- |
| `iss` | Halo's issuer, `HALO_PUBLIC_URL`, such as `https://auth.example.com` |
| `sub` | The person's Halo id, such as `usr_01J…`, or the client ID for client credentials tokens |
| `aud` | An array of API identifiers |
| `exp`, `iat`, `nbf` | Expiry, issue time and not-before time, in Unix seconds |
| `jti` | The token's id |
| `client_id` | The application that requested the token |
| `scope` | The granted scopes, separated by spaces. `profile` and `email` are left out; `openid`, `groups` and `offline_access` stay when requested. |

Example payload:

```json
{
  "iss": "https://auth.example.com",
  "sub": "usr_01J9Z8Q4M2N5P7R9T1V3X5Z7B9",
  "aud": ["https://billing.example.com"],
  "exp": 1791101700,
  "iat": 1791100800,
  "nbf": 1791100800,
  "jti": "atk_01JA2X5Q6R7S8T9V0W1X2Y3Z4A",
  "client_id": "hl_6Wq1…",
  "scope": "openid billing:read"
}
```

The token does not carry the person's email address, name or groups. If the API needs them, call `/oauth2/userinfo` with the token, which requires the matching `email`, `profile` or `groups` scope.

## Validate tokens in your API

For every request, the API checks the bearer token:

1. Verify the RS256 signature with the key from `https://auth.example.com/oauth2/keys` whose `kid` matches the token header. Cache the key set and fetch it again when a token names a `kid` you have not seen.
2. Check that `iss` equals your `HALO_PUBLIC_URL` exactly.
3. Check that `aud` contains the API's identifier. This stops a token meant for another API, or for an application, from working here.
4. Check that `exp` is in the future, allowing a minute of clock skew.
5. Check that `scope` contains the scope the endpoint needs. ID tokens never carry a `scope` claim, so this check also refuses an ID token sent in place of an access token.
6. Use `sub` as the caller and `client_id` as the application acting for them.

The console shows the issuer and JWKS URL on each API's page.

In Go, with the [zitadel/oidc](https://github.com/zitadel/oidc) library that the example application in `examples/go-web-client` also uses:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	issuer   = "https://auth.example.com"
	audience = "https://billing.example.com"
)

var verifier = op.NewAccessTokenVerifier(issuer, rp.NewRemoteKeySet(http.DefaultClient, issuer+"/oauth2/keys"))

func requireScope(scope string, next func(w http.ResponseWriter, r *http.Request, caller string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "send an access token", http.StatusUnauthorized)
			return
		}
		claims, err := op.VerifyAccessToken[*oidc.AccessTokenClaims](r.Context(), token, verifier)
		if err != nil || !slices.Contains(claims.Audience, audience) {
			http.Error(w, "invalid access token", http.StatusUnauthorized)
			return
		}
		if !slices.Contains(claims.Scopes, scope) {
			http.Error(w, "missing scope "+scope, http.StatusForbidden)
			return
		}
		next(w, r, claims.Subject)
	}
}

func main() {
	http.HandleFunc("GET /invoices", requireScope("billing:read", func(w http.ResponseWriter, r *http.Request, caller string) {
		fmt.Fprintf(w, "invoices for %s\n", caller)
	}))
	log.Fatal(http.ListenAndServe(":9200", nil))
}
```

`op.VerifyAccessToken` checks the issuer, the signature and the expiry; the handler adds the audience and scope checks.

Halo's introspection endpoint answers only the application a token was issued to, so APIs validate JWTs themselves as shown here. Validation does not notice that Halo revoked a token, for example because the person was suspended or the application disabled, until the token expires. Choose the access token lifetime with that in mind.

## API

API resources are at `/api/v1/api-resources`, and grants at `PUT /api/v1/api-resources/{id}/grants/{appId}`; see the [API](api.md).

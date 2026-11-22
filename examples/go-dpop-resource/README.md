# DPoP-protected billing API

This Go example serves a sample balance from `GET /balance`. It checks Halo's JWT signature, issuer, expiration, audience and `billing.read` scope. It also verifies the caller's DPoP proof, compares the proof key with the token's `cnf.jkt`, checks the `ath` token hash and rejects reused proofs.

## Run with Halo

1. Create an API resource with identifier `http://localhost:9002` and scope `billing.read`.
2. Register an OAuth service application, enable **Require DPoP** and grant it `billing.read` on the new resource.
3. Start the API from the root of this repository.

   ```sh
   HALO_ISSUER=http://localhost:3200 go run ./examples/go-dpop-resource
   ```

4. Request a client credentials token from Halo's `/oauth2/token` with `scope=billing.read` and `resource=http://localhost:9002`. Authenticate the OAuth client using its configured method. Send a DPoP proof with a unique `jti`, a current `iat`, `htm=POST`, and `htu` set to the token endpoint URL. Halo returns an access token with `token_type=DPoP`.
5. Use your DPoP client to sign a fresh proof with the same key. Set `htm` to `GET`, `htu` to `http://localhost:9002/balance`, and `ath` to the base64url-encoded SHA-256 hash of the access token, without padding. Send the token and proof together.

   ```sh
   curl http://localhost:9002/balance \
     -H "Authorization: DPoP $ACCESS_TOKEN" \
     -H "DPoP: $RESOURCE_PROOF"
   ```

A successful response contains `{"balance_cents":4200,"currency":"USD"}`.

`HALO_RESOURCE` sets the expected resource identifier and the public base URL used to match the proof. The API listens on loopback. The replay cache is local to the process and is cleared when it restarts. A deployment with multiple instances needs a shared store that claims each proof ID atomically. This example validates signed JWT access tokens locally. To enforce immediate revocation, add authorization server introspection.

Run `go test ./examples/go-dpop-resource` to check the sample with generated tokens and proofs without starting Halo. See [RFC 9449](https://www.rfc-editor.org/rfc/rfc9449) for proof JWT headers and claims.

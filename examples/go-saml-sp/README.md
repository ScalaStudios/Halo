# Example: a Go service provider that signs in with Halo over SAML

A small web app that starts SP-initiated SAML sign-in through Halo (HTTP-Redirect request, HTTP-POST response), checks the signed assertion against Halo's metadata, and shows the NameID, the groups and every attribute it receives. It uses [crewjam/saml](https://github.com/crewjam/saml), which Halo already depends on.

1. In the Halo console, open **Applications → Add application → SAML 2.0**, choose **Enter values**, and use:
   - Entity ID `http://localhost:9100/saml/metadata`
   - ACS URL `http://localhost:9100/saml/acs`

   Assign it to a group you belong to.
2. Run the app:

   ```bash
   go run ./examples/go-saml-sp
   ```

3. Open http://localhost:9100. The app sends you to Halo, and after you sign in it shows what Halo asserted about you.

Settings: `HALO_URL` (default `http://localhost:3200`, the app reads `$HALO_URL/saml/metadata` at startup), `BASE_URL` (default `http://localhost:9100`), `ENTITY_ID` (default `$BASE_URL/saml/metadata`), `LISTEN` (default `localhost:9100`).

The app keeps pending requests in memory and has no session of its own, so every visit to `/` signs in again.

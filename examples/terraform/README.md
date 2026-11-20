# Example: manage Halo with Terraform or OpenTofu

`main.tf` creates two groups, a person and their membership, an OpenID Connect application, an access policy in report-only mode and an API resource with two scopes.

1. In the Halo console, open **Service accounts**, add one with the roles the configuration needs (user administrator, application administrator and security administrator, or global administrator), and create an API key with the `api` scope.
2. Build the provider:

   ```bash
   go build -o "$HOME/.local/bin/terraform-provider-halo" ./cmd/terraform-provider-halo
   ```

3. The provider is not in a registry yet, so point Terraform (`~/.terraformrc`) or OpenTofu (`~/.tofurc`) at the binary:

   ```hcl
   provider_installation {
     dev_overrides {
       "scalastudios/halo" = "/home/you/.local/bin"
     }
     direct {}
   }
   ```

4. Apply the example. With `dev_overrides`, skip `init`:

   ```bash
   export HALO_ENDPOINT=https://auth.example.com HALO_API_KEY=hlk_…
   cd examples/terraform
   tofu apply
   ```

Resources: `halo_user`, `halo_group`, `halo_group_member`, `halo_application`, `halo_access_policy` and `halo_api_resource`. Each one imports by its Halo id, and `halo_group_member` by `<group id>/<user id>`:

```bash
tofu import halo_group.engineering grp_01J9Z8Q4M2N5P7R9T1V3X5Z7B9
```

Destroying a `halo_user` suspends the account rather than deleting it; a global administrator deletes a person permanently in the console. Applications are OpenID Connect and OAuth only; manage SAML applications in the console.

Run the provider's acceptance tests against a throwaway Halo and PostgreSQL with `TF_ACC=1 go test ./internal/terraform`. To run them with OpenTofu, also set `TF_ACC_TERRAFORM_PATH` to the `tofu` binary and `TF_ACC_PROVIDER_NAMESPACE=hashicorp`.

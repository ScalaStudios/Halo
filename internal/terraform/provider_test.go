package terraform_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"halo/internal/config"
	"halo/internal/server"
	"halo/internal/store"
	"halo/internal/terraform"
	"halo/internal/testdb"
)

func halo(t *testing.T) (string, string) {
	t.Helper()
	st := testdb.New(t)
	public, _ := url.Parse("http://localhost:3200")
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	app, err := server.New(config.Config{Organization: "Test", PublicURL: public, SecretKey: secret, Dev: true}, st)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	account, err := st.CreateServiceAccount(ctx, store.NewServiceAccount{Name: "terraform", Roles: []string{"global_admin"}})
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := st.CreateAPIKey(ctx, account.ID, "terraform", []string{"api"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, key
}

func TestSchemas(t *testing.T) {
	ctx := context.Background()
	p := terraform.New("test")()
	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var meta fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "halo"}, &meta)
		var resp fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &resp)
		resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)
		if resp.Diagnostics.HasError() {
			t.Errorf("%s: %v", meta.TypeName, resp.Diagnostics)
		}
	}
}

func TestClient(t *testing.T) {
	endpoint, key := halo(t)
	ctx := context.Background()
	c := &terraform.Client{Endpoint: endpoint, APIKey: key, HTTP: http.DefaultClient}
	var g store.Group
	if err := c.Do(ctx, http.MethodPost, "/groups", map[string]string{"name": "Engineering", "kind": "assigned"}, &g); err != nil {
		t.Fatal(err)
	}
	if g.ID == "" || g.Name != "Engineering" {
		t.Fatalf("unexpected group %+v", g)
	}
	if err := c.Do(ctx, http.MethodDelete, "/groups/"+g.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	err := c.Do(ctx, http.MethodGet, "/groups/"+g.ID, nil, &g)
	if e, ok := err.(*terraform.APIError); !ok || e.Status != http.StatusNotFound || e.Code != "ERR_NOT_FOUND" {
		t.Fatalf("want a 404 ERR_NOT_FOUND, got %v", err)
	}
	bad := &terraform.Client{Endpoint: endpoint, APIKey: "hlk_wrong", HTTP: http.DefaultClient}
	if err := bad.Do(ctx, http.MethodGet, "/groups", nil, nil); err == nil {
		t.Fatal("want an error for a wrong key")
	}
}

func TestAcc(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run the acceptance tests")
	}
	endpoint, key := halo(t)
	provider := fmt.Sprintf("provider \"halo\" {\n  endpoint = %q\n  api_key  = %q\n}\n", endpoint, key)
	config := func(v map[string]string) string {
		return provider + fmt.Sprintf(`
resource "halo_user" "ada" {
  email = "ada@example.com"
  name  = "Ada Lovelace"
  title = %[1]q
}

resource "halo_group" "engineering" {
  name        = "Engineering"
  description = %[2]q
  kind        = "assigned"
}

resource "halo_group" "berlin" {
  name = "Berlin"
  kind = "dynamic"
  rule = %[3]q
}

resource "halo_group_member" "ada" {
  group_id = halo_group.engineering.id
  user_id  = halo_user.ada.id
}

resource "halo_application" "grafana" {
  name          = %[4]q
  protocol      = "oidc"
  type          = "web"
  redirect_uris = [%[5]s]
  group_ids     = [halo_group.engineering.id]
}

resource "halo_application" "dashboard" {
  name          = %[6]q
  protocol      = "oidc"
  type          = "spa"
  redirect_uris = ["https://dashboard.example.com/callback"]
}

resource "halo_access_policy" "phishing_resistant" {
  name    = %[7]q
  enabled = false
  mode    = "report"
  effect  = "require-phishing-resistant"
  conditions = {
    %[8]s
    app_ids = [halo_application.grafana.id]
    risk    = ["high", "medium"]
  }
}

resource "halo_api_resource" "billing" {
  name             = "Billing API"
  identifier       = "https://billing.example.com"
  access_token_ttl = %[9]s
  scopes = [
    { name = "billing:read", description = "Read invoices" },
    %[10]s
  ]
}
`, v["title"], v["description"], v["rule"], v["app"], v["redirects"], v["spa"], v["policy"], v["users"], v["ttl"], v["scopes"])
	}
	first := map[string]string{
		"title": "Engineer", "description": "Builds the product", "rule": `user.location == "Berlin"`,
		"app": "Grafana", "redirects": `"https://grafana.example.com/login/generic_oauth"`, "spa": "Dashboard",
		"policy": "Phishing-resistant for Grafana", "users": "group_ids = [halo_group.engineering.id]",
		"ttl": "900", "scopes": `{ name = "billing:write" },`,
	}
	second := map[string]string{
		"title": "Principal engineer", "description": "Builds and runs the product", "rule": `user.location == "Berlin" or user.location == "Potsdam"`,
		"app": "Grafana Cloud", "redirects": `"https://grafana.example.com/login/generic_oauth", "https://grafana.example.org/login/generic_oauth"`, "spa": "Status dashboard",
		"policy": "Phishing-resistant everywhere", "users": "all_users = true",
		"ttl": "1800", "scopes": `{ name = "billing:write", description = "Change invoices" }, { name = "billing:refund" },`,
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"halo": providerserver.NewProtocol6WithError(terraform.New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: config(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("halo_user.ada", "status", "invited"),
					resource.TestCheckResourceAttr("halo_application.grafana", "scopes.#", "4"),
					resource.TestCheckResourceAttrSet("halo_application.grafana", "client_secret"),
					resource.TestCheckNoResourceAttr("halo_application.dashboard", "client_secret"),
					resource.TestCheckResourceAttr("halo_access_policy.phishing_resistant", "conditions.network", "any"),
					resource.TestCheckResourceAttr("halo_api_resource.billing", "scopes.#", "2"),
				),
			},
			{
				Config: config(second),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("halo_user.ada", "title", "Principal engineer"),
					resource.TestCheckResourceAttr("halo_group.engineering", "description", "Builds and runs the product"),
					resource.TestCheckResourceAttr("halo_application.grafana", "redirect_uris.#", "2"),
					resource.TestCheckResourceAttrSet("halo_application.grafana", "client_secret"),
					resource.TestCheckResourceAttr("halo_application.dashboard", "name", "Status dashboard"),
					resource.TestCheckResourceAttr("halo_api_resource.billing", "access_token_ttl", "1800"),
					resource.TestCheckResourceAttr("halo_api_resource.billing", "scopes.#", "3"),
					resource.TestCheckResourceAttr("halo_access_policy.phishing_resistant", "conditions.all_users", "true"),
					resource.TestCheckResourceAttr("halo_access_policy.phishing_resistant", "conditions.group_ids.#", "0"),
				),
			},
			{ResourceName: "halo_user.ada", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_group.engineering", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_group.berlin", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_group_member.ada", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_application.grafana", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"client_secret"}},
			{ResourceName: "halo_application.dashboard", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_access_policy.phishing_resistant", ImportState: true, ImportStateVerify: true},
			{ResourceName: "halo_api_resource.billing", ImportState: true, ImportStateVerify: true},
		},
	})
}

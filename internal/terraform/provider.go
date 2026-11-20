package terraform

import (
	"context"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type haloProvider struct {
	version string
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIKey   types.String `tfsdk:"api_key"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &haloProvider{version: version} }
}

func (p *haloProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "halo"
	resp.Version = p.version
}

func (p *haloProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages users, groups, applications, access policies and API resources in Halo through the `/api/v1` management API.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The address of the Halo server, such as `https://auth.example.com`. Defaults to `HALO_ENDPOINT`.",
			},
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "An API key of a service account with the `api` scope, starting with `hlk_`. Defaults to `HALO_API_KEY`.",
			},
		},
	}
}

func (p *haloProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	client := &Client{Endpoint: os.Getenv("HALO_ENDPOINT"), APIKey: os.Getenv("HALO_API_KEY"), HTTP: http.DefaultClient}
	if v := cfg.Endpoint.ValueString(); v != "" {
		client.Endpoint = v
	}
	if v := cfg.APIKey.ValueString(); v != "" {
		client.APIKey = v
	}
	if client.Endpoint == "" || client.APIKey == "" {
		resp.Diagnostics.AddError("Missing Halo credentials", "Set endpoint and api_key in the provider block, or HALO_ENDPOINT and HALO_API_KEY in the environment.")
		return
	}
	resp.ResourceData = client
}

func (p *haloProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &userResource{} },
		func() resource.Resource { return &groupResource{} },
		func() resource.Resource { return &groupMemberResource{} },
		func() resource.Resource { return &applicationResource{} },
		func() resource.Resource { return &accessPolicyResource{} },
		func() resource.Resource { return &apiResourceResource{} },
	}
}

func (p *haloProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

func apiFailed(diags *diag.Diagnostics, action string, err error) {
	diags.AddError("Halo could not "+action, err.Error())
}

func stringList(values []string) types.List {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

func stringSet(values []string) types.Set {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

func stringsOf(ctx context.Context, v interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(context.Context, any, bool) diag.Diagnostics
}) []string {
	out := []string{}
	if v.IsNull() || v.IsUnknown() {
		return out
	}
	v.ElementsAs(ctx, &out, false)
	return out
}

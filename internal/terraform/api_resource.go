package terraform

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"halo/internal/store"
)

type apiResourceResource struct {
	withClient
}

type apiResourceModel struct {
	ID             types.String    `tfsdk:"id"`
	Name           types.String    `tfsdk:"name"`
	Identifier     types.String    `tfsdk:"identifier"`
	Description    types.String    `tfsdk:"description"`
	AccessTokenTTL types.Int64     `tfsdk:"access_token_ttl"`
	Scopes         []apiScopeModel `tfsdk:"scopes"`
}

type apiScopeModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

type apiResourceInput struct {
	Name           string           `json:"name"`
	Identifier     string           `json:"identifier"`
	Description    string           `json:"description"`
	AccessTokenTTL int64            `json:"accessTokenTtl"`
	Scopes         []store.APIScope `json:"scopes"`
}

func (r *apiResourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_resource"
}

func (r *apiResourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One of your own APIs and the scopes applications can request on it. Changing it keeps the grants of scopes that remain.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":             schema.StringAttribute{Required: true},
			"identifier":       schema.StringAttribute{Required: true, MarkdownDescription: "The audience access tokens carry in `aud`, such as `https://billing.example.com`."},
			"description":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"access_token_ttl": schema.Int64Attribute{Required: true, MarkdownDescription: "Seconds, from 60 to 86400."},
			"scopes": schema.SetNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Required: true, MarkdownDescription: "Unique across every API, such as `billing:read`."},
						"description": schema.StringAttribute{Optional: true},
					},
				},
			},
		},
	}
}

func (m *apiResourceModel) input() apiResourceInput {
	in := apiResourceInput{Name: m.Name.ValueString(), Identifier: m.Identifier.ValueString(), Description: m.Description.ValueString(), AccessTokenTTL: m.AccessTokenTTL.ValueInt64(), Scopes: []store.APIScope{}}
	for _, s := range m.Scopes {
		in.Scopes = append(in.Scopes, store.APIScope{Name: s.Name.ValueString(), Description: s.Description.ValueString()})
	}
	return in
}

func (m *apiResourceModel) set(res store.APIResource) {
	m.ID = types.StringValue(res.ID)
	m.Name = types.StringValue(res.Name)
	m.Identifier = types.StringValue(res.Identifier)
	m.Description = types.StringValue(res.Description)
	m.AccessTokenTTL = types.Int64Value(int64(res.AccessTokenTTL))
	m.Scopes = []apiScopeModel{}
	for _, s := range res.Scopes {
		description := types.StringNull()
		if s.Description != "" {
			description = types.StringValue(s.Description)
		}
		m.Scopes = append(m.Scopes, apiScopeModel{Name: types.StringValue(s.Name), Description: description})
	}
}

func (r *apiResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m apiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var res store.APIResource
	if err := r.client.Do(ctx, http.MethodPost, "/api-resources", m.input(), &res); err != nil {
		apiFailed(&resp.Diagnostics, "add the API resource", err)
		return
	}
	m.set(res)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *apiResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m apiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var res store.APIResource
	err := r.client.Do(ctx, http.MethodGet, "/api-resources/"+url.PathEscape(m.ID.ValueString()), nil, &res)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the API resource", err)
		return
	}
	m.set(res)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *apiResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m apiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var res store.APIResource
	if err := r.client.Do(ctx, http.MethodPut, "/api-resources/"+url.PathEscape(m.ID.ValueString()), m.input(), &res); err != nil {
		apiFailed(&resp.Diagnostics, "update the API resource", err)
		return
	}
	m.set(res)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *apiResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m apiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, "/api-resources/"+url.PathEscape(m.ID.ValueString()), nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "delete the API resource", err)
	}
}

func (r *apiResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

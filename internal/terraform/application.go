package terraform

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"halo/internal/store"
)

type applicationResource struct {
	withClient
}

type applicationModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Protocol       types.String `tfsdk:"protocol"`
	Type           types.String `tfsdk:"type"`
	Homepage       types.String `tfsdk:"homepage"`
	RedirectURIs   types.List   `tfsdk:"redirect_uris"`
	PostLogoutURIs types.List   `tfsdk:"post_logout_uris"`
	Scopes         types.List   `tfsdk:"scopes"`
	GroupIDs       types.Set    `tfsdk:"group_ids"`
	ClientID       types.String `tfsdk:"client_id"`
	ClientSecret   types.String `tfsdk:"client_secret"`
}

type applicationInput struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Protocol       string   `json:"protocol,omitempty"`
	Type           string   `json:"type,omitempty"`
	Homepage       string   `json:"homepage"`
	RedirectURIs   []string `json:"redirectUris"`
	PostLogoutURIs []string `json:"postLogoutUris"`
	Scopes         []string `json:"scopes,omitempty"`
	GroupIDs       []string `json:"groupIds"`
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	emptyList := listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{}))
	resp.Schema = schema.Schema{
		MarkdownDescription: "An OpenID Connect or OAuth application.",
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name":             schema.StringAttribute{Required: true},
			"description":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"protocol":         schema.StringAttribute{Required: true, MarkdownDescription: "`oidc` or `oauth`. Changing it replaces the application.", PlanModifiers: replace},
			"type":             schema.StringAttribute{Required: true, MarkdownDescription: "`web`, `spa`, `native` or `service`. Changing it replaces the application.", PlanModifiers: replace},
			"homepage":         schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"redirect_uris":    schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, Default: emptyList},
			"post_logout_uris": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, Default: emptyList},
			"scopes":           schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, MarkdownDescription: "Defaults to `openid`, `profile`, `email` and `groups`.", PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
			"group_ids":        schema.SetAttribute{ElementType: types.StringType, Optional: true, Computed: true, MarkdownDescription: "Groups whose members may sign in.", Default: setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{}))},
			"client_id":        schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"client_secret":    schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Web applications and services only. Halo returns it once, on creation, so it is empty after an import.", PlanModifiers: keep},
		},
	}
}

func (m *applicationModel) input(ctx context.Context) applicationInput {
	return applicationInput{
		Name:           m.Name.ValueString(),
		Description:    m.Description.ValueString(),
		Protocol:       m.Protocol.ValueString(),
		Type:           m.Type.ValueString(),
		Homepage:       m.Homepage.ValueString(),
		RedirectURIs:   stringsOf(ctx, m.RedirectURIs),
		PostLogoutURIs: stringsOf(ctx, m.PostLogoutURIs),
		Scopes:         stringsOf(ctx, m.Scopes),
		GroupIDs:       stringsOf(ctx, m.GroupIDs),
	}
}

func (m *applicationModel) set(a store.Application) {
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Description = types.StringValue(a.Description)
	m.Protocol = types.StringValue(a.Protocol)
	m.Type = types.StringValue(a.Type)
	m.Homepage = types.StringValue(a.Homepage)
	m.RedirectURIs = stringList(a.RedirectURIs)
	m.PostLogoutURIs = stringList(a.PostLogoutURIs)
	m.Scopes = stringList(a.Scopes)
	m.GroupIDs = stringSet(a.GroupIDs)
	m.ClientID = types.StringValue(a.ClientID)
}

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Application  store.Application `json:"application"`
		ClientSecret *string           `json:"clientSecret"`
	}
	if err := r.client.Do(ctx, http.MethodPost, "/applications", m.input(ctx), &out); err != nil {
		apiFailed(&resp.Diagnostics, "register the application", err)
		return
	}
	m.set(out.Application)
	m.ClientSecret = types.StringPointerValue(out.ClientSecret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var a store.Application
	err := r.client.Do(ctx, http.MethodGet, "/applications/"+url.PathEscape(m.ID.ValueString()), nil, &a)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the application", err)
		return
	}
	m.set(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := m.input(ctx)
	in.Protocol, in.Type = "", ""
	var a store.Application
	if err := r.client.Do(ctx, http.MethodPatch, "/applications/"+url.PathEscape(m.ID.ValueString()), in, &a); err != nil {
		apiFailed(&resp.Diagnostics, "update the application", err)
		return
	}
	m.set(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, "/applications/"+url.PathEscape(m.ID.ValueString()), nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "delete the application", err)
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

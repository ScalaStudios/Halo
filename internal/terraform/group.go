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

type groupResource struct {
	withClient
}

type groupModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Kind        types.String `tfsdk:"kind"`
	Rule        types.String `tfsdk:"rule"`
}

type groupInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Kind        string  `json:"kind,omitempty"`
	Rule        *string `json:"rule,omitempty"`
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An assigned or rule-based group.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":        schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"kind":        schema.StringAttribute{Required: true, MarkdownDescription: "`assigned` or `dynamic`. Changing it replaces the group.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"rule":        schema.StringAttribute{Optional: true, MarkdownDescription: "Dynamic groups only, such as `user.department == \"Engineering\"`."},
		},
	}
}

func (m *groupModel) input() groupInput {
	return groupInput{Name: m.Name.ValueString(), Description: m.Description.ValueString(), Kind: m.Kind.ValueString(), Rule: m.Rule.ValueStringPointer()}
}

func (m *groupModel) set(g store.Group) {
	m.ID = types.StringValue(g.ID)
	m.Name = types.StringValue(g.Name)
	m.Description = types.StringValue(g.Description)
	m.Kind = types.StringValue(g.Kind)
	m.Rule = types.StringPointerValue(g.Rule)
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m groupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var g store.Group
	if err := r.client.Do(ctx, http.MethodPost, "/groups", m.input(), &g); err != nil {
		apiFailed(&resp.Diagnostics, "create the group", err)
		return
	}
	m.set(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var g store.Group
	err := r.client.Do(ctx, http.MethodGet, "/groups/"+url.PathEscape(m.ID.ValueString()), nil, &g)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the group", err)
		return
	}
	m.set(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m groupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var g store.Group
	if err := r.client.Do(ctx, http.MethodPatch, "/groups/"+url.PathEscape(m.ID.ValueString()), m.input(), &g); err != nil {
		apiFailed(&resp.Diagnostics, "update the group", err)
		return
	}
	m.set(g)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, "/groups/"+url.PathEscape(m.ID.ValueString()), nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "delete the group", err)
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

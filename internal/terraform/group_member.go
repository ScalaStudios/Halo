package terraform

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"halo/internal/store"
)

type groupMemberResource struct {
	withClient
}

type groupMemberModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	UserID  types.String `tfsdk:"user_id"`
}

func (r *groupMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_member"
}

func (r *groupMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A person's membership of an assigned group. Import with `<group id>/<user id>`.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"group_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"user_id":  schema.StringAttribute{Required: true, PlanModifiers: replace},
		},
	}
}

func (r *groupMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m groupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]string{"userId": m.UserID.ValueString()}
	if err := r.client.Do(ctx, http.MethodPost, "/groups/"+url.PathEscape(m.GroupID.ValueString())+"/members", body, nil); err != nil {
		apiFailed(&resp.Diagnostics, "add the person to the group", err)
		return
	}
	m.ID = types.StringValue(m.GroupID.ValueString() + "/" + m.UserID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *groupMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m groupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var u store.User
	err := r.client.Do(ctx, http.MethodGet, "/users/"+url.PathEscape(m.UserID.ValueString()), nil, &u)
	if notFound(err) || err == nil && !slices.Contains(u.GroupIDs, m.GroupID.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the person's groups", err)
	}
}

func (r *groupMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m groupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *groupMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m groupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, "/groups/"+url.PathEscape(m.GroupID.ValueString())+"/members/"+url.PathEscape(m.UserID.ValueString()), nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "remove the person from the group", err)
	}
}

func (r *groupMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	groupID, userID, ok := strings.Cut(req.ID, "/")
	if !ok || groupID == "" || userID == "" {
		resp.Diagnostics.AddError("Invalid import id", "Import a membership as <group id>/<user id>, such as grp_01J…/usr_01J….")
		return
	}
	m := groupMemberModel{ID: types.StringValue(req.ID), GroupID: types.StringValue(groupID), UserID: types.StringValue(userID)}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

package terraform

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"halo/internal/store"
)

type accessPolicyResource struct {
	withClient
}

type accessPolicyModel struct {
	ID          types.String     `tfsdk:"id"`
	Name        types.String     `tfsdk:"name"`
	Description types.String     `tfsdk:"description"`
	Enabled     types.Bool       `tfsdk:"enabled"`
	Mode        types.String     `tfsdk:"mode"`
	Effect      types.String     `tfsdk:"effect"`
	Priority    types.Int64      `tfsdk:"priority"`
	Conditions  *conditionsModel `tfsdk:"conditions"`
}

type conditionsModel struct {
	AllUsers        types.Bool   `tfsdk:"all_users"`
	GroupIDs        types.Set    `tfsdk:"group_ids"`
	UserIDs         types.Set    `tfsdk:"user_ids"`
	ExcludeGroupIDs types.Set    `tfsdk:"exclude_group_ids"`
	ExcludeUserIDs  types.Set    `tfsdk:"exclude_user_ids"`
	AllApps         types.Bool   `tfsdk:"all_apps"`
	AppIDs          types.Set    `tfsdk:"app_ids"`
	Network         types.String `tfsdk:"network"`
	ZoneIDs         types.Set    `tfsdk:"zone_ids"`
	Device          types.String `tfsdk:"device"`
	Compliance      types.String `tfsdk:"compliance"`
	Risk            types.Set    `tfsdk:"risk"`
	Methods         types.Set    `tfsdk:"methods"`
}

type accessPolicyInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Mode        string                 `json:"mode"`
	Effect      string                 `json:"effect"`
	Conditions  store.PolicyConditions `json:"conditions"`
}

func (r *accessPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_policy"
}

func (r *accessPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	ids := func(description string) schema.SetAttribute {
		return schema.SetAttribute{ElementType: types.StringType, Optional: true, Computed: true, MarkdownDescription: description, Default: setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{}))}
	}
	flag := schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A conditional access policy. New policies go to the end of the order.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":        schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"enabled":     schema.BoolAttribute{Required: true},
			"mode":        schema.StringAttribute{Required: true, MarkdownDescription: "`enforce` or `report`."},
			"effect":      schema.StringAttribute{Required: true, MarkdownDescription: "`allow`, `require-phishing-resistant` or `block`."},
			"priority":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Position in the saved order."},
			"conditions": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"all_users":         flag,
					"group_ids":         ids("Leave empty when `all_users` is set."),
					"user_ids":          ids("Leave empty when `all_users` is set."),
					"exclude_group_ids": ids(""),
					"exclude_user_ids":  ids(""),
					"all_apps":          flag,
					"app_ids":           ids("Application ids, or `halo` for the console and account portal. Leave empty when `all_apps` is set."),
					"network":           schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("any"), MarkdownDescription: "`any`, `in` or `not-in`."},
					"zone_ids":          ids("Network zones for `in` and `not-in`."),
					"device":            schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("any"), MarkdownDescription: "`any`, `trusted` or `untrusted`."},
					"compliance":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("any"), MarkdownDescription: "`any`, `compliant` or `not-compliant`, as reported by a device compliance source."},
					"risk":              ids("`none`, `low`, `medium` or `high`. Empty matches every level."),
					"methods":           ids("Sign-in methods to match. Empty matches every method."),
				},
			},
		},
	}
}

func (r *accessPolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("conditions"), &obj)...)
	if resp.Diagnostics.HasError() || obj.IsNull() || obj.IsUnknown() {
		return
	}
	var c conditionsModel
	resp.Diagnostics.Append(obj.As(ctx, &c, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}
	nonEmpty := func(s types.Set) bool { return !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 }
	if c.AllUsers.ValueBool() && (nonEmpty(c.GroupIDs) || nonEmpty(c.UserIDs)) {
		resp.Diagnostics.AddAttributeError(path.Root("conditions"), "Conflicting conditions", "Leave group_ids and user_ids empty when all_users is true.")
	}
	if c.AllApps.ValueBool() && nonEmpty(c.AppIDs) {
		resp.Diagnostics.AddAttributeError(path.Root("conditions"), "Conflicting conditions", "Leave app_ids empty when all_apps is true.")
	}
	if (c.Network.IsNull() || c.Network.ValueString() == "any") && nonEmpty(c.ZoneIDs) {
		resp.Diagnostics.AddAttributeError(path.Root("conditions"), "Conflicting conditions", "Set network to in or not-in to use zone_ids.")
	}
}

func (m *accessPolicyModel) input(ctx context.Context) accessPolicyInput {
	c := m.Conditions
	return accessPolicyInput{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		Enabled:     m.Enabled.ValueBool(),
		Mode:        m.Mode.ValueString(),
		Effect:      m.Effect.ValueString(),
		Conditions: store.PolicyConditions{
			AllUsers:        c.AllUsers.ValueBool(),
			GroupIDs:        stringsOf(ctx, c.GroupIDs),
			UserIDs:         stringsOf(ctx, c.UserIDs),
			ExcludeGroupIDs: stringsOf(ctx, c.ExcludeGroupIDs),
			ExcludeUserIDs:  stringsOf(ctx, c.ExcludeUserIDs),
			AllApps:         c.AllApps.ValueBool(),
			AppIDs:          stringsOf(ctx, c.AppIDs),
			Network:         c.Network.ValueString(),
			ZoneIDs:         stringsOf(ctx, c.ZoneIDs),
			Device:          c.Device.ValueString(),
			Compliance:      c.Compliance.ValueString(),
			Risk:            stringsOf(ctx, c.Risk),
			Methods:         stringsOf(ctx, c.Methods),
		},
	}
}

func (m *accessPolicyModel) set(p store.AccessPolicy) {
	c := p.Conditions
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Description = types.StringValue(p.Description)
	m.Enabled = types.BoolValue(p.Enabled)
	m.Mode = types.StringValue(p.Mode)
	m.Effect = types.StringValue(p.Effect)
	m.Priority = types.Int64Value(int64(p.Priority))
	m.Conditions = &conditionsModel{
		AllUsers:        types.BoolValue(c.AllUsers),
		GroupIDs:        stringSet(c.GroupIDs),
		UserIDs:         stringSet(c.UserIDs),
		ExcludeGroupIDs: stringSet(c.ExcludeGroupIDs),
		ExcludeUserIDs:  stringSet(c.ExcludeUserIDs),
		AllApps:         types.BoolValue(c.AllApps),
		AppIDs:          stringSet(c.AppIDs),
		Network:         types.StringValue(c.Network),
		ZoneIDs:         stringSet(c.ZoneIDs),
		Device:          types.StringValue(c.Device),
		Compliance:      types.StringValue(c.Compliance),
		Risk:            stringSet(c.Risk),
		Methods:         stringSet(c.Methods),
	}
}

func (r *accessPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m accessPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var p store.AccessPolicy
	if err := r.client.Do(ctx, http.MethodPost, "/policies", m.input(ctx), &p); err != nil {
		apiFailed(&resp.Diagnostics, "create the access policy", err)
		return
	}
	m.set(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *accessPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m accessPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var p store.AccessPolicy
	err := r.client.Do(ctx, http.MethodGet, "/policies/"+url.PathEscape(m.ID.ValueString()), nil, &p)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the access policy", err)
		return
	}
	m.set(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *accessPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m accessPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var p store.AccessPolicy
	if err := r.client.Do(ctx, http.MethodPut, "/policies/"+url.PathEscape(m.ID.ValueString()), m.input(ctx), &p); err != nil {
		apiFailed(&resp.Diagnostics, "update the access policy", err)
		return
	}
	m.set(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *accessPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m accessPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, "/policies/"+url.PathEscape(m.ID.ValueString()), nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "delete the access policy", err)
	}
}

func (r *accessPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

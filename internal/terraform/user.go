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

type userResource struct {
	withClient
}

type userModel struct {
	ID         types.String `tfsdk:"id"`
	Email      types.String `tfsdk:"email"`
	Name       types.String `tfsdk:"name"`
	Title      types.String `tfsdk:"title"`
	Department types.String `tfsdk:"department"`
	Location   types.String `tfsdk:"location"`
	ManagerID  types.String `tfsdk:"manager_id"`
	Status     types.String `tfsdk:"status"`
}

type userInput struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	Department string `json:"department"`
	Location   string `json:"location"`
	ManagerID  string `json:"managerId"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optional := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A person in the directory. Creating one invites them. Halo has no way to delete a person, so destroying this resource suspends the account; inviting the same email again then conflicts until the account is restored and imported.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"email":      schema.StringAttribute{Required: true},
			"name":       schema.StringAttribute{Required: true},
			"title":      optional(""),
			"department": optional(""),
			"location":   optional(""),
			"manager_id": optional("Id of the person they report to."),
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "`invited`, `active`, `suspended` or `deprovisioned`.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (m *userModel) input() userInput {
	return userInput{Email: m.Email.ValueString(), Name: m.Name.ValueString(), Title: m.Title.ValueString(), Department: m.Department.ValueString(), Location: m.Location.ValueString(), ManagerID: m.ManagerID.ValueString()}
}

func (m *userModel) set(u store.User) {
	m.ID = types.StringValue(u.ID)
	m.Email = types.StringValue(u.Email)
	m.Name = types.StringValue(u.Name)
	m.Title = types.StringValue(u.Title)
	m.Department = types.StringValue(u.Department)
	m.Location = types.StringValue(u.Location)
	m.ManagerID = types.StringValue("")
	if u.ManagerID != nil {
		m.ManagerID = types.StringValue(*u.ManagerID)
	}
	m.Status = types.StringValue(u.Status)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		User store.User `json:"user"`
	}
	if err := r.client.Do(ctx, http.MethodPost, "/users", m.input(), &out); err != nil {
		apiFailed(&resp.Diagnostics, "invite the person", err)
		return
	}
	m.set(out.User)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var u store.User
	err := r.client.Do(ctx, http.MethodGet, "/users/"+url.PathEscape(m.ID.ValueString()), nil, &u)
	if notFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiFailed(&resp.Diagnostics, "read the person", err)
		return
	}
	m.set(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var u store.User
	if err := r.client.Do(ctx, http.MethodPatch, "/users/"+url.PathEscape(m.ID.ValueString()), m.input(), &u); err != nil {
		apiFailed(&resp.Diagnostics, "update the person", err)
		return
	}
	m.set(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodPost, "/users/"+url.PathEscape(m.ID.ValueString())+"/suspend", nil, nil)
	if err != nil && !notFound(err) {
		apiFailed(&resp.Diagnostics, "suspend the person", err)
		return
	}
	resp.Diagnostics.AddWarning("Account suspended, not deleted", "Halo keeps "+m.Email.ValueString()+" as a suspended account. Restore it in the console and import it to manage it again.")
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

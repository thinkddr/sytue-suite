// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/client"
)

var (
	_ resource.Resource                = &domainResource{}
	_ resource.ResourceWithConfigure   = &domainResource{}
	_ resource.ResourceWithImportState = &domainResource{}
)

func NewDomainResource() resource.Resource { return &domainResource{} }

type domainResource struct{ c *client.Client }

type domainModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Hostname    types.String `tfsdk:"hostname"`
	Status      types.String `tfsdk:"status"`
	VerifyToken types.String `tfsdk:"verify_token"`
}

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A custom domain attached to a Sytue Cloud project. The API has no update endpoint, so any change replaces the domain.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Domain UUID.",
			},
			"project_id":   schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Owning project UUID."},
			"hostname":     schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Custom hostname, e.g. app.example.com."},
			"status":       schema.StringAttribute{Computed: true, Description: "pending | verified | active | failed."},
			"verify_token": schema.StringAttribute{Computed: true, Sensitive: true, Description: "DNS TXT verification token."},
		},
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.CreateDomain(ctx, m.ProjectID.ValueString(), m.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Create domain failed", err.Error())
		return
	}
	m.ID = types.StringValue(out.ID)
	m.Status = types.StringValue(out.Status)
	m.VerifyToken = types.StringValue(out.VerifyToken)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// No single GET: list the project's domains and match by id.
	list, err := r.c.ListDomains(ctx, m.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read domain failed", err.Error())
		return
	}
	for _, d := range list {
		if d.ID != m.ID.ValueString() {
			continue
		}
		m.Hostname = types.StringValue(d.Hostname)
		m.Status = types.StringValue(d.Status)
		m.VerifyToken = types.StringValue(d.VerifyToken)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

// Update is unreachable (every attribute RequiresReplace) but the interface
// demands it.
func (r *domainResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteDomain(ctx, m.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete domain failed", err.Error())
	}
}

// ImportState accepts "<project_id>/<domain_id>" since list is keyed by project.
func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pid, did, ok := strings.Cut(req.ID, "/")
	if !ok || pid == "" || did == "" {
		resp.Diagnostics.AddError("Invalid import ID", `expected "<project_id>/<domain_id>"`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, importPath("project_id"), pid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, importPath("id"), did)...)
}

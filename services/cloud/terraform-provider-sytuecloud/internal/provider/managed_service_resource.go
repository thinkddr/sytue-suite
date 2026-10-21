// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/client"
)

var (
	_ resource.Resource                = &managedServiceResource{}
	_ resource.ResourceWithConfigure   = &managedServiceResource{}
	_ resource.ResourceWithImportState = &managedServiceResource{}
)

func NewManagedServiceResource() resource.Resource { return &managedServiceResource{} }

type managedServiceResource struct{ c *client.Client }

type managedServiceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Kind      types.String `tfsdk:"kind"`
	Name      types.String `tfsdk:"name"`
	Status    types.String `tfsdk:"status"`
	Config    types.Map    `tfsdk:"config"`
}

func (r *managedServiceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_managed_service"
}

func (r *managedServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A managed backing service (postgres/storage/functions/cron/job/auth). The API has no update endpoint, so any change replaces the service.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Service UUID.",
			},
			"project_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Owning project UUID."},
			"kind":       schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "postgres | storage | functions | cron | job | auth."},
			"name":       schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: replace, Description: "Service name."},
			"status":     schema.StringAttribute{Computed: true, Description: "provisioning | ready | failed | deleting."},
			"config": schema.MapAttribute{
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
				Description:   "Non-sensitive config (string→string), e.g. cron schedule/image.",
			},
		},
	}
}

func (r *managedServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *managedServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m managedServiceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg := map[string]string{}
	resp.Diagnostics.Append(m.Config.ElementsAs(ctx, &cfg, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.CreateService(ctx, m.ProjectID.ValueString(), client.ManagedService{
		Kind:   m.Kind.ValueString(),
		Name:   m.Name.ValueString(),
		Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Create service failed", err.Error())
		return
	}
	r.apply(ctx, &m, out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *managedServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m managedServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.GetService(ctx, m.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read service failed", err.Error())
		return
	}
	r.apply(ctx, &m, out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update is unreachable (every writable attribute RequiresReplace).
func (r *managedServiceResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *managedServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m managedServiceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteService(ctx, m.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete service failed", err.Error())
	}
}

func (r *managedServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// apply copies API fields into the model. config is left as planned unless the
// API returned one (it echoes the non-sensitive config back).
func (r *managedServiceResource) apply(ctx context.Context, m *managedServiceModel, s *client.ManagedService, diags *diag.Diagnostics) {
	m.ID = types.StringValue(s.ID)
	m.ProjectID = types.StringValue(s.ProjectID)
	m.Kind = types.StringValue(s.Kind)
	m.Name = types.StringValue(s.Name)
	m.Status = types.StringValue(s.Status)
	if s.Config != nil {
		cfg, d := types.MapValueFrom(ctx, types.StringType, s.Config)
		diags.Append(d...)
		m.Config = cfg
	}
}

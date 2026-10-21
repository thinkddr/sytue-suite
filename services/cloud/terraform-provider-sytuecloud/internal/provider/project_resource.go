// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/client"
)

var (
	_ resource.Resource                = &projectResource{}
	_ resource.ResourceWithConfigure   = &projectResource{}
	_ resource.ResourceWithImportState = &projectResource{}
)

func NewProjectResource() resource.Resource { return &projectResource{} }

type projectResource struct{ c *client.Client }

// projectModel mirrors client.Project. Slug and git_provider are immutable on
// the API (absent from UpdateProject) → RequiresReplace.
type projectModel struct {
	ID             types.String `tfsdk:"id"`
	Slug           types.String `tfsdk:"slug"`
	Name           types.String `tfsdk:"name"`
	RepoURL        types.String `tfsdk:"repo_url"`
	GitProvider    types.String `tfsdk:"git_provider"`
	ProductionRef  types.String `tfsdk:"production_ref"`
	Framework      types.String `tfsdk:"framework"`
	BuildStrategy  types.String `tfsdk:"build_strategy"`
	RootDir        types.String `tfsdk:"root_dir"`
	BuildCommand   types.String `tfsdk:"build_command"`
	OutputDir      types.String `tfsdk:"output_dir"`
	InstallCommand types.String `tfsdk:"install_command"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	optComputed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Description: desc}
	}
	resp.Schema = schema.Schema{
		Description: "A Sytue Cloud project (a git repo connected for build & deploy).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Project UUID.",
			},
			"slug": schema.StringAttribute{
				Required:      true,
				PlanModifiers: replace,
				Description:   "Unique project slug (immutable).",
			},
			"git_provider": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: replace,
				Description:   "git | github | gitlab (immutable).",
			},
			"name":            optComputed("Human-readable name."),
			"repo_url":        optComputed("Git repository URL."),
			"production_ref":  optComputed("Production branch/ref."),
			"framework":       optComputed("auto | astro | static | node | go | docker."),
			"build_strategy":  optComputed("'' (auto) | dockerfile | buildpacks."),
			"root_dir":        optComputed("Monorepo sub-directory."),
			"build_command":   optComputed("Override build command."),
			"output_dir":      optComputed("Static output directory."),
			"install_command": optComputed("Override install command."),
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (m projectModel) toAPI() client.Project {
	return client.Project{
		ID:             m.ID.ValueString(),
		Slug:           m.Slug.ValueString(),
		Name:           m.Name.ValueString(),
		RepoURL:        m.RepoURL.ValueString(),
		GitProvider:    m.GitProvider.ValueString(),
		ProductionRef:  m.ProductionRef.ValueString(),
		Framework:      m.Framework.ValueString(),
		BuildStrategy:  m.BuildStrategy.ValueString(),
		RootDir:        m.RootDir.ValueString(),
		BuildCommand:   m.BuildCommand.ValueString(),
		OutputDir:      m.OutputDir.ValueString(),
		InstallCommand: m.InstallCommand.ValueString(),
	}
}

func (m *projectModel) fromAPI(p *client.Project) {
	m.ID = types.StringValue(p.ID)
	m.Slug = types.StringValue(p.Slug)
	m.Name = types.StringValue(p.Name)
	m.RepoURL = types.StringValue(p.RepoURL)
	m.GitProvider = types.StringValue(p.GitProvider)
	m.ProductionRef = types.StringValue(p.ProductionRef)
	m.Framework = types.StringValue(p.Framework)
	m.BuildStrategy = types.StringValue(p.BuildStrategy)
	m.RootDir = types.StringValue(p.RootDir)
	m.BuildCommand = types.StringValue(p.BuildCommand)
	m.OutputDir = types.StringValue(p.OutputDir)
	m.InstallCommand = types.StringValue(p.InstallCommand)
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.CreateProject(ctx, m.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Create project failed", err.Error())
		return
	}
	m.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.GetProject(ctx, m.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read project failed", err.Error())
		return
	}
	m.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.c.UpdateProject(ctx, m.ID.ValueString(), m.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Update project failed", err.Error())
		return
	}
	m.fromAPI(out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteProject(ctx, m.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Delete project failed", err.Error())
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// configureClient extracts the *client.Client the provider stashed in
// ProviderData; shared by every resource/data source.
func configureClient(data any, diags interface {
	AddError(summary, detail string)
},
) *client.Client {
	if data == nil {
		return nil // provider not yet configured (early Configure call)
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", data))
		return nil
	}
	return c
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/client"
)

var (
	_ datasource.DataSource              = &projectDataSource{}
	_ datasource.DataSourceWithConfigure = &projectDataSource{}
)

func NewProjectDataSource() datasource.DataSource { return &projectDataSource{} }

type projectDataSource struct{ c *client.Client }

// projectDSModel: look up by `id` or by `name`; the rest are computed.
type projectDSModel struct {
	ID            types.String `tfsdk:"id"`
	Slug          types.String `tfsdk:"slug"`
	Name          types.String `tfsdk:"name"`
	RepoURL       types.String `tfsdk:"repo_url"`
	GitProvider   types.String `tfsdk:"git_provider"`
	ProductionRef types.String `tfsdk:"production_ref"`
	Framework     types.String `tfsdk:"framework"`
}

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an existing Sytue Cloud project by id or name.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Optional: true, Computed: true, Description: "Project UUID (one of id/name required)."},
			"name":           schema.StringAttribute{Optional: true, Computed: true, Description: "Project name (one of id/name required)."},
			"slug":           schema.StringAttribute{Computed: true},
			"repo_url":       schema.StringAttribute{Computed: true},
			"git_provider":   schema.StringAttribute{Computed: true},
			"production_ref": schema.StringAttribute{Computed: true},
			"framework":      schema.StringAttribute{Computed: true},
		},
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m projectDSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var p *client.Project
	switch {
	case !m.ID.IsNull():
		got, err := d.c.GetProject(ctx, m.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Lookup project by id failed", err.Error())
			return
		}
		p = got
	case !m.Name.IsNull():
		list, err := d.c.ListProjects(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Lookup project by name failed", err.Error())
			return
		}
		for i := range list {
			if list[i].Name == m.Name.ValueString() {
				p = &list[i]
				break
			}
		}
		if p == nil {
			resp.Diagnostics.AddError("Project not found", fmt.Sprintf("no project named %q", m.Name.ValueString()))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing lookup key", "set either `id` or `name`.")
		return
	}

	m.ID = types.StringValue(p.ID)
	m.Slug = types.StringValue(p.Slug)
	m.Name = types.StringValue(p.Name)
	m.RepoURL = types.StringValue(p.RepoURL)
	m.GitProvider = types.StringValue(p.GitProvider)
	m.ProductionRef = types.StringValue(p.ProductionRef)
	m.Framework = types.StringValue(p.Framework)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

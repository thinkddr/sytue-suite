// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package provider implements the terraform-plugin-framework provider for the
// Sytue Cloud control plane.
package provider

import (
	"context"
	"errors"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/client"
)

// isNotFound reports whether err is the client's 404 sentinel.
func isNotFound(err error) bool { return errors.Is(err, client.ErrNotFound) }

// importPath is a shorthand for path.Root used by ImportState handlers.
func importPath(attr string) path.Path { return path.Root(attr) }

const defaultEndpoint = "https://cloud-api.sytue.eu"

type sytueProvider struct {
	version string
}

// New returns the provider factory for main.go / tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &sytueProvider{version: version} }
}

func (p *sytueProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "sytuecloud"
	resp.Version = p.version
}

func (p *sytueProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Sytue Cloud (PaaS control plane) resources via IaC.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "API base URL. Defaults to $SYTUE_CLOUD_API or " + defaultEndpoint + ".",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Bearer token (scl_... API token or Sytue ID JWT). Defaults to $SYTUE_CLOUD_TOKEN.",
			},
		},
	}
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
}

func (p *sytueProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := os.Getenv("SYTUE_CLOUD_API")
	if !cfg.Endpoint.IsNull() {
		endpoint = cfg.Endpoint.ValueString()
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	token := os.Getenv("SYTUE_CLOUD_TOKEN")
	if !cfg.Token.IsNull() {
		token = cfg.Token.ValueString()
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing API token",
			"Set the provider `token` attribute or the SYTUE_CLOUD_TOKEN environment variable.",
		)
		return
	}

	c := client.New(endpoint, token)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *sytueProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewEnvVarResource,
		NewDomainResource,
		NewManagedServiceResource,
	}
}

func (p *sytueProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
	}
}

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
	_ resource.Resource                = &envVarResource{}
	_ resource.ResourceWithConfigure   = &envVarResource{}
	_ resource.ResourceWithImportState = &envVarResource{}
)

func NewEnvVarResource() resource.Resource { return &envVarResource{} }

type envVarResource struct{ c *client.Client }

type envVarModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Key       types.String `tfsdk:"key"`
	Value     types.String `tfsdk:"value"`
	IsSecret  types.Bool   `tfsdk:"is_secret"`
	Scope     types.String `tfsdk:"scope"`
}

func (r *envVarResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_env_var"
}

func (r *envVarResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A single environment variable/secret on a Sytue Cloud project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Description:   "Env var UUID.",
			},
			"project_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Owning project UUID."},
			"key":        schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Variable name (immutable; change = replace)."},
			"value":      schema.StringAttribute{Required: true, Sensitive: true, Description: "Value. Not reconciled on read for secrets (API masks them)."},
			"is_secret":  schema.BoolAttribute{Optional: true, Computed: true, Description: "Store as an encrypted secret (value masked in API reads)."},
			"scope":      schema.StringAttribute{Optional: true, Computed: true, Description: "all | production | preview."},
		},
	}
}

func (r *envVarResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *envVarResource) put(ctx context.Context, m *envVarModel) error {
	out, err := r.c.PutEnv(ctx, m.ProjectID.ValueString(), client.EnvVar{
		Key:      m.Key.ValueString(),
		Value:    m.Value.ValueString(),
		IsSecret: m.IsSecret.ValueBool(),
		Scope:    m.Scope.ValueString(),
	})
	if err != nil {
		return err
	}
	m.ID = types.StringValue(out.ID)
	m.IsSecret = types.BoolValue(out.IsSecret)
	m.Scope = types.StringValue(out.Scope)
	return nil
}

func (r *envVarResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m envVarModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &m); err != nil {
		resp.Diagnostics.AddError("Create env var failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *envVarResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m envVarModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// No single GET for env vars: list the project's set and match by key.
	list, err := r.c.ListEnv(ctx, m.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read env var failed", err.Error())
		return
	}
	for _, e := range list {
		if e.Key != m.Key.ValueString() {
			continue
		}
		m.ID = types.StringValue(e.ID)
		m.IsSecret = types.BoolValue(e.IsSecret)
		m.Scope = types.StringValue(e.Scope)
		// ponytail: the API masks secret values, so only reconcile the value
		// for non-secret vars; secrets keep the state value. Drift on a secret's
		// value is invisible by design — upgrade path: a dedicated compare
		// endpoint if the API ever exposes a value hash.
		if !e.IsSecret {
			m.Value = types.StringValue(e.Value)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	resp.State.RemoveResource(ctx) // key gone → drift
}

func (r *envVarResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m envVarModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &m); err != nil { // PUT is upsert → same call as create
		resp.Diagnostics.AddError("Update env var failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *envVarResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m envVarModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteEnv(ctx, m.ID.ValueString()); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Delete env var failed", err.Error())
	}
}

// ImportState accepts "<project_id>/<key>" since delete needs the env id but
// list is keyed by project; Read resolves the id from the key.
func (r *envVarResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pid, key, ok := strings.Cut(req.ID, "/")
	if !ok || pid == "" || key == "" {
		resp.Diagnostics.AddError("Invalid import ID", `expected "<project_id>/<key>"`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, importPath("project_id"), pid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, importPath("key"), key)...)
}

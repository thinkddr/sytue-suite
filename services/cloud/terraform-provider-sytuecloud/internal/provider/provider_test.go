// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package provider

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestResourceSchemas validates every resource/data source schema WITHOUT any
// network: it fetches the schema via the framework and asserts no diagnostics.
// A malformed attribute (bad plan modifier, wrong type) fails here.
func TestResourceSchemas(t *testing.T) {
	ctx := context.Background()
	p := &sytueProvider{}

	for _, f := range p.Resources(ctx) {
		var resp fwresource.SchemaResponse
		f().Schema(ctx, fwresource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("resource schema errors: %v", resp.Diagnostics.Errors())
		}
		if _, ok := resp.Schema.Attributes["id"]; !ok {
			t.Errorf("resource schema missing computed id")
		}
	}

	for _, f := range p.DataSources(ctx) {
		var resp datasource.SchemaResponse
		f().Schema(ctx, datasource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("data source schema errors: %v", resp.Diagnostics.Errors())
		}
	}
}

// --- Acceptance test (gated by TF_ACC + a live API) ---
//
// ponytail: end-to-end verification against a live control plane is DEFERRED.
// resource.Test only runs when TF_ACC is set (Terraform's standard gate);
// testAccPreCheck additionally skips it unless SYTUE_CLOUD_TOKEN points at a
// reachable API. It is the scaffold for the real CI acceptance run once a
// disposable test tenant exists.

var testAccProtoV6 = map[string]func() (tfprotov6.ProviderServer, error){
	"sytuecloud": providerserver.NewProtocol6WithError(New("acc")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("SYTUE_CLOUD_TOKEN") == "" {
		t.Skip("SYTUE_CLOUD_TOKEN not set; skipping live acceptance test")
	}
}

func TestAccProjectResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: `
resource "sytuecloud_project" "test" {
  slug         = "tf-acc-blog"
  name         = "TF Acc Blog"
  git_provider = "github"
  repo_url     = "https://github.com/sytue/example"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("sytuecloud_project.test", "id"),
					resource.TestCheckResourceAttr("sytuecloud_project.test", "slug", "tf-acc-blog"),
				),
			},
		},
	})
}

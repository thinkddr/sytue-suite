// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command terraform-provider-sytuecloud serves the Terraform provider for the
// Sytue Cloud control plane.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"git.sytue.eu/sytue/one/services/cloud/terraform-provider-sytuecloud/internal/provider"
)

// version is set via -ldflags at release time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/sytue/sytuecloud",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

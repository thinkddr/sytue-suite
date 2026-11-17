// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var corePolicies = webport.NewPolicies()

func init() {
	corePortRevocations = append(corePortRevocations, corePolicies.RevokePort)
	asyncDomainOperations["core.publication.authorize"] = publicationAuthorize
}

func publicationAuthorize(ctx context.Context, req js.Value) (any, error) {
	return corePolicies.AuthorizeJS(ctx, req)
}

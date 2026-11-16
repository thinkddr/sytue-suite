// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"context"
	"syscall/js"
)

func init() { asyncDomainOperations["core.source.finalize"] = serviceSourceFinalize }
func serviceSourceFinalize(ctx context.Context, req js.Value) (any, error) {
	return coreSources.FinalizeJS(ctx, req)
}

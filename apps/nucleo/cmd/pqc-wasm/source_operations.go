// SPDX-License-Identifier: Apache-2.0
//go:build js && wasm

package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

var coreSources = webport.NewSources()

func init() { asyncDomainOperations["core.source.read"] = sourceRead }

func sourceRead(ctx context.Context, req js.Value) (any, error) { return coreSources.ReadJS(ctx, req) }

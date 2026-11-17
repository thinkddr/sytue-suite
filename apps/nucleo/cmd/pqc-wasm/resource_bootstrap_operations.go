//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

func init() {
	domainOperations["resource.bootstrap.context"] = func(req js.Value) (any, error) {
		raw, err := bytesArg(req.Get("context"), 16<<10)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
		value, err := resource.DecodeBootstrapContext(raw)
		if err != nil {
			return nil, err
		}
		canonical, err := resource.BootstrapContextBytes(value)
		if err != nil {
			return nil, err
		}
		defer clear(canonical)
		hash, err := resource.BootstrapContextSHA256(value)
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": bytesValue(canonical), "sha256": hash}, nil
	}
}

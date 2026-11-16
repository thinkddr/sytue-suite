//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func init() { domainOperations["storage.identity.plan"] = storageIdentityPlan }

// Public routing only; this cannot grant a key, role or verified completion.
func storageIdentityPlan(req js.Value) (any, error) {
	raw, err := bytesArg(req.Get("state"), 64<<10)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var state storage.IdentityCutoverState
	if json.Unmarshal(raw, &state) != nil {
		return nil, storage.ErrRecord
	}
	requireLegacy, err := state.RequiresLegacy(scopeArg(req))
	if err != nil {
		return nil, err
	}
	return map[string]any{"phase": state.Phase, "require_legacy": requireLegacy}, nil
}

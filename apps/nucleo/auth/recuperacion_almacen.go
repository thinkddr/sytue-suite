// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"encoding/hex"
	"strings"
)

// PrefijoRecuperacionAlmacen is exclusively for validated SSSS recovery scoped
// to one account and persisted login revision. No Matrix device credentials,
// database keys, widget data or FileProvider tokens use the shared group.
const PrefijoRecuperacionAlmacen = "rooms-matrix-recovery-shared-"

func cuentaRecuperacionAlmacen(cuenta string) bool {
	suffix, ok := strings.CutPrefix(cuenta, PrefijoRecuperacionAlmacen)
	if !ok || len(suffix) != 97 || suffix[64] != '-' {
		return false
	}
	for _, part := range []string{suffix[:64], suffix[65:]} {
		raw, err := hex.DecodeString(part)
		if err != nil || hex.EncodeToString(raw) != part {
			return false
		}
	}
	return true
}

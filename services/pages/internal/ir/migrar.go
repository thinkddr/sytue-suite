// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ir

import (
	"encoding/json"
	"fmt"
)

// Migrar aplica migradores paso a paso desde la versión de raw hasta VersionIR.
func Migrar(raw []byte) ([]byte, error) {
	var ver struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &ver); err != nil {
		return nil, fmt.Errorf("ir.Migrar: json inválido: %w", err)
	}
	if ver.Version == VersionIR {
		return raw, nil
	}
	if ver.Version < 1 {
		return nil, fmt.Errorf("ir.Migrar: versión %d no soportada", ver.Version)
	}
	if ver.Version > VersionIR {
		return nil, fmt.Errorf("ir.Migrar: versión %d más nueva que VersionIR (%d)", ver.Version, VersionIR)
	}
	return raw, nil
}

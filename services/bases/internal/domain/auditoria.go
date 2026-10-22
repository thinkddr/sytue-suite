// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strconv"
)

// MaxValorAuditado es el tope de bytes de un antes o un después en la traza.
const MaxValorAuditado = 1024

// Acotar devuelve el valor para la traza: tal cual si cabe y, si no, un marcador
// con su tamaño. La traza prueba QUÉ cambió, no guarda copias enteras de los datos.
func Acotar(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return nil
	}
	if len(v) <= MaxValorAuditado {
		return v
	}
	return json.RawMessage(`{"truncated":true,"bytes":` + strconv.Itoa(len(v)) + `}`)
}

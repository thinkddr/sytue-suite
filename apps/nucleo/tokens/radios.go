// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed spacing.json
var espaciadoJSON []byte

var radios = func() map[string]string {
	var hoja struct {
		Radios map[string]string `json:"radii"`
	}
	if err := json.Unmarshal(espaciadoJSON, &hoja); err != nil {
		panic("tokens: spacing.json: " + err.Error())
	}
	return hoja.Radios
}()

// Radio devuelve una esquina del canon en px de CSS: "sm", "md", "lg", "xl",
// "2xl" o "pill". El radio relativo "full" depende de la caja y no es una medida.
func Radio(nombre string) float32 {
	valor, ok := radios[nombre]
	if !ok || !strings.HasSuffix(valor, "px") {
		panic("tokens: no existe un radio en px llamado " + nombre)
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(valor, "px"), 32)
	if err != nil || n < 0 {
		panic("tokens: radio inválido " + nombre + ": " + valor)
	}
	return float32(n)
}

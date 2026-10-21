// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package redsys

import "testing"

func TestFirmaRedsysEsVerificableYNoAdmiteCambios(t *testing.T) {
	const (
		clave  = "sq7HjrUOBfKmC576ILgskD5srU870gJ7"
		orden  = "00014338z744"
		params = "eyJEU19NRVJjaGFudF9PcmRlciI6IjAwMDE0MzM4ejc0NCJ9"
	)
	firma, err := Firmar(params, orden, clave)
	if err != nil || !Verificar(params, orden, clave, firma) {
		t.Fatalf("firma válida: %q, %v", firma, err)
	}
	if Verificar(params+"A", orden, clave, firma) || Verificar(params, orden+"x", clave, firma) {
		t.Fatal("una variación verificó")
	}
}

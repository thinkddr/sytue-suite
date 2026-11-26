// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"reflect"
	"testing"
)

// TestLaTablaDeRoomsCuadra: los textos de «Cuentas de mensajería» están en los 26 idiomas
// (C-12), y la tabla tiene exactamente un texto por campo de `textos`.
func TestLaTablaDeRoomsCuadra(t *testing.T) {
	for _, f := range tabla.Faltan() {
		t.Error(f)
	}
	tipo := reflect.TypeOf(textos{})
	if tipo.NumField() != len(tablaTextos) {
		t.Errorf("textos tiene %d campos y la tabla %d claves", tipo.NumField(), len(tablaTextos))
	}
	for _, idioma := range []string{"es", "en", "de", "fr", "pt-BR"} {
		v := reflect.ValueOf(textosDe(idioma))
		for i := range tipo.NumField() {
			campo := tipo.Field(i).Name
			if _, ok := tablaTextos[campo]; !ok {
				t.Errorf("el campo %q no está en la tabla", campo)
			}
			if s := v.Field(i).String(); s == "" || s == campo {
				t.Errorf("%s: el campo %q sale %q", idioma, campo, s)
			}
		}
	}
	if textosDe("de").titulo != "Messaging-Konten" || textosDe("es").titulo != "Cuentas de mensajería" || textosDe("xx").titulo != "Messaging accounts" {
		t.Error("idioma")
	}
}

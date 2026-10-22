// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import "testing"

func TestNIF(t *testing.T) {
	validos := []string{
		"12345678Z",    // DNI
		"12.345.678-z", // con puntos, guion y en minúsculas
		"ES 12345678Z", // NIF-IVA
		"X1234567L",    // NIE
		"Y1234567X",    // NIE
		"Z1234567R",    // NIE
		"K1234567L",    // NIF especial de persona física
		"B12345674",    // sociedad limitada: dígito
		"A58818501",    // sociedad anónima: dígito
		"Q2826000H",    // organismo público: letra
		"P2807900B",    // entidad local: letra
		"G12345674",    // asociación: dígito…
		"G1234567D",    // …o letra
		"ESB12345674",
	}
	for _, s := range validos {
		if !NIF(s) {
			t.Errorf("NIF(%q) = false, es válido", s)
		}
	}
	invalidos := []string{
		"", "12345678", "12345678A", "123456789Z", "1234567Z", "X1234567A", "W1234567",
		"B12345675",   // control equivocado
		"B1234567D",   // una S.L. no lleva letra
		"Q28260008",   // un organismo no lleva dígito
		"I12345674",   // letra de forma jurídica que no existe
		"FR12345678Z", // NIF-IVA de otro país
		"12345678Ñ", "X1234567", "XX1234567L", "K1234567A",
	}
	for _, s := range invalidos {
		if NIF(s) {
			t.Errorf("NIF(%q) = true, no es válido", s)
		}
	}
}

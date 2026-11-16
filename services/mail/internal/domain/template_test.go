// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizarTemplate(t *testing.T) {
	name, body, err := NormalizarTemplate("  Horario  ", "Abrimos de 9 a 14.\n\n")
	if err != nil || name != "Horario" || body != "Abrimos de 9 a 14.\n\n" {
		t.Fatalf("= %q, %q, %v; quiero el nombre recortado y el cuerpo intacto", name, body, err)
	}
	// Un cuerpo vacío vale: la plantilla puede ser solo un saludo que se rellena luego.
	if _, _, err := NormalizarTemplate("Vacía", ""); err != nil {
		t.Fatalf("cuerpo vacío: %v", err)
	}
	// El nombre cuenta caracteres, no bytes: cien «ñ» caben.
	if _, _, err := NormalizarTemplate(strings.Repeat("ñ", TemplateNameMax), "x"); err != nil {
		t.Fatalf("cien caracteres de dos bytes: %v", err)
	}
	if _, _, err := NormalizarTemplate("ok", strings.Repeat("x", TemplateBodyMaxBytes)); err != nil {
		t.Fatalf("cuerpo en el tope: %v", err)
	}
	for _, c := range []struct{ nombre, cuerpo string }{
		{"", "x"},
		{"   ", "x"},
		{strings.Repeat("a", TemplateNameMax+1), "x"},
		{"dos\nlíneas", "x"},
		{"ok", strings.Repeat("x", TemplateBodyMaxBytes+1)},
		{"ok", "con\x00nulo"},
		{"ok", "\xff\xfe"},
	} {
		if _, _, err := NormalizarTemplate(c.nombre, c.cuerpo); !errors.Is(err, ErrInvalidTemplate) {
			t.Fatalf("%q / %d bytes: err = %v, quiero ErrInvalidTemplate", c.nombre, len(c.cuerpo), err)
		}
	}
}

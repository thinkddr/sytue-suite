// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2"
)

// TestSoltarTextoEnElRedactor: con el redactor abierto, el texto soltado sobre el cuerpo
// entra en el cuerpo, y el soltado sobre el asunto, en el asunto; sin redactor no entra
// en ninguna parte.
func TestSoltarTextoEnElRedactor(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	if b.Soltar("nada", fyne.NewPos(600, 400)) {
		t.Fatal("sin redactor, lo soltado ha entrado en algún sitio")
	}
	b.abrirRedactor(Borrador{Para: "grace@example.org"})
	r := b.redaccion.r
	d := fyne.CurrentApp().Driver()
	zona := r.rico.Zona().(fyne.CanvasObject)
	if !b.Soltar("Hola, Grace", d.AbsolutePositionForObject(zona).AddXY(5, 5)) || r.B.Cuerpo != "Hola, Grace" {
		t.Errorf("el cuerpo queda %q", r.B.Cuerpo)
	}
	if !b.Soltar("Plano", d.AbsolutePositionForObject(r.asunto).AddXY(5, 5)) || r.asunto.Texto() != "Plano" {
		t.Errorf("el asunto queda %q", r.asunto.Texto())
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaGLlevaAUnaFecha es `IrAFecha.test.tsx`: g abre «Ir a una fecha» con el día que
// se ve, «Ir» lleva al elegido sin cambiar de vista, y con una fecha que no existe el
// botón se apaga y no se mueve nada.
func TestLaGLlevaAUnaFecha(t *testing.T) {
	a, w := appConVentana(t, &buscador{})
	vista := a.P.Vista
	if !a.Atajo('G') {
		t.Fatal("«g» no es un atajo")
	}
	capa := w.Canvas().Overlays().Top()
	if capa == nil {
		t.Fatal("«g» no abrió el diálogo")
	}
	campo, _ := buscarObjeto(capa, func(o fyne.CanvasObject) bool { _, ok := o.(*ui.Entrada); return ok }).(*ui.Entrada)
	if campo == nil {
		t.Fatal("el diálogo no tiene campo de fecha")
	}
	if campo.Texto() != "21/09/2026" {
		t.Errorf("el campo arranca en %q y no en el día que se ve", campo.Texto())
	}
	guardar(t, w.Canvas().Capture(), "ir-a-una-fecha")
	ir := buscarBoton(capa, "Ir")
	if ir == nil {
		t.Fatal("el diálogo no tiene el botón «Ir»")
	}

	campo.PonerTexto("31/02/2027")
	if !ir.Inerte {
		t.Error("con el 31 de febrero «Ir» sigue encendido")
	}
	test.Tap(ir)
	if !a.P.Cursor.Equal(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)) || w.Canvas().Overlays().Top() == nil {
		t.Fatalf("una fecha que no existe movió el cursor a %v o cerró el diálogo", a.P.Cursor)
	}

	campo.PonerTexto("04/07/2031")
	if ir.Inerte {
		t.Error("con una fecha válida «Ir» sigue apagado")
	}
	test.Tap(ir)
	if got := a.P.Cursor; got.Year() != 2031 || got.Month() != time.July || got.Day() != 4 {
		t.Errorf("«Ir» llevó a %v y no al 4 de julio de 2031", got)
	}
	if a.P.Vista != vista {
		t.Errorf("ir a una fecha cambió la vista de %v a %v", vista, a.P.Vista)
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Error("el diálogo sigue abierto tras ir")
	}
}

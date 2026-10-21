// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// buscarArea devuelve la primera área de texto de un árbol de objetos.
func buscarArea(o fyne.CanvasObject) *ui.AreaTexto {
	switch x := o.(type) {
	case *ui.AreaTexto:
		return x
	case *fyne.Container:
		for _, h := range x.Objects {
			if a := buscarArea(h); a != nil {
				return a
			}
		}
	}
	return nil
}

// TestLaCombinacionDesdeElFolio: el origen pegado se guarda en el folio, un campo va al
// cursor, la vista previa pinta el valor del registro y deja el folio en solo lectura, y
// «Reemplazar documento» deja una copia por registro. El área del origen es monoespaciada
// (`font-mono`, decidido por el dueño). Se vio en ROJO sin `Mono: true`.
func TestLaCombinacionDesdeElFolio(t *testing.T) {
	f := folioCon(t, `<p>Hola</p>`)
	a := buscarArea(container.NewStack(panelOrigen(f.ctx, f, func() {})))
	if a == nil || a.Propia == nil || !a.Propia.Mono {
		t.Fatalf("el área del origen no es monoespaciada: %+v", a)
	}
	f.Mandar(cmdOrigen, `{"fields":["Nombre"],"records":[{"Nombre":"Ana"},{"Nombre":"Luis"}]}`)
	f.mover(punto{0, 4}, false)
	f.Mandar(cmdCampo, "Nombre")
	if got := f.Serializar(); !strings.HasPrefix(got, `<p>Hola<span data-mergefield="Nombre" contenteditable="false">«Nombre»</span></p>`) {
		t.Fatalf("el campo: %s", got)
	}
	f.VistaPrevia(1)
	if f.editable() || f.bloques[0].Tramos[1].Rotulo != "Luis" {
		t.Errorf("la vista previa: editable %v, rótulo %q", f.editable(), f.bloques[0].Tramos[1].Rotulo)
	}
	f.VistaPrevia(-1)
	f.Mandar(cmdReemplazar, "")
	if got := f.Serializar(); strings.Count(got, "data-page-break") != 1 || !strings.Contains(got, "<p>HolaAna</p>") || !strings.Contains(got, "<p>HolaLuis</p>") {
		t.Errorf("reemplazar: %s", got)
	}
}

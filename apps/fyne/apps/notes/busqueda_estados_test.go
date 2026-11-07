// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLaBusquedaDiceEnQueEsta: buscando lo dice con palabras y no solo con la rueda; con
// resultados, cuántos hay encima de la lista; y un fallo no se disfraza de «no se ha
// encontrado nada»: lo dice y ofrece repetir la MISMA búsqueda. Es lo que pinta
// `PanelBusqueda` en la web (pantalla.test.tsx). VISTO EN ROJO con la `lista` de antes:
// ni «Buscando…», ni el recuento, ni «Reintentar».
func TestLaBusquedaDiceEnQueEsta(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	p.vista = VistaBusqueda
	tr := p.tr.T

	p.busqueda.Fase = Cargando
	if textos := pintado(t, p); !slices.Contains(textos, tr("search.searching", nil)) || slices.Contains(textos, tr("search.empty", nil)) {
		t.Fatalf("buscando pinta %q", textos)
	}

	p.busqueda.Fase, p.busqueda.Buscada = Fallo, "plan"
	textos := pintado(t, p)
	if !slices.Contains(textos, tr("search.failed", nil)) || slices.Contains(textos, tr("search.empty", nil)) {
		t.Fatalf("el fallo pinta %q", textos)
	}
	var repetida string
	p.busqueda.AlBuscar = func(q string) { repetida = q }
	recorrerInterno(p.busqueda.lista(p.variante()), func(o fyne.CanvasObject) {
		if b, ok := o.(fyne.Tappable); ok && slices.Contains(textosInternos(o), tr("common.retry", nil)) {
			b.Tapped(&fyne.PointEvent{})
		}
	})
	if repetida != "plan" {
		t.Fatalf("«Reintentar» busca %q", repetida)
	}

	p.busqueda.Fase = Exito
	p.busqueda.Resultados = []api.NoteHit{{ID: "a", NotebookID: "c1", Title: "uno"}, {ID: "b", NotebookID: "c1", Title: "dos"}}
	if textos := pintado(t, p); !slices.Contains(textos, tr("search.results", i18n.Vars{"count": 2})) {
		t.Fatalf("con dos resultados pinta %q", textos)
	}
}

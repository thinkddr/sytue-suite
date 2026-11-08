// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestUnFiltroSinCoincidenciasLoDiceYOfreceQuitarlo: con notas en el cuaderno y un
// filtro que las oculta todas, el lienzo no finge un cuaderno vacío («Este cuaderno está
// vacío» y «Nueva nota»): dice que nada coincide y ofrece quitar el filtro, que vacía el
// texto y el estado. VISTO EN ROJO quitando la rama de `sinCoincidencias` del lienzo.
func TestUnFiltroSinCoincidenciasLoDiceYOfreceQuitarlo(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Title: "compra"},
		api.Note{ID: "b", NotebookID: "c1", Title: "mudanza"})
	p.lienzo.AlCrearPrimera = func() {}
	p.filtro, p.filtroEstado = "zzz", ""
	p.sincronizar()
	if p.lienzo.AlQuitarFiltro == nil || p.arbol.AlQuitarFiltro == nil {
		t.Fatal("con un filtro que lo oculta todo no se ofrece quitarlo")
	}
	textos := textosInternos(p.lienzo.Objeto(tema.Claro))
	if !slices.Contains(textos, p.tr.T("filter.noMatches", nil)) || !slices.Contains(textos, p.tr.T("filter.clear", nil)) {
		t.Fatalf("el lienzo filtrado pinta %q", textos)
	}
	if slices.Contains(textos, p.tr.T("empty.noNotes.title", nil)) || slices.Contains(textos, p.tr.T("note.new", nil)) {
		t.Fatalf("el lienzo filtrado finge un cuaderno vacío: %q", textos)
	}
	arbol := textosInternos(p.arbol.Objeto(tema.Claro))
	if !slices.Contains(arbol, p.tr.T("filter.noMatches", nil)) || slices.Contains(arbol, p.tr.T("empty.noNotes.title", nil)) {
		t.Fatalf("el árbol filtrado pinta %q", arbol)
	}

	p.filtroEstado = "s1"
	p.lienzo.AlQuitarFiltro()
	if p.filtro != "" || p.filtroEstado != "" {
		t.Fatalf("quitar el filtro deja %q / %q", p.filtro, p.filtroEstado)
	}
	p.sincronizar()
	if p.lienzo.AlQuitarFiltro != nil || len(p.lienzo.Notas) != 2 {
		t.Fatalf("sin filtro: %d notas y quitar-filtro %v", len(p.lienzo.Notas), p.lienzo.AlQuitarFiltro != nil)
	}
}

// TestUnCuadernoVacioDeVerdadNoHablaDelFiltro: sin notas, aunque haya algo escrito en el
// filtro, el vacío es el de siempre.
func TestUnCuadernoVacioDeVerdadNoHablaDelFiltro(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	p.filtro = "zzz"
	p.sincronizar()
	if p.lienzo.AlQuitarFiltro != nil {
		t.Fatal("un cuaderno sin notas no está vacío por el filtro")
	}
}

// TestQuitarElFiltroNoSeOfreceSiLasOcultaOtraCosa: con «ocultar completadas» y todas
// hechas, el filtro no es lo que las esconde; ofrecer quitarlo volvería al mismo vacío.
// VISTO EN ROJO contando `p.notas` en vez de las que deja el enfoque.
func TestQuitarElFiltroNoSeOfreceSiLasOcultaOtraCosa(t *testing.T) {
	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Kind: "todo", Checked: true, Title: "compra"})
	p.ocultarTerminadas, p.filtro = true, "zzz"
	p.sincronizar()
	if p.lienzo.AlQuitarFiltro != nil || p.arbol.AlQuitarFiltro != nil {
		t.Fatal("se ofrece quitar un filtro que no es lo que oculta las notas")
	}
}

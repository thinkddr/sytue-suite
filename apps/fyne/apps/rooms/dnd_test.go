// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Un canal arrastrado a una categoría va a ella; a «Favoritos», vuelve a su sección y
// se marca favorito. Con ratón las filas de la casa se arrastran, y una conversación de
// otra organización no.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin la rama de Favoritos en SoltarCanal, el
// canal soltado allí no se marca.
func TestArrastrarUnCanalAOtraSeccion(t *testing.T) {
	s := servicioConDos()
	s.categorias = []api.RoomsCategory{{ID: "k1", Name: "Clientes"}}
	s.fuera = []api.RoomsChannel{{ID: "x1", Name: "proveedor", Kind: "public", External: true}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	var c1, x1 Canal
	for _, c := range l.Canales {
		switch c.ID {
		case "c1":
			c1 = c
		case "x1":
			x1 = c
		}
	}
	if _, ok := l.fila(c1, tema.Claro).(*FilaCanalArrastrable); !ok {
		t.Error("con ratón la fila de un canal de la casa no se arrastra")
	}
	if _, ok := l.fila(x1, tema.Claro).(*FilaCanalArrastrable); ok {
		t.Error("la conversación de otra organización se arrastra")
	}
	var clientes, favoritos Seccion
	for _, x := range AgruparCanales(l.Canales, l.Categorias) {
		switch {
		case x.Categoria != nil:
			clientes = x
		case x.Clave == ClaveCanales:
			favoritos = x
			favoritos.Clave = ClaveFavoritos
		}
	}
	sa.SoltarCanal(c1, clientes)
	if !s.pidio("SetChannelCategory(c1,k1)") {
		t.Fatalf("soltar en «Clientes»: %v", s.llamadas)
	}
	c1.CategoriaID = "k1"
	sa.SoltarCanal(c1, favoritos)
	if !s.pidio("SetChannelCategory(c1,)") || !s.pidio("SetFavorite(c1,sí)") {
		t.Errorf("soltar en «Favoritos»: %v", s.llamadas)
	}
	// El punto de soltar se busca en las secciones pintadas.
	if _, ok := l.SeccionEn(fyne.NewPos(-5, -5)); ok {
		t.Error("fuera del carril hay sección")
	}
}

// dianaDe es la sección pintada de esa categoría.
func dianaDe(t *testing.T, l *Lateral, id string) diana {
	t.Helper()
	for _, x := range l.dianas {
		if x.s.Categoria != nil && x.s.Categoria.ID == id {
			return x
		}
	}
	t.Fatalf("no se ha pintado la categoría %q", id)
	return diana{}
}

// arrastre es un paso de arrastre que empieza en `desde` (relativo a la pieza) y va por
// `abs` en la ventana.
func arrastre(desde, abs fyne.Position) *fyne.DragEvent {
	return &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: desde.AddXY(0, 2), AbsolutePosition: abs}, Dragged: fyne.NewDelta(0, 2)}
}

// Una categoría propia se arrastra por su CABECERA y, soltada sobre otra, se pone en su
// puesto renumerando todas (`reorderCategoryBefore`); mientras, la de debajo lleva su
// aro y ella misma no. Un arrastre que empieza en sus filas no es de la sección. Un
// canal enciende el aro de cualquier sección.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin apagar los aros en
// soltar, «Clientes» se queda marcada después de soltar.
func TestArrastrarUnaCategoria(t *testing.T) {
	s := servicioConDos()
	s.canales[0].CategoryID = "k1"
	s.categorias = []api.RoomsCategory{{ID: "k1", Name: "Clientes", SortOrder: 0}, {ID: "k2", Name: "Zeta", SortOrder: 1}}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	l := sa.P.Lateral
	k1, k2 := dianaDe(t, l, "k1"), dianaDe(t, l, "k2")
	zeta, ok := k2.o.(*ui.SeccionLateralArrastrable)
	if !ok {
		t.Fatalf("la cabecera de una categoría propia no se arrastra: %T", k2.o)
	}
	sobre := func(x diana) fyne.Position {
		return fyne.CurrentApp().Driver().AbsolutePositionForObject(x.o).AddXY(20, 6)
	}

	zeta.Dragged(arrastre(fyne.NewPos(20, 4), sobre(k2)))
	if k2.sec.Diana {
		t.Error("la categoría arrastrada se marca como destino de sí misma")
	}
	zeta.Dragged(arrastre(fyne.NewPos(20, 4), sobre(k1)))
	if !k1.sec.Diana || k2.sec.Diana {
		t.Fatalf("sobre «Clientes»: aro en Clientes %v, en Zeta %v", k1.sec.Diana, k2.sec.Diana)
	}
	zeta.DragEnd()
	if k1.sec.Diana {
		t.Error("al soltar, «Clientes» sigue con su aro")
	}
	if !s.pidio("UpdateCategory(k2,orden=0)") || !s.pidio("UpdateCategory(k1,orden=1)") {
		t.Fatalf("soltar «Zeta» sobre «Clientes»: %v", s.llamadas)
	}

	// Empezar en una fila (por debajo de la cabecera) no arrastra la sección.
	n := len(s.llamadas)
	k1, k2 = dianaDe(t, l, "k1"), dianaDe(t, l, "k2")
	clientes := k1.o.(*ui.SeccionLateralArrastrable)
	clientes.Dragged(arrastre(fyne.NewPos(20, ui.AltoEncabezadoSeccion(k1.sec)+10), sobre(k2)))
	clientes.DragEnd()
	if k2.sec.Diana || len(s.llamadas) != n {
		t.Errorf("un arrastre desde una fila movió la sección: %v", s.llamadas[n:])
	}

	// Un canal arrastrado marca la sección de debajo, sea cual sea.
	var c1 Canal
	for _, c := range l.Canales {
		if c.ID == "c1" {
			c1 = c
		}
	}
	fila := l.fila(c1, tema.Claro).(*FilaCanalArrastrable)
	fila.Dragged(arrastre(fyne.Position{}, sobre(k2)))
	if !k2.sec.Diana {
		t.Error("un canal sobre «Zeta» no la marca")
	}
	fila.DragEnd()
}

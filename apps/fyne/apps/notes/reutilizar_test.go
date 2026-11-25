// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"testing"
)

// TestRepintarReutilizaLasTarjetasQueNoCambian: un repintado sin cambios en una nota
// conserva SU tarjeta (con su renderer y sus medidas); cambiar la nota, o elegirla, la
// rehace. VISTO EN ROJO rehaciendo todas las tarjetas en cada repintado.
func TestRepintarReutilizaLasTarjetasQueNoCambian(t *testing.T) {
	p, _ := montarCaptura(t, 1280, 800, "columns")
	p.Repintar()
	antes := map[string]*Tarjeta{}
	for id, tj := range p.lienzo.vivas {
		antes[id] = tj
	}
	if len(antes) != 4 {
		t.Fatalf("hay %d tarjetas", len(antes))
	}
	c := p.notas[1].Crudo
	c.Title = "Cajas de cartón"
	p.notas[1] = DeNota(c)
	p.lienzo.Seleccionada = "n3"
	p.Repintar()
	for id, tj := range p.lienzo.vivas {
		cambia := id == "n2" || id == "n3"
		if (tj != antes[id]) != cambia {
			t.Errorf("la tarjeta de %s: rehecha %v, y tenía que ser %v", id, tj != antes[id], cambia)
		}
	}
	if got := p.lienzo.vivas["n2"].nota.Titulo; got != "Cajas de cartón" {
		t.Errorf("la tarjeta rehecha pinta %q", got)
	}
}

// TestElFiltroEsperaAQueSeDejeDeEscribir: cada tecla apunta el filtro y aplaza el
// repintado; el cuaderno se filtra al pasar EsperaFiltro sin teclas.
func TestElFiltroEsperaAQueSeDejeDeEscribir(t *testing.T) {
	p, _ := montarCaptura(t, 1280, 800, "list")
	p.Repintar()
	p.barra.AlFiltrar("caj")
	p.barra.AlFiltrar("cajas")
	if p.filtro != "cajas" || len(p.lienzo.Notas) != 4 {
		t.Fatalf("tras dos teclas: filtro %q y %d notas pintadas", p.filtro, len(p.lienzo.Notas))
	}
	if p.relojFiltro == nil || !p.relojFiltro.Stop() {
		t.Fatal("no hay un repintado aplazado")
	}
	p.Repintar()
	if len(p.lienzo.Notas) != 2 || p.filtroPintado != "cajas" { // «Cajas» y la lista de n1
		t.Errorf("filtrado: %d notas, pintado con %q", len(p.lienzo.Notas), p.filtroPintado)
	}
}

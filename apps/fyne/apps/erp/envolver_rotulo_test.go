// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"testing"

	"fyne.io/fyne/v2"
)

// TestUnaFilaConRotuloQueNoCabeBajaElRotuloASuLinea: con el rótulo delante y sin sitio
// para todo, el rótulo se queda solo arriba y las piezas empiezan TODAS en x = 0 debajo.
// Envuelto a la par, la segunda línea empezaba donde no empezaba la primera.
func TestUnaFilaConRotuloQueNoCabeBajaElRotuloASuLinea(t *testing.T) {
	anchos := []float32{100, 80, 80, 80, 80}
	altos := []float32{15, 24, 24, 24, 24}
	pos, ocupa := EnvolverConRotulo(anchos, altos, 4, 4, 250)
	if pos[0] != (fyne.Position{}) {
		t.Fatalf("el rótulo está en %v; va arriba a la izquierda", pos[0])
	}
	if pos[1].X != 0 || pos[1].Y < altos[0] {
		t.Errorf("la primera píldora está en %v; va al borde, bajo el rótulo", pos[1])
	}
	for i := 2; i < len(pos); i++ {
		if pos[i].Y > pos[i-1].Y && pos[i].X != 0 {
			t.Errorf("la pieza %d abre línea en x=%v; quiero 0", i, pos[i].X)
		}
	}
	if ultima := pos[len(pos)-1]; ocupa.Height < ultima.Y+altos[len(altos)-1] {
		t.Errorf("la fila mide %v de alto y la última pieza acaba en %v", ocupa.Height, ultima.Y+altos[len(altos)-1])
	}
	// Si cabe en una línea, nada cambia.
	if p, o := EnvolverConRotulo(anchos, altos, 4, 4, 1000); p[1].Y == 0 && p[1].X == 0 || o.Height != 24 {
		t.Errorf("en una línea la primera píldora va tras el rótulo (%v) y la fila mide 24 (%v)", p[1], o.Height)
	}
}

// TestLosFiltrosDelTelefonoEmpiezanTodosEnElBorde: a 390 (342 de columna) «Filtrar por
// estado:» va en su línea y cada línea de píldoras empieza en el borde, también la que
// empieza por «Papelera», que perdía su `ml-2` solo en mitad de una línea.
func TestLosFiltrosDelTelefonoEmpiezanTodosEnElBorde(t *testing.T) {
	l := NuevaLista(traductor(t))
	l.Modelo, l.Pagina, l.Fase = facturaArnes(), facturasArnes(), Exito
	l.Total, l.PorEstado = 4, map[string]int{"borrador": 1, "confirmado": 1, "pagado": 1, "anulado": 1}
	o, _ := l.filtros(0, 342)
	f, es := o.(*fyne.Container).Layout.(*filaFiltros)
	if !es {
		t.Fatalf("los filtros no son una filaFiltros: %T", o.(*fyne.Container).Layout)
	}
	if f.pos[0] != (fyne.Position{}) || f.pos[1].X != 0 || f.pos[1].Y == 0 {
		t.Fatalf("a 342 el rótulo va solo arriba y «Todos» al borde debajo: %v, %v", f.pos[0], f.pos[1])
	}
	for i := 2; i < len(f.pos); i++ {
		if f.pos[i].Y > f.pos[i-1].Y && f.pos[i].X != 0 {
			t.Errorf("la píldora %d abre línea en x=%v; quiero 0", i, f.pos[i].X)
		}
	}
}

// TestLasEtapasDelTelefonoNoSePartenUnaASola: a 390 la barra de la ficha pone «Etapa:»
// encima y las cuatro etapas de la factura en UNA línea; antes «Anulado» bajaba sola.
func TestLasEtapasDelTelefonoNoSePartenUnaASola(t *testing.T) {
	p := pantallaDePrueba(t)
	p.Fase = Exito
	p.Catalogo.Modulos[0].Modelos[0] = facturaArnes()
	p.lista.Modelo, p.lista.Pagina, p.lista.Fase = facturaArnes(), facturasArnes(), Exito
	p.Ventana = fyne.NewSize(390, 844)
	p.abrirFicha(&p.lista.Pagina.Registros[1], nil)
	f := p.ficha
	f.ancho = 390
	o, _ := f.barraEstado(0)
	b := o.(*fyne.Container).Layout.(*barraEstado)
	if len(b.posE) < 3 {
		t.Fatalf("la barra tiene %d piezas de etapa", len(b.posE))
	}
	// Centradas en su línea: la elegida es un poco más alta.
	centro := func(i int) float32 { return b.posE[i].Y + b.etapas[i].alto/2 }
	for i := 2; i < len(b.posE); i++ {
		if centro(i) != centro(1) {
			t.Errorf("la etapa %d va centrada en y=%v y la primera en %v: el control se parte", i, centro(i), centro(1))
		}
	}
	if b.posE[1].Y <= b.posE[0].Y {
		t.Errorf("a 390 «Etapa:» va en su línea, encima de las etapas")
	}
}

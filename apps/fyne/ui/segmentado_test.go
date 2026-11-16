// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLasTallasDelSegmentadoSonLasDeLaWeb: `size="sm"` es 24 de alto por opción y
// `size="md"` 28 (`h-7`), las dos con el `p-0.5` del carril. La mediana separa las
// opciones con su `gap-0.5`; la pequeña las junta, como el `flex` de AccountMenu.
func TestLasTallasDelSegmentadoSonLasDeLaWeb(t *testing.T) {
	opciones := []string{"Mes", "Semana", "Día", "Agenda"}
	sm := NuevoSegmentado("Vista", opciones, nil)
	if got := sm.MinSize(); got != MedidaSegmentado(opciones) || got.Height != 28 {
		t.Errorf("la pequeña mide %v; quiero %v (24 + 2×2)", got, MedidaSegmentado(opciones))
	}
	md := NuevoSegmentado("Vista", opciones, nil)
	md.Talla = SegmentoMD
	ancho := 2 * RellenoSegmentado
	for i, o := range opciones {
		if i > 0 {
			ancho += HuecoRejillaSegmento
		}
		ancho += Medir(o, tema.TextoSM, SemiNegrita).Width + 2*2.5*0.25*tema.Rem
	}
	if got := md.MinSize(); got.Height != 32 || got.Width != ancho {
		t.Errorf("la mediana mide %v; quiero %v × 32 (h-7 + 2×2, px-2.5, gap-0.5)", got, ancho)
	}
}

// TestElSegmentadoDeSoloIconosEsDeCuadrados: con SoloIcono cada opción es un cuadrado
// del alto de la talla (`w-7` en la mediana), el rótulo no se pinta y sigue siendo el
// nombre accesible: el conmutador lista/cuadrícula de Drive.
func TestElSegmentadoDeSoloIconosEsDeCuadrados(t *testing.T) {
	s := NuevoSegmentado("Vista de los archivos", []string{"Vista de lista", "Vista de cuadrícula"}, nil)
	s.Talla, s.SoloIcono = SegmentoMD, true
	s.Iconos = []iconos.Icono{iconos.Menu, iconos.Rejilla}
	quiero := fyne.NewSize(2*RellenoSegmentado+28+HuecoRejillaSegmento+28, 32)
	if got := s.MinSize(); got != quiero {
		t.Errorf("mide %v; quiero %v (dos w-7 con su gap-0.5 y el p-0.5)", got, quiero)
	}
	s.Resize(quiero)
	r := test.WidgetRenderer(s).(*segmentadoRenderer)
	for i, rotulo := range r.rotulos {
		if rotulo.Visible() {
			t.Errorf("con SoloIcono se pinta el rótulo %q", s.Opciones[i])
		}
		if r.iconos[i] == nil {
			t.Fatalf("la opción %d no lleva su icono", i)
		}
	}
	if got := s.AccessibilityLabel(); got != "Vista de los archivos: Vista de lista" {
		t.Errorf("el nombre accesible es %q", got)
	}
	// El icono va centrado en su cuadrado.
	ic := r.iconos[1].Objeto()
	centro := ic.Position().X + ic.Size().Width/2
	if want := RellenoSegmentado + 28 + HuecoRejillaSegmento + 14; centro < want-0.5 || centro > want+0.5 {
		t.Errorf("el icono de la cuadrícula está centrado en %v; quiero %v", centro, want)
	}
}

// TestElSegmentadoConIconoYRotuloSumaLosDos: un icono delante del rótulo ensancha la
// opción en su lado y el `gap-1.5`.
func TestElSegmentadoConIconoYRotuloSumaLosDos(t *testing.T) {
	sin := NuevoSegmentado("Vista", []string{"Lista"}, nil)
	con := NuevoSegmentado("Vista", []string{"Lista"}, nil)
	con.Iconos = []iconos.Icono{iconos.Menu}
	if d := con.MinSize().Width - sin.MinSize().Width; d != 14+HuecoIconoSegmento {
		t.Errorf("el icono ensancha %v; quiero 14 + %v", d, HuecoIconoSegmento)
	}
	// iconos.Ninguno es «sin icono» para esa opción.
	con.Iconos = []iconos.Icono{iconos.Ninguno}
	if con.MinSize() != sin.MinSize() {
		t.Error("iconos.Ninguno pinta un icono")
	}
}

// TestLaRejillaDelSegmentadoRepartePorColumnasYTrunca: con Rejilla las opciones van en
// columnas iguales a todo el ancho (el 2×2 del cajón del calendario), el MinSize no lo
// ponen los rótulos (`min-w-0`) y cada celda dice si su rótulo cabe entero.
func TestLaRejillaDelSegmentadoRepartePorColumnasYTrunca(t *testing.T) {
	s := NuevoSegmentado("Vista", []string{"Kuukausi", "Viikko", "Päivä", "Esityslista"}, nil)
	s.Talla, s.Rejilla = SegmentoMD, 2
	if got := s.MinSize(); got.Height != 2*28+HuecoRejillaSegmento+2*RellenoSegmentado || got.Width > 10 {
		t.Errorf("la rejilla pide %v: dos filas de 28 y ningún ancho propio", got)
	}
	s.Resize(fyne.NewSize(244, s.MinSize().Height))
	pos, tams, _ := s.reparto()
	celda := (244 - 2*RellenoSegmentado - HuecoRejillaSegmento) / 2
	for i := range s.Opciones {
		if tams[i].Width != celda {
			t.Errorf("la celda %d mide %v; quiero %v", i, tams[i].Width, celda)
		}
		if pos[i].X+tams[i].Width > 244 {
			t.Errorf("la celda %d se sale del grupo", i)
		}
		if !s.CabeRotulo(i) {
			t.Errorf("%q no cabe en 244", s.Opciones[i])
		}
	}
	if pos[2].Y != RellenoSegmentado+28+HuecoRejillaSegmento || pos[2].X != pos[0].X {
		t.Errorf("la tercera opción va en %v; quiero la segunda fila, primera columna", pos[2])
	}
	// En un grupo estrecho el rótulo largo no cabe y se trunca dentro de su celda.
	s.Resize(fyne.NewSize(120, s.MinSize().Height))
	if s.CabeRotulo(3) {
		t.Error("«Esityslista» dice que cabe en una celda de 59")
	}
	r := test.WidgetRenderer(s).(*segmentadoRenderer)
	r.Layout(s.Size())
	if got := r.rotulos[3].Text; got == "Esityslista" {
		t.Error("el rótulo que no cabe no se trunca")
	}
}

// TestElTecladoDelSegmentadoSigueElPatronDeRadios: ←/→ de uno en uno con vuelta,
// Inicio y Fin a los extremos y, en la rejilla, ↑/↓ de fila en fila. Es lo que hace el
// `SegmentedControl` de la web.
func TestElTecladoDelSegmentadoSigueElPatronDeRadios(t *testing.T) {
	var avisos []int
	s := NuevoSegmentado("Vista", []string{"Mes", "Semana", "Día", "Agenda"}, func(i int) { avisos = append(avisos, i) })
	pulsa := func(k fyne.KeyName) int { s.TypedKey(&fyne.KeyEvent{Name: k}); return s.Elegida }
	if got := pulsa(fyne.KeyLeft); got != 3 {
		t.Errorf("← desde la primera va a %d; quiero la última", got)
	}
	if got := pulsa(fyne.KeyRight); got != 0 {
		t.Errorf("→ desde la última va a %d; quiero la primera", got)
	}
	if got := pulsa(fyne.KeyEnd); got != 3 {
		t.Errorf("Fin va a %d", got)
	}
	if got := pulsa(fyne.KeyHome); got != 0 {
		t.Errorf("Inicio va a %d", got)
	}
	if got := pulsa(fyne.KeyDown); got != 1 {
		t.Errorf("↓ en una fila va a %d; quiero la siguiente", got)
	}
	s.Rejilla = 2
	if got := pulsa(fyne.KeyDown); got != 3 {
		t.Errorf("↓ en la rejilla desde la segunda va a %d; quiero la de debajo", got)
	}
	if got := pulsa(fyne.KeyUp); got != 1 {
		t.Errorf("↑ en la rejilla desde la cuarta va a %d; quiero la de encima", got)
	}
	if got := pulsa(fyne.KeyUp); got != 3 {
		t.Errorf("↑ en la rejilla desde la primera fila va a %d; quiero dar la vuelta", got)
	}
	if want := []int{3, 0, 3, 0, 1, 3, 1, 3}; len(avisos) != len(want) {
		t.Errorf("avisa %v; quiero %v", avisos, want)
	}
}

// TestLaMedianaPintaLaElegidaEnPrimaryText: la elegida va en `primary-text` (que
// invierte en oscuro) y la que está bajo el puntero gana el fondo `surface`.
func TestLaMedianaPintaLaElegidaEnPrimaryText(t *testing.T) {
	s := NuevoSegmentado("Vista", []string{"Mes", "Semana"}, nil)
	s.Talla = SegmentoMD
	s.Resize(s.MinSize())
	r := test.WidgetRenderer(s).(*segmentadoRenderer)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if got := r.rotulos[0].Color; got != tema.Color("primary-text", v) {
		t.Errorf("la elegida pinta %v y no primary-text", got)
	}
	if r.encima.Objetos()[0].Visible() {
		t.Error("sin puntero encima ya hay fondo de hover")
	}
	s.encima = 1
	r.Refresh()
	if !r.encima.Objetos()[0].Visible() || r.rotulos[1].Color != tema.Color("text", v) {
		t.Error("con el puntero encima la mediana no gana el fondo surface y el texto")
	}
	// La pequeña conserva su hover de solo tinta.
	p := NuevoSegmentado("Vista", []string{"Mes", "Semana"}, nil)
	p.Resize(p.MinSize())
	rp := test.WidgetRenderer(p).(*segmentadoRenderer)
	p.encima = 1
	rp.Refresh()
	if rp.encima.Objetos()[0].Visible() || rp.rotulos[0].Color != tema.Color("text", v) {
		t.Error("la pequeña cambia de fondo al pasar por encima o su elegida no va en text")
	}
}

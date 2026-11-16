// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las piezas del menú que pidió la sección de cuentas de correo externas de Mail en el
// cajón del teléfono (MAIL-15).

// Una dirección que no cabe conserva su dominio: «daniel.del…@outlook.com» y no
// «daniel.delgador@outlook.co…», que era lo que se veía (cortado a pelo, además). Sin
// «@» recorta por el final, y si el dominio solo ya no cabe, también.
//
// VISTO EN ROJO (2026-11-09) con `recorta` en su lugar: «…@outlook.co…».
func TestUnaDireccionSeRecortaEnMedio(t *testing.T) {
	const dir = "daniel.delgador@outlook.com"
	e, p := tema.TextoBase, Medio
	ancho := Medir("daniel.de@outlook.com", e, p).Width
	got := recortaEnMedio(dir, e, p, ancho)
	if !strings.HasSuffix(got, "…@outlook.com") || !strings.HasPrefix(got, "daniel") {
		t.Errorf("%q en %v queda %q y conserva «@outlook.com»", dir, ancho, got)
	}
	if Medir(got, e, p).Width > ancho {
		t.Errorf("%q no cabe en %v", got, ancho)
	}
	if got := recortaEnMedio(dir, e, p, Medir(dir, e, p).Width); got != dir {
		t.Errorf("la que cabe se recorta: %q", got)
	}
	// Sin el punto pegado a los puntos suspensivos.
	if got := recortaEnMedio(dir, e, p, Medir("daniel.@outlook.com…", e, p).Width); strings.Contains(got, ".…") {
		t.Errorf("queda %q, con el punto delante de los puntos suspensivos", got)
	}
	if got := recortaEnMedio("Cuenta del trabajo", e, p, Medir("Cuenta", e, p).Width); !strings.HasSuffix(got, "…") {
		t.Errorf("sin «@» queda %q y recorta por el final", got)
	}
	if got := recortaEnMedio(dir, e, p, Medir("@outlook", e, p).Width); strings.HasSuffix(got, "@outlook.com") {
		t.Errorf("sin sitio ni para el dominio queda %q", got)
	}
}

// La fila con RecorteEnMedio va en UNA línea aunque haya sitio para dos, con la pista
// del rótulo entero cuando se ha recortado, y mide lo mismo que una fila de una línea.
func TestLaFilaDeUnaDireccionVaEnUnaLinea(t *testing.T) {
	f := NuevaFilaLateral("una.direccion.bastante.larga@empresa.eu", canvas.NewRectangle(nil), nil)
	f.RecorteEnMedio = true
	corta := NuevaFilaLateral("Recibidos", canvas.NewRectangle(nil), nil)
	if f.MinSize().Height != corta.MinSize().Height {
		t.Errorf("la fila mide %v de alto y una de una línea %v", f.MinSize().Height, corta.MinSize().Height)
	}
	w := test.NewTempWindow(t, container.NewWithoutLayout(f))
	w.Resize(fyne.NewSize(300, 100))
	f.Resize(fyne.NewSize(180, f.MinSize().Height))
	test.WidgetRenderer(f).Layout(f.Size())
	if !f.recortado || f.titulo == nil {
		t.Fatalf("recortada %v, con pista %v", f.recortado, f.titulo != nil)
	}
	if !strings.HasSuffix(test.WidgetRenderer(f).(*filaRenderer).rotulo.Text, "@empresa.eu") {
		t.Errorf("se pinta %q", test.WidgetRenderer(f).(*filaRenderer).rotulo.Text)
	}
}

// Una rama se pliega también con las flechas, como un `treeitem` de la web: → la
// despliega, ← la pliega y la que no cambia nada no hace nada. Sus hijos, con
// SangriaRamaCompacta, van a `ml-3`.
//
// VISTO EN ROJO (2026-11-09) sin el caso de las flechas en TypedKey.
func TestLaRamaSePliegaConLasFlechas(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	pulsada := 0
	f := NuevaFilaLateral("ana@example.eu", nil, nil)
	RamaDesplegableCon(f, false, func() { pulsada++ }, SangriaRamaCompacta, v, canvas.NewRectangle(nil))
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if pulsada != 0 {
		t.Fatal("← en una rama plegada la despliega")
	}
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if pulsada != 1 {
		t.Fatal("→ no despliega la rama")
	}
	abierta := RamaDesplegableCon(f, true, func() { pulsada++ }, SangriaRamaCompacta, v, canvas.NewRectangle(nil))
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if pulsada != 2 {
		t.Fatalf("en la abierta, → y ← avisan %d veces y es una (←)", pulsada-1)
	}
	if m := abierta[1].(*fyne.Container).Layout.(Margen); m.Izq != tema.Espacio(3) {
		t.Errorf("los hijos van a %v y es `ml-3`", m.Izq)
	}
}

// El rótulo de una sección que no cabe acaba en «…» dentro de su línea, como el
// `truncate` de la web, en vez de salirse por el borde del cajón.
func TestElRotuloDeLaSeccionSeRecorta(t *testing.T) {
	s := NuevaSeccionLateral("Cuentas de correo externas", NuevaFilaLateral("x", nil, nil))
	w := test.NewTempWindow(t, container.NewWithoutLayout(s))
	w.Resize(fyne.NewSize(400, 200))
	s.Resize(fyne.NewSize(150, s.MinSize().Height))
	r := test.WidgetRenderer(s).(*seccionRenderer)
	r.Layout(s.Size())
	derecha, puntos := float32(0), false
	for _, l := range r.rotulo.letras {
		if !l.Visible() {
			continue
		}
		derecha = max(derecha, l.Position().X+Medir(l.Text, tema.TextoXS, SemiNegrita).Width)
		puntos = puntos || l.Text == "…"
	}
	if !puntos || derecha > 150-tema.Espacio(1) {
		t.Errorf("el rótulo acaba en %v (con «…» %v) y la sección mide 150", derecha, puntos)
	}
	// Con sitio, entero otra vez.
	s.Resize(fyne.NewSize(400, s.MinSize().Height))
	r.Layout(s.Size())
	for _, l := range r.rotulo.letras {
		if !l.Visible() || l.Text == "…" {
			t.Fatal("con sitio el rótulo sigue recortado")
		}
	}
}

// Un lateral nunca desplaza de lado: ni con un cuerpo montado con `container.NewScroll`
// (los dos ejes) ni con un contenido más ancho que él. El desplazador queda en vertical
// y el contenido mide lo que el menú, menos el canalón de la barra.
//
// VISTO EN ROJO (2026-11-09) sin `Direction` en alCanalDeLaBarra: el desplazador seguía
// en los dos ejes y el contenido medía sus 600.
func TestElLateralNoDesplazaDeLado(t *testing.T) {
	ancho := canvas.NewRectangle(nil)
	ancho.SetMinSize(fyne.NewSize(600, 40))
	s := container.NewScroll(ancho)
	l := NuevoLateral("Menú", s)
	w := test.NewTempWindow(t, l)
	w.Resize(fyne.NewSize(AnchoMenu(false), 400))
	l.Resize(fyne.NewSize(AnchoMenu(false), 400))
	test.WidgetRenderer(l).Layout(l.Size())
	if s.Direction != container.ScrollVerticalOnly {
		t.Fatalf("el cuerpo desplaza en %v y solo puede en vertical", s.Direction)
	}
	s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-200, 0)})
	if s.Offset.X != 0 {
		t.Errorf("el cuerpo se ha ido %v de lado", s.Offset.X)
	}
	if got, quiero := ancho.Size().Width, s.Size().Width-canalonMenu; got != quiero {
		t.Errorf("el contenido mide %v de ancho y cabe en %v", got, quiero)
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// bandaAncha es una pestaña de doce grupos de cuatro botones: unos 1.700 px, más que una
// ventana de 1280, como Datos de Calcus (1.502 medidos el 2026-10-16).
func bandaAncha(c *Contexto) fyne.CanvasObject {
	var objs []fyne.CanvasObject
	for i := range 12 {
		if i > 0 {
			objs = append(objs, SeparadorGrupos())
		}
		ctl := make([]fyne.CanvasObject, 0, 4)
		for range 4 {
			ctl = append(ctl, NuevoBotonCinta(iconos.EdNegrita, "b", func() {}))
		}
		objs = append(objs, NuevoGrupo("Grupo con nombre largo", ctl...))
	}
	return Grupos(objs...)
}

func cintaAncha(t *testing.T, ancho float32) (*Cinta, fyne.Window) {
	t.Helper()
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Listo, T: func(k string, _ i18n.Vars) string { return k }}
	r := Genericas()
	r.Sustituir("inicio", bandaAncha)
	m := NuevoMarco(ctx, r, nil)
	w := test.NewTempWindow(t, m)
	w.Resize(fyne.NewSize(ancho, 800))
	return m.cinta, w
}

// TestLaBandaQueNoCabeSeDesplazaYTieneMas: lo que no cabe NO se queda fuera de la ventana,
// que es lo que pasaba en Calcus a 1280 con «Ventana» y «Proteger». La banda acaba antes de
// «Más», «Más» va en la fila de controles, y el último grupo se alcanza desplazando.
// VISTO EN ROJO con la banda sin DesplazaLado (el último grupo empezaba en x=1.6xx).
func TestLaBandaQueNoCabeSeDesplazaYTieneMas(t *testing.T) {
	c, _ := cintaAncha(t, 1280)
	if !c.mas.Visible() {
		t.Fatal("la banda no cabe y no hay «Más»")
	}
	if fin := c.banda.Position().X + c.banda.Size().Width; fin > c.mas.Position().X+0.5 {
		t.Errorf("la banda acaba en %.1f y «Más» empieza en %.1f", fin, c.mas.Position().X)
	}
	fila := AltoDeTira() + ArribaGrupo + (AltoFilaGrupo-c.mas.Size().Height)/2
	if c.mas.Position().Y != fila {
		t.Errorf("«Más» está en y=%.1f y la fila de controles lo pone en %.1f", c.mas.Position().Y, fila)
	}
	objs := objetosDeBanda(c.cuerpo)
	ultimo := objs[len(objs)-1]
	d := fyne.CurrentApp().Driver()
	if d.AbsolutePositionForObject(ultimo).X < 1280 {
		t.Fatal("con la banda sin desplazar, el último grupo ya se ve: la prueba no mide nada")
	}
	c.banda.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-10000, 0)})
	x := d.AbsolutePositionForObject(ultimo).X
	if fin := x + ultimo.Size().Width; x < 0 || fin > c.mas.Position().X+0.5 {
		t.Errorf("desplazada hasta el fondo, el último grupo va de %.1f a %.1f y «Más» empieza en %.1f", x, fin, c.mas.Position().X)
	}
}

// TestMasDespliegaLaBandaYUnaAccionLaPliega: «Más» despliega TODA la pestaña en líneas,
// con los rótulos y sin separadores, y pulsar una acción —que reconstruye la cinta— la
// pliega, como `onPanelClick` de Ribbon.tsx. VISTO EN ROJO sin plegarMas en Reconstruir.
func TestMasDespliegaLaBandaYUnaAccionLaPliega(t *testing.T) {
	c, w := cintaAncha(t, 1280)
	c.mas.Tapped(nil)
	if c.desplegada == nil || !c.desplegada.Abierto() || !c.mas.Abierto {
		t.Fatal("«Más» no despliega la banda")
	}
	capas := w.Canvas().Overlays().List()
	if len(capas) == 0 {
		t.Fatal("la banda desplegada no está en la capa de encima")
	}
	grupos, separadores := 0, 0
	var contar func(o fyne.CanvasObject)
	contar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *Grupo:
			if x.Visible() {
				grupos++
				if !x.conRotulo() {
					t.Errorf("desplegado, el grupo %q no lleva rótulo", x.Rotulo)
				}
			}
			return
		case *separador:
			if x.Visible() {
				separadores++
			}
			return
		case *fyne.Container:
			for _, h := range x.Objects {
				contar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				contar(h)
			}
		}
	}
	contar(capas[len(capas)-1])
	if grupos != 12 || separadores != 0 {
		t.Errorf("desplegada enseña %d grupos y %d separadores; tendrían que ser 12 y 0", grupos, separadores)
	}
	c.Reconstruir()
	if c.desplegada != nil || c.mas.Abierto {
		t.Error("pulsar una acción no pliega la banda desplegada")
	}
}

// TestLosSeparadoresQueSobranNoSePintan: un grupo que no se monta deja juntos a sus dos
// separadores, y la banda enseñaba un «‖» (medido en la web entre «Formulario» y
// «Comentario»). Ni al principio, ni al final, ni detrás de otro.
func TestLosSeparadoresQueSobranNoSePintan(t *testing.T) {
	g := func() fyne.CanvasObject { return NuevoGrupo("g", NuevoBotonCinta(iconos.EdNegrita, "b", nil)) }
	s1, s2, s3, s4 := SeparadorGrupos(), SeparadorGrupos(), SeparadorGrupos(), SeparadorGrupos()
	fila := Grupos(s1, g(), s2, s3, g(), s4)
	fila.Refresh()
	_ = fila.MinSize()
	for i, c := range []struct {
		s     fyne.CanvasObject
		visto bool
	}{{s1, false}, {s2, true}, {s3, false}, {s4, false}} {
		if c.s.Visible() != c.visto {
			t.Errorf("separador %d: visible %v, y tendría que ser %v", i+1, c.s.Visible(), c.visto)
		}
	}
}

// TestLaCintaConElDedoMide44: con el dedo, todo lo que se pulsa en la cinta mide 44 como
// poco, y la tira de pestañas sube a 44 (`data-suelo-tactil` y `--editor-tabs-h` de la
// web). VISTO EN ROJO sin el suelo: los botones medían 28.
func TestLaCintaConElDedoMide44(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	if m := MedidaBotonCinta(); m.Width < 44 || m.Height < 44 {
		t.Errorf("el botón de la cinta mide %v con el dedo", m)
	}
	if h := MedidaPestanaCinta("Vista", false).Height; h < 43 {
		t.Errorf("la pestaña mide %.1f de alto con el dedo", h)
	}
	if h := MedidaChipCinta("Más").Height; h < 44 {
		t.Errorf("«Más» mide %.1f de alto con el dedo", h)
	}
	if h := MedidaBotonArchivo("Archivo").Height; h < 44 {
		t.Errorf("«Archivo» mide %.1f de alto con el dedo", h)
	}
	if AltoDeTira() != 44 {
		t.Errorf("la tira mide %.1f con el dedo", AltoDeTira())
	}
}

// TestElChipAbiertoSeVe: con su desplegable abierto («Historial», «Actividad»), el chip se
// pinta `selected`, como `aria-expanded:bg-selected` de CHIP_BTN; cerrado, vuelve. VISTO EN
// ROJO sin el `abierto()` del renderer: el chip no cambiaba con el menú abierto.
func TestElChipAbiertoSeVe(t *testing.T) {
	chip := NuevoChipCinta("Historial", "Historial", nil)
	w := test.NewTempWindow(t, chip)
	w.Resize(fyne.NewSize(200, 80))
	r := test.WidgetRenderer(chip).(*chipRenderer)
	fondo := func() color.Color {
		objs := r.caja.Objetos()
		return objs[len(objs)-1].(*canvas.Rectangle).FillColor
	}
	reposo := fondo()
	m := ui.NuevoMenu(ui.NuevoElementoMenu("v1", iconos.Ninguno, func() {}))
	chip.mostrarMenu(m)
	if !chip.abierto() || fondo() == reposo {
		t.Fatalf("con el menú abierto el chip no cambia (fondo %v)", fondo())
	}
	m.Cerrar()
	if chip.abierto() || fondo() != reposo {
		t.Errorf("cerrado el menú, el chip sigue abierto (fondo %v)", fondo())
	}
}

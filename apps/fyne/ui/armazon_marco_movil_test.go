// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestElMenuMovilDeMarcoDesplazaLaHojaSinEncogerla(t *testing.T) {
	enMarco(t)
	conPetroleoClaro(t)
	sinAnimacionDelMenu(t)
	for _, ancho := range []float32{320, 390, 767} {
		a := NuevoArmazon(contenidoDePrueba())
		a.Menu = NuevoLateral("Navegación", nil)
		a.Panel = contenidoDePrueba()
		a.Resize(fyne.NewSize(ancho, 600))
		r := test.WidgetRenderer(a).(*armazonRenderer)
		a.Refresh()
		util := ancho - AnchoCarrilMovil
		comprobar := func(x float32) {
			t.Helper()
			for _, o := range []fyne.CanvasObject{a.Panel, r.marco.hoja} {
				if o.Position() != fyne.NewPos(x, 0) || o.Size() != fyne.NewSize(util, 600) {
					t.Fatalf("a %g: la hoja no conserva su ancho/alineación: %v/%v; quiero x=%g ancho=%g", ancho, o.Position(), o.Size(), x, util)
				}
			}
		}
		comprobar(AnchoCarrilMovil)
		if a.barra.Size() != fyne.NewSize(AnchoCarrilMovil, 600) {
			t.Fatal("la navegación sigue ocupando una fila encima de la cabecera")
		}
		if centro := a.barra.boton.Position().Y + a.barra.boton.Size().Height/2; centro != MinCabeceraApp/2 {
			t.Fatalf("el menú no está alineado a la cabecera: centro=%g", centro)
		}
		a.AbrirCajon()
		menu := a.Menu.Size().Width
		comprobar(AnchoCarrilMovil + menu)
		if a.Menu.Position() != fyne.NewPos(AnchoCarrilMovil, 0) || r.velo.Position().X != AnchoCarrilMovil+menu {
			t.Fatal("el menú o su velo cubren el mando lateral")
		}
		a.entrada = -menu / 2
		a.Refresh()
		comprobar(AnchoCarrilMovil + menu/2)
		a.CerrarCajon()
		comprobar(AnchoCarrilMovil)
		a.DetalleAbierto = true
		a.Refresh()
		if a.Contenido.Position().X != AnchoCarrilMovil || a.Contenido.Size().Width != util || a.Panel.Visible() {
			t.Fatal("el detalle no usa el mismo espacio que la lista")
		}
	}
}

func TestElCarrilMovilSigueElColorElegidoYNoDejaPasarElMenu(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		for _, paleta := range []string{"petroleo", "cobalto", "app"} {
			tema.ElegirColor(paleta)
			tema.ElegirApp("drive")
			a := NuevoArmazon(contenidoDePrueba())
			a.Menu = NuevoLateral("Navegación", contenidoDePrueba())
			a.Resize(fyne.NewSize(390, 600))
			r := test.WidgetRenderer(a).(*armazonRenderer)
			a.AbrirCajon()
			a.entrada = -a.Menu.Size().Width / 2
			a.Refresh()
			img := pinta(t, 390, 600, a)
			for _, x := range []float32{1, AnchoCarrilMovil / 2, AnchoCarrilMovil - 2} {
				mira(t, img, variante+"/"+paleta+": el carril conserva su color", x, 300, r.marco.fondo.colorEn(x, 390))
			}
		}
	}
}

func TestElCarrilMovilConservaElCierrePorTecladoYLaVuelta(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	uno := NuevaFilaLateral("Recibidos", nil, nil)
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu = NuevoLateral("Navegación", container.New(Columna{}, uno))
	a.Panel = contenidoDePrueba()
	vueltas := 0
	a.AlVolver = func() { vueltas++ }
	w := test.NewTempWindow(t, a)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 600))
	a.AbrirCajon()
	c := w.Canvas()
	alcanzable := false
	for range 5 {
		c.FocusNext()
		if c.Focused() == a.barra.boton {
			alcanzable = true
		}
	}
	if !alcanzable {
		t.Fatal("el botón visible de cierre queda fuera del recorrido de Tab")
	}
	Descartar(c)
	if a.CajonAbierto || c.Focused() != fyne.Focusable(a.barra.boton) {
		t.Fatal("Escape no cierra o no devuelve el foco al carril")
	}
	a.DetalleAbierto = true
	a.Refresh()
	a.barra.boton.AlPulsar()
	if a.DetalleAbierto || vueltas != 1 {
		t.Fatal("la navegación lateral perdió la acción de volver a la lista")
	}
}

func sinAnimacionDelMenu(t *testing.T) {
	t.Helper()
	antes := SinMovimiento
	SinMovimiento = true
	t.Cleanup(func() { SinMovimiento = antes })
}

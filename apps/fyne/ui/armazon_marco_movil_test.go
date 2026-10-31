// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaHojaMovilOcupaTodoElAnchoYElCajonFlotaEncima(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	conPetroleoClaro(t)
	sinAnimacionDelMenu(t)
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, ancho := range []float32{280, 320, 390, 767} {
			t.Run(fmt.Sprintf("%s/%g", forma, ancho), func(t *testing.T) {
				a := NuevoArmazon(contenidoDePrueba())
				a.Menu = NuevoLateral("Navegación", nil)
				a.Panel = contenidoDePrueba()
				a.Resize(fyne.NewSize(ancho, 600))
				r := test.WidgetRenderer(a).(*armazonRenderer)
				a.Refresh()
				altoHoja := 600 - AltoBarraMovil
				comprobarHoja := func() {
					t.Helper()
					if a.barra.Position() != fyne.NewPos(0, 0) || a.barra.Size() != fyne.NewSize(ancho, AltoBarraMovil) {
						t.Fatalf("barra móvil en %v/%v, quiero (0,0)/(%g,%g)", a.barra.Position(), a.barra.Size(), ancho, AltoBarraMovil)
					}
					if a.Panel.Position() != fyne.NewPos(0, AltoBarraMovil) || a.Panel.Size() != fyne.NewSize(ancho, altoHoja) {
						t.Fatalf("el panel móvil no ocupa todo el ancho: %v/%v", a.Panel.Position(), a.Panel.Size())
					}
					if forma == "marco" && (r.marco.hoja.Position() != fyne.NewPos(0, AltoBarraMovil) || r.marco.hoja.Size() != fyne.NewSize(ancho, altoHoja)) {
						t.Fatalf("la hoja de Marco no ocupa todo el ancho: %v/%v", r.marco.hoja.Position(), r.marco.hoja.Size())
					}
				}
				comprobarHoja()
				a.AbrirCajon()
				comprobarHoja()
				quieroMenu := min(tema.AnchoLateral, ancho-MargenCajonMovil)
				if a.Menu.Position() != fyne.NewPos(0, 0) || a.Menu.Size() != fyne.NewSize(quieroMenu, 600) {
					t.Fatalf("el cajón mide %v en %v, quiero %g×600 en (0,0)", a.Menu.Size(), a.Menu.Position(), quieroMenu)
				}
				if ancho-a.Menu.Size().Width < MargenCajonMovil {
					t.Fatalf("el cajón no deja el margen libre de velo (%g px)", MargenCajonMovil)
				}
				a.arrastrarCierreCajon(-quieroMenu / 2)
				comprobarHoja()
				a.CerrarCajon()
				comprobarHoja()
				a.DetalleAbierto = true
				a.Refresh()
				if a.Contenido.Position() != fyne.NewPos(0, AltoBarraMovil) || a.Contenido.Size() != fyne.NewSize(ancho, altoHoja) || a.Panel.Visible() {
					t.Fatal("el detalle móvil no usa el ancho completo bajo la barra")
				}
			})
		}
	}
}

func TestElCajonMovilTieneSombraFileteBaseYVeloProporcional(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		for _, paleta := range []string{"petroleo", "cobalto", "app"} {
			tema.ElegirColor(paleta)
			tema.ElegirApp("drive")
			a := NuevoArmazon(contenidoDePrueba())
			a.Menu = NuevoLateral("Navegación", nil)
			a.Resize(fyne.NewSize(390, 600))
			r := test.WidgetRenderer(a).(*armazonRenderer)
			a.Refresh()
			if a.Menu.Visible() || r.velo.Visible() || r.sombraCajon.Objeto().Visible() || r.bordeCajon.Visible() || r.base.Visible() {
				t.Fatalf("%s/%s: capas del cajón visibles estando cerrado", variante, paleta)
			}

			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			mitad := tema.AnchoLateral / 2
			a.arrastrarAperturaCajon(mitad)
			if a.Menu.Position() != fyne.NewPos(-mitad, 0) || !r.sombraCajon.Objeto().Visible() || !r.bordeCajon.Visible() {
				t.Fatalf("%s/%s: el arrastre no desplaza el cajón con su sombra y filete", variante, paleta)
			}
			if r.velo.fondo.FillColor != ColorVelo(v, 0.5) {
				t.Fatalf("%s/%s: velo a mitad=%v, quiero %v", variante, paleta, r.velo.fondo.FillColor, ColorVelo(v, 0.5))
			}
			if r.base.Visible() != (variante == "traslucido") {
				t.Fatalf("%s/%s: base opaca visible=%v", variante, paleta, r.base.Visible())
			}

			a.AbrirCajon()
			if a.Menu.Position() != fyne.NewPos(0, 0) || r.bordeCajon.Position().X != tema.AnchoLateral-tema.Borde {
				t.Fatalf("%s/%s: posición final del cajón o su filete incorrecta", variante, paleta)
			}
			if r.velo.fondo.FillColor != ColorVelo(v, 1) {
				t.Fatalf("%s/%s: velo abierto=%v, quiero %v", variante, paleta, r.velo.fondo.FillColor, ColorVelo(v, 1))
			}
			img := pinta(t, 390, 600, a)
			if got := px(img, 40, 300); cercaDe(got, magenta, 24) {
				t.Fatalf("%s/%s: el contenido magenta se transparenta a través del cajón: %v", variante, paleta, got)
			}
		}
	}
}

func TestElCajonMovilSeCierraAlElegirDestinoYConservaTeclado(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	campoModal := NuevoBoton("En diálogo", nil)
	elegidos := 0
	var destino, filtro *FilaLateral
	var w fyne.Window

	destino = NuevaFilaLateral("Recibidos", nil, func() {
		elegidos++
		w.Canvas().Focus(campoModal)
	})
	filtro = NuevaFilaLateral("Solo no leídos", nil, func() { elegidos++ })
	filtro.MantieneCajon = true

	accionArbol := NuevaAccionLateral("Nueva página", iconoFalso(), nil)
	rama := NuevaFilaLateral("Notas", nil, nil)
	rama.Rama = &RamaLateral{ColaAlPasar: accionArbol}

	cabeceraPulsada := 0
	botonCabecera := NuevoBoton("Redactar", func() {
		cabeceraPulsada++
		w.Canvas().Focus(campoModal)
	})

	lat := NuevoLateral("Navegación", container.New(Columna{}, destino, filtro, rama)).ConCabecera(botonCabecera)
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu = lat
	a.Panel = container.NewStack(contenidoDePrueba(), campoModal)
	vueltas := 0
	a.AlVolver = func() { vueltas++ }

	w = test.NewTempWindow(t, a)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 600))
	c := w.Canvas()

	// En táctil las acciones secundarias de árbol en el cajón son alcanzables sin ratón.
	restaurarTactil := TactilDePrueba(true)
	rama.Resize(fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(rama).Height))
	_ = test.WidgetRenderer(rama)
	rama.Refresh()
	if !accionArbol.Visible() {
		restaurarTactil()
		t.Fatal("la acción de árbol con ColaAlPasar queda oculta en dispositivos táctiles")
	}
	restaurarTactil()

	// Trampa de foco y cierre con Escape.
	a.AbrirCajon()
	if a.FocusScope() != a.Menu {
		t.Fatal("el cajón abierto no encierra el recorrido de Tab en el menú")
	}
	for range 6 {
		c.FocusNext()
		if c.Focused() == fyne.Focusable(campoModal) {
			t.Fatal("Tab escapa del cajón hacia el contenido inferior")
		}
	}
	Descartar(c)
	if a.CajonAbierto || c.Focused() != fyne.Focusable(a.barra.boton) {
		t.Fatal("Escape no cierra el cajón o no devuelve el foco al botón de menú")
	}

	// Elegir un destino cierra el cajón ANTES de AlPulsar sin robar el foco posterior.
	a.AbrirCajon()
	test.Tap(destino)
	if a.CajonAbierto || elegidos != 1 || c.Focused() != fyne.Focusable(campoModal) {
		t.Fatalf("elegir destino: cajon=%v elegidos=%d foco=%T", a.CajonAbierto, elegidos, c.Focused())
	}

	// MantieneCajon conserva el cajón abierto para conmutadores o filtros internos.
	a.AbrirCajon()
	test.Tap(filtro)
	if !a.CajonAbierto || elegidos != 2 {
		t.Fatal("una fila con MantieneCajon cerró el cajón")
	}

	// El botón de cabecera del lateral también cierra el cajón antes de su acción.
	test.Tap(botonCabecera)
	if a.CajonAbierto || cabeceraPulsada != 1 || c.Focused() != fyne.Focusable(campoModal) {
		t.Fatalf("acción de cabecera: cajon=%v pulsada=%d foco=%T", a.CajonAbierto, cabeceraPulsada, c.Focused())
	}

	// Y el botón de volver en detalle sigue intacto.
	a.DetalleAbierto = true
	a.Refresh()
	a.barra.boton.AlPulsar()
	if a.DetalleAbierto || vueltas != 1 {
		t.Fatal("la barra móvil perdió la acción de volver a la lista")
	}
}

func sinAnimacionDelMenu(t *testing.T) {
	t.Helper()
	antes := SinMovimiento
	SinMovimiento = true
	t.Cleanup(func() { SinMovimiento = antes })
}

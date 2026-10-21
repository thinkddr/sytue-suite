// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// La barra de título del SISTEMA lleva el color de la barra de la suite (dueño,
// 2026-09-25: en macOS salía blanca encima de un cromo oscuro). La pone la barra
// porque es la pieza que las trece apps montan y la que se repinta al cambiar el tema,
// así que sigue el cambio en caliente sin que ningún `main` haga nada.
//
// Qué llega en cada sistema lo cuenta `escritorio.ColorearBarraTitulo`.

// Las dos puertas al sistema, para que un guardián las sustituya: el driver de pruebas
// no tiene ventana nativa.
var (
	ventanaNativaDe = ventanaNativa
	colorearTitulo  = escritorio.ColorearBarraTitulo
	identificar     = escritorio.IdentificarVentana
	// varianteSistema es la del SISTEMA, no la de la app (esa es `tema.De`).
	varianteSistema = func() fyne.ThemeVariant { return fyne.CurrentApp().Settings().ThemeVariant() }
)

// tituloPuesto es lo último que se le dijo al sistema, para no repetírselo en cada
// `Layout`.
type tituloPuesto struct {
	ventana uintptr
	color   color.NRGBA
	sistema fyne.ThemeVariant
}

// pintarTitulo le pasa al sistema el fondo de la barra si ha cambiado la ventana o el
// color. Se llama en `Layout` además de en `Refresh`: la ventana nativa no existe hasta
// que se enseña, y el primer `Refresh` llega antes.
func (r *barraSuiteRenderer) pintarTitulo() {
	h := ventanaNativaDe(r.b)
	if h == 0 {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := tituloPuesto{h, tema.Color(r.tokenFondo(), v), varianteSistema()}
	if p == r.titulo {
		return
	}
	cambiaSistema := r.titulo.ventana == h && p.sistema != r.titulo.sistema
	if r.titulo.ventana != h {
		// Una ventana nativa nueva: se le dice de qué app es (Plasma la une así a su
		// .desktop, y con él al número de avisos del lanzador).
		identificar(h, fyne.CurrentApp().Metadata().ID)
	}
	r.titulo = p
	colorearTitulo(p.ventana, p.color, v == tema.Oscuro)
	if cambiaSistema {
		// Al cambiar el tema del SISTEMA, el driver de Fyne repinta los widgets (y esto
		// corre) y DESPUÉS le pone a cada ventana el modo oscuro del sistema
		// (`applyThemeToWindow` → `setDarkMode` en Windows), que pisa el de la app
		// cuando la persona eligió otra apariencia. Se vuelve a poner cuando acabe.
		fyne.Do(func() { colorearTitulo(p.ventana, p.color, v == tema.Oscuro) })
	}
}

// ventanaNativa es el identificador nativo de la ventana en la que está `o`, o cero si
// aún no está en ninguna o no se ha enseñado.
func ventanaNativa(o fyne.CanvasObject) (h uintptr) {
	d := fyne.CurrentApp().Driver()
	c := d.CanvasForObject(o)
	if c == nil {
		return 0
	}
	for _, w := range d.AllWindows() {
		nw, ok := w.(driver.NativeWindow)
		if !ok || w.Canvas() != c {
			continue
		}
		nw.RunNative(func(ctx any) {
			switch x := ctx.(type) {
			case driver.MacWindowContext:
				h = x.NSWindow
			case driver.WindowsWindowContext:
				h = x.HWND
			case driver.X11WindowContext:
				h = x.WindowHandle
			}
		})
		return h
	}
	return 0
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Montar pone la barra de Sytue Office encima de contenido: la marca de la app y, a la
// derecha, Nube y Ajustes. No hay buscador global, campana ni cuenta: no hay cuenta ni
// servicio que los atienda (D-28).
func (l *App) Montar(w fyne.Window, p iconos.Producto, contenido fyne.CanvasObject) fyne.CanvasObject {
	marca := iconos.Marca(p, 24, tema.De(l.A.Settings().ThemeVariant())).Objeto()
	nombre := canvas.NewText(p.Nombre()+" · Office", theme.Color(theme.ColorNameForeground))
	nombre.TextStyle = fyne.TextStyle{Bold: true}
	var nube *ui.Boton
	nube = BotonDeBarra(l.T("nube"), iconos.Nube, func() { l.MenuNube(w, nube) })
	barra := container.NewHBox(container.NewCenter(marca), nombre, layout.NewSpacer(), nube,
		BotonDeBarra(l.T("ajustes"), iconos.Ajustes, func() { l.MostrarAjustes(w) }))
	w.SetOnClosed(l.Cerrar)
	return container.NewBorder(container.NewVBox(container.NewPadded(barra), widget.NewSeparator()), nil, nil, nil, contenido)
}

// BotonDeBarra es un botón de la barra de la edición (Nube, Ajustes; en el Hub también
// Traer y Exportar): el fantasma atenuado de `ui` con su glifo, `text-muted` en reposo y
// `text` sobre `bg-subtle` con el puntero, en talla `sm` (TestLaBarraDeLaEdicionUsaLosBotonesDeUI).
func BotonDeBarra(rotulo string, glifo iconos.Icono, alPulsar func()) *ui.Boton {
	b := ui.NuevoBoton(rotulo, alPulsar).Con(ui.Fantasma, ui.TallaSM).ConIcono(glifo, 0)
	b.Atenuado = true
	return b
}

// MenuNube despliega, colgado del botón Nube de la barra (disparador), lo que se puede
// hacer con la nube desde esta app: el `Menu` de `ui` (el Dropdown de la web, en su
// capa, alineado con el borde derecho del botón) con sus `ElementoMenu`, y no un diálogo
// con botones de Fyne. «Guardar en la nube…» solo sale si la app sabe exportar lo que
// tiene abierto (AlExportar). Devuelve el menú abierto.
func (l *App) MenuNube(w fyne.Window, disparador fyne.CanvasObject) *ui.Menu {
	elementos := []fyne.CanvasObject{
		ui.NuevoElementoMenu(l.T("nube.abrir"), iconos.Descargar, func() { l.ExplorarNube(w, false, "", nil) }),
	}
	l.mu.Lock()
	exportar := l.exportar
	l.mu.Unlock()
	if exportar != nil {
		elementos = append(elementos, ui.NuevoElementoMenu(l.T("nube.guardar"), iconos.Subir, func() {
			nombre, datos, err := exportar()
			if err != nil {
				l.Fallo(w, err)
				return
			}
			l.ExplorarNube(w, true, nombre, datos)
		}))
	}
	elementos = append(elementos, ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu(l.T("ajustes")+"…", iconos.Ajustes, func() { l.MostrarAjustes(w) }))
	m := ui.NuevoMenu(elementos...)
	m.Rotulo = l.T("nube")
	m.Mostrar(disparador)
	return m
}

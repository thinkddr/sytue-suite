// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// Los diálogos de Sytue Office se abren con `ui`, como los de toda la suite: así se
// cierran con Escape aunque nada tenga el foco y con «atrás» en el móvil, y salen en el
// inventario de modales (`ui/inventario_modales_test.go`). Estas funciones son la forma
// corta de los cuatro que usa la edición; no dependen de una `App` para que los usen
// también los paquetes de cada app (`local/correo`, `cmd-local/*`).

// Ventana abre un diálogo con cuerpo y un pie opcional (nil: solo «Cerrar»).
func Ventana(w fyne.Window, titulo string, cuerpo, pie fyne.CanvasObject) *ui.Dialogo {
	d := ui.NuevoDialogo(titulo, cuerpo).Con(ui.DialogoLG, pie)
	d.Mostrar(w.Canvas())
	return d
}

// Info dice algo y se cierra.
func Info(w fyne.Window, titulo, texto string) *ui.Dialogo {
	l := widget.NewLabel(texto)
	l.Wrapping = fyne.TextWrapWord
	d := ui.NuevoDialogo(titulo, l).Con(ui.DialogoSM, nil)
	d.Mostrar(w.Canvas())
	return d
}

// Confirmar pregunta antes de hacer algo. `verbo` dice qué pasa al confirmar; `peligro`
// para lo que borra o desconecta.
func Confirmar(w fyne.Window, titulo, texto, cancelar, verbo string, peligro bool, si func()) *ui.Dialogo {
	l := widget.NewLabel(texto)
	l.Wrapping = fyne.TextWrapWord
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, cancelar, verbo, peligro, l, nil, func() {
		d.Cerrar()
		si()
	})
	d.Mostrar(w.Canvas())
	return d
}

// Formulario pide unos campos. `si` recibe el diálogo para cerrarlo cuando haya
// terminado (puede quedarse abierto si algo no valida).
func Formulario(w fyne.Window, titulo, cancelar, verbo string, campos []*widget.FormItem, si func(d *ui.Dialogo)) *ui.Dialogo {
	var d *ui.Dialogo
	d = ui.Confirmacion(titulo, cancelar, verbo, false, widget.NewForm(campos...), nil, func() { si(d) })
	d.Mostrar(w.Canvas())
	return d
}

// Espera enseña que algo está en marcha; quien la abre la cierra con Cerrar.
func Espera(w fyne.Window, titulo, texto string) *ui.Dialogo {
	l := widget.NewLabel(texto)
	l.Wrapping = fyne.TextWrapWord
	d := ui.NuevoDialogo(titulo, container.NewVBox(l, widget.NewProgressBarInfinite())).Con(ui.DialogoSM, nil)
	d.Ocupado(true)
	d.Mostrar(w.Canvas())
	return d
}

// ElegirFichero abre el selector del sistema y llama a listo, en el hilo de la interfaz,
// con lo elegido. Cancelar no llama a nada.
func ElegirFichero(listo func(rutas []string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil || len(rutas) == 0 {
			return
		}
		fyne.Do(func() { listo(rutas) })
	}()
}

// ElegirCarpeta abre el selector de carpetas del sistema.
func ElegirCarpeta(titulo string, listo func(ruta string)) {
	go func() {
		ruta, err := ficheros.ElegirCarpeta(titulo)
		if err != nil || ruta == "" {
			return
		}
		fyne.Do(func() { listo(ruta) })
	}()
}

// Tactil dice si la app corre en un dispositivo táctil (móvil), por la puerta de `ui`.
func Tactil() bool { return ui.EsTactil() }

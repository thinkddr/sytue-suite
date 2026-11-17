// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// Ocultar y mostrar filas o columnas sueltas, como el comando `hide` de SheetCanvas.tsx:
// los tramos de `hiddenRows` y `hiddenCols`, con la lógica de `analisis` (la misma que
// sheets/ocultas.ts, con los mismos vectores). No es el esquema: no hay grupo ni botón de
// contraer, solo lo oculto sobre lo que haya seleccionado.

// CmdOcultar es `hide`: JSON {axis: "row"|"col", op: "hide"|"show"}.
const CmdOcultar editor.Comando = "hide"

// peticionOcultar es el valor del comando `hide`.
type peticionOcultar struct {
	Axis string `json:"axis"`
	Op   string `json:"op"`
}

// Ocultar es el comando `hide` sobre el rango: oculta o vuelve a mostrar sus filas o sus
// columnas. Devuelve si cambió algo y `todas`, que es que se pidió ocultar la última que
// quedaba a la vista (no se hace, y la web lo avisa). La selección no se mueve: «Mostrar»
// justo después deshace lo que se acaba de ocultar.
func (l *Libro) Ocultar(valor string, c1, f1, c2, f2 int) (cambio, todas bool) {
	var req peticionOcultar
	if json.Unmarshal([]byte(valor), &req) != nil || req.Op != "hide" && req.Op != "show" {
		return false, false
	}
	h := l.h()
	tramos, tope, a, b := h.ocultFilas, MaxFilas, f1, f2
	if req.Axis == "col" {
		tramos, tope, a, b = h.ocultCols, MaxColumnas, c1, c2
	}
	var nuevos []analisis.Tramo
	if req.Op == "hide" {
		// Contra los límites de la hoja (los de Excel, como en la web): lo que no se
		// puede es dejar sin nada a la vista la hoja entera.
		n, ok := analisis.OcultarTramo(tramos, a, b, tope)
		if !ok {
			return false, true
		}
		nuevos = n
	} else {
		nuevos = analisis.MostrarTramo(tramos, a, b)
	}
	if slices.Equal(nuevos, tramos) {
		return false, false
	}
	if len(nuevos) == 0 {
		nuevos = nil // vacío es «nada oculto»: no se guarda
	}
	l.anotar()
	l.version++
	if req.Axis == "col" {
		h.ocultCols = nuevos
	} else {
		h.ocultFilas = nuevos
	}
	return true, false
}

// TramosOcultos son las filas y las columnas ocultas a mano de la hoja activa.
func (l *Libro) TramosOcultos() (filas, cols []analisis.Tramo) {
	return l.h().ocultFilas, l.h().ocultCols
}

// mandarOcultar ejecuta `hide`; avisa si se pidió ocultarlo todo.
func (c *Lienzo) mandarOcultar(valor string, c1, f1, c2, f2 int) bool {
	cambio, todas := c.Libro.Ocultar(valor, c1, f1, c2, f2)
	if todas && c.AlAviso != nil {
		var req peticionOcultar
		_ = json.Unmarshal([]byte(valor), &req)
		clave := "sheet.msg.hideAll.rows"
		if req.Axis == "col" {
			clave = "sheet.msg.hideAll.cols"
		}
		c.AlAviso(ui.AvisoInfo, c.t(clave, nil), "")
	}
	return cambio
}

// opcionesOcultar son las cuatro órdenes del menú, en el orden de HIDE_SHOW_ITEMS de la web.
var opcionesOcultar = []struct{ eje, op, clave string }{
	{"row", "hide", "data.hideRows"},
	{"row", "show", "data.showRows"},
	{"col", "hide", "data.hideCols"},
	{"col", "show", "data.showCols"},
}

// anchoPanelOcultar y rellenoMenuOcultar son el `w-48 p-1` del menú de la web.
var (
	anchoPanelOcultar  = tema.Espacio(48)
	rellenoMenuOcultar = tema.Espacio(1)
)

// botonOcultar es `HideShowMenu`: el ojo del grupo Celdas, que abre las cuatro órdenes.
func (lz *Lienzo) botonOcultar(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdEye, c.Traducir("data.hideShow"), func() { abrirOcultar(c, b) })
	b.Inerte = c.SoloLectura
	return b
}

// abrirOcultar despliega el menú: filas arriba, columnas abajo, con la raya de MenuSeparator.
func abrirOcultar(c *editor.Contexto, disparador *editor.BotonCinta) {
	var em *ui.Emergente
	filas := make([]fyne.CanvasObject, 0, len(opcionesOcultar)+1)
	for i, o := range opcionesOcultar {
		if i == 2 {
			filas = append(filas, ui.NuevoSeparadorMenu())
		}
		valor := `{"axis":"` + o.eje + `","op":"` + o.op + `"}`
		it := ui.NuevoElementoMenu(c.Traducir(o.clave), iconos.Ninguno, func() {
			em.Cerrar()
			c.Mandar(CmdOcultar, valor)
		})
		it.Denso = true
		filas = append(filas, it)
	}
	disparador.Activo = true
	disparador.Refresh()
	ancho := anchoPanelOcultar - 2*(rellenoMenuOcultar+tema.Borde)
	em = ui.MostrarEmergente(disparador, container.New(anchoFijo{ancho: ancho, col: ui.Columna{}}, filas...), rellenoMenuOcultar)
	em.AlRecogerse = func() { disparador.Activo = false; disparador.Refresh() }
}

// tramosONil es la lista, o nil si está vacía: vacío es «nada oculto» y no se guarda.
func tramosONil(ts []analisis.Tramo) []analisis.Tramo {
	if len(ts) == 0 {
		return nil
	}
	return ts
}

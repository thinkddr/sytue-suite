// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los controles de la cinta de Σ (Fórmulas) y «Limpiar texto» (Datos): `AutoSumMenu` y
// `CleanTextMenu` de `ribbon/tabs/Datos.tsx`, con sus mismos rótulos.

// rotuloFuncionAutosuma es la clave del rótulo de cada función del «▾» de Σ.
var rotuloFuncionAutosuma = map[string]string{
	"SUM": "query.agg.sum", "AVERAGE": "query.agg.avg", "COUNT": "pivot.agg.countNums",
	"MAX": "query.agg.max", "MIN": "query.agg.min",
}

// rotuloModoTexto es la clave del rótulo de cada modo de «Limpiar texto».
var rotuloModoTexto = map[string]string{
	"trim": "data.cleanText.trim", "sentence": "format.case.sentence", "lower": "format.case.lower",
	"upper": "format.case.upper", "title": "format.case.title",
}

// menuDeValores despliega bajo `b` un menú con un elemento por valor, que manda `cmd` con
// él. `w-48`/`w-56` de la web: el ancho del panel lo da quien llama.
func menuDeValores(c *editor.Contexto, b *editor.BotonCinta, rotulo string, ancho float32,
	cmd editor.Comando, valores []string, claves map[string]string,
) func() {
	var menu *ui.Menu
	return func() {
		if menu != nil && menu.Abierto() {
			menu.Cerrar()
			return
		}
		elems := make([]fyne.CanvasObject, 0, len(valores))
		for _, v := range valores {
			e := ui.NuevoElementoMenu(c.Traducir(claves[v]), iconos.Ninguno, func() { c.Mandar(cmd, v) })
			e.Denso = true
			elems = append(elems, e)
		}
		menu = ui.NuevoMenu(elems...)
		menu.Rotulo, menu.AlInicio, menu.Ancho = rotulo, true, ancho
		menu.Mostrar(b)
	}
}

// botonesAutosuma son Σ (la suma de un clic) y su «▾» con las cinco funciones.
func (lz *Lienzo) botonesAutosuma(c *editor.Contexto) []fyne.CanvasObject {
	suma := editor.NuevoBotonCinta(iconos.EdSubtotal, c.Traducir("sheet.autosum"), func() { c.Mandar(CmdAutosuma, "SUM") })
	mas := editor.NuevoBotonCinta(iconos.EdChevronAbajo, c.Traducir("sheet.autosum.more"), nil)
	mas.AlPulsar = menuDeValores(c, mas, c.Traducir("sheet.autosum"), anchoPanelTexto, CmdAutosuma, FuncionesAutosuma, rotuloFuncionAutosuma)
	suma.Inerte, mas.Inerte = c.SoloLectura, c.SoloLectura
	return []fyne.CanvasObject{suma, mas}
}

// botonLimpiarTexto abre los cinco modos de «Limpiar texto».
func (lz *Lienzo) botonLimpiarTexto(c *editor.Contexto) *editor.BotonCinta {
	b := editor.NuevoBotonCinta(iconos.EdMayusculas, c.Traducir("data.cleanText"), nil)
	b.AlPulsar = menuDeValores(c, b, c.Traducir("data.cleanText"), anchoPanelCelda, CmdLimpiarTexto, ModosTexto, rotuloModoTexto)
	b.Inerte = c.SoloLectura
	return b
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// # Los atajos de hoja que no son de la cinta (`sheets/atajosHoja.ts`)
//
// Ctrl+D y Ctrl+R rellenan, Ctrl+; escribe la fecha de hoy, Mayús+Espacio y Ctrl+Espacio
// marcan filas y columnas enteras, Ctrl+G (y F5) lleva al cuadro de nombres, y Ctrl+/
// abre la hoja de atajos con todos los de la rejilla. Quien viene de Excel o de Sheets los busca con los dedos antes que con el
// ratón, y una tecla que nadie enseña no la usa nadie.

// rellenoDeAtajo es el de la web: con un rango, la primera fila (o columna) se copia
// sobre el resto; con una sola, se copia la de encima (o la de la izquierda). Sin de
// dónde copiar —la fila 1 con Ctrl+D, la columna A con Ctrl+R— no hay relleno.
func rellenoDeAtajo(c1, f1, c2, f2 int, abajo bool) (origen, destino [4]int, ok bool) {
	if abajo {
		if f1 == f2 {
			if f1 == 0 {
				return origen, destino, false
			}
			return [4]int{c1, f1 - 1, c2, f1 - 1}, [4]int{c1, f1, c2, f2}, true
		}
		return [4]int{c1, f1, c2, f1}, [4]int{c1, f1 + 1, c2, f2}, true
	}
	if c1 == c2 {
		if c1 == 0 {
			return origen, destino, false
		}
		return [4]int{c1 - 1, f1, c1 - 1, f2}, [4]int{c1, f1, c2, f2}, true
	}
	return [4]int{c1, f1, c1, f2}, [4]int{c1 + 1, f1, c2, f2}, true
}

// RellenarConAtajo es Ctrl+D (abajo) o Ctrl+R: el asa de autorrelleno sin arrastrar. La
// selección no se mueve: con una sola fila, el asa la ampliaría a la de encima, que es de
// donde se copió y no lo que se había marcado.
func (r *Rejilla) RellenarConAtajo(abajo bool) {
	if r.SoloLectura {
		return
	}
	c1, f1, c2, f2 := r.Rango()
	o, d, ok := rellenoDeAtajo(c1, f1, c2, f2, abajo)
	if !ok || r.bloqueado(d[0], d[1], d[2], d[3]) {
		return
	}
	r.L.Rellenar(o, d)
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

// EscribirHoy es Ctrl+;: la fecha de `ahora` como VALOR en la celda activa —no `=HOY()`,
// que cambia cada día— y con formato de fecha si la celda no lo tenía. Respeta la
// protección y la validación como teclearla.
func (r *Rejilla) EscribirHoy(ahora time.Time) {
	if r.SoloLectura || r.protegida(r.sel) {
		return
	}
	texto := strconv.Itoa(formula.SerialDeFecha(ahora.Year(), int(ahora.Month()), ahora.Day()))
	if r.rechazado(texto) {
		return
	}
	r.L.PonerFecha(r.sel.C, r.sel.F, texto)
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

// PonerFecha escribe el número de serie de una fecha en la celda (con su negrita y su
// alineación) y le pone el formato de fecha si no lo tenía, en UN paso de deshacer.
func (l *Libro) PonerFecha(c, f int, serial string) {
	l.anotar()
	l.poner(l.f.activa, c, f, conTexto(l.HTML(c, f), serial))
	h := l.h()
	k := Celda{C: c, F: f}
	if h.formatos[k].ID != "date" {
		h.formatos[k] = FormatoNum{ID: "date"}
	}
}

// SeleccionarEnteras es Mayús+Espacio (filas) o Ctrl+Espacio (columnas): las del rango,
// como el clic en las cabeceras; el ancla va a la columna A o a la fila 1.
func (r *Rejilla) SeleccionarEnteras(filas bool) {
	c1, f1, c2, f2 := r.Rango()
	if filas {
		r.sel, r.ext = Celda{C: 0, F: f1}, &Celda{C: r.Cols - 1, F: f2}
	} else {
		r.sel, r.ext = Celda{C: c1, F: 0}, &Celda{C: c2, F: r.Fils - 1}
	}
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// atajoDeHoja atiende los de este fichero que llegan con Ctrl (⌘ en el Mac). Ctrl+; y
// Ctrl+/ valen con Mayúsculas o sin ellas: en un teclado español los dos signos van con
// Mayúsculas, y lo que se busca es el signo. Devuelve si era uno de ellos.
func (r *Rejilla) atajoDeHoja(k fyne.KeyName, mod fyne.KeyModifier) bool {
	ctrl := fyne.KeyModifierShortcutDefault
	switch {
	case mod&^fyne.KeyModifierShift == ctrl && k == fyne.KeySlash:
		if r.AlAtajos != nil {
			r.AlAtajos()
		}
	case mod&^fyne.KeyModifierShift == ctrl && k == fyne.KeySemicolon:
		r.EscribirHoy(time.Now())
	case mod == ctrl && (k == fyne.KeyD || k == fyne.KeyR):
		r.RellenarConAtajo(k == fyne.KeyD)
	case mod == ctrl && k == fyne.KeyG:
		r.enfocarCuadro()
	case (mod == ctrl || mod == fyne.KeyModifierControl) && k == fyne.KeySpace:
		// En el Mac, ⌃Espacio, como Excel: ⌘+Espacio lo coge Spotlight y no llega.
		r.SeleccionarEnteras(false)
	default:
		return false
	}
	return true
}

// MostrarAtajos abre la hoja de atajos. Dentro del editor es la del editor entero
// (`editor/atajosayuda.go`, la misma de Vista y de Ctrl+/ con el foco fuera de la hoja),
// que en Calcus acaba con los de la rejilla ([Lienzo.AtajosPropios]); la rejilla suelta,
// sin marco, enseña solo los suyos.
func (c *Lienzo) MostrarAtajos() {
	if ctx := c.Rejilla.Contexto; ctx != nil && ctx.Atajos != nil {
		ctx.Atajos()
		return
	}
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(c)
	if lienzo == nil {
		return
	}
	ui.MostrarHojaAtajos(lienzo, c.t("sheet.shortcuts.title", nil), c.t("ui.close", nil),
		GruposDeAtajosHoja(func(k string) string { return c.t(k, nil) }))
}

// filaAtajo es una fila de la hoja: combinaciones en notación del registro y la clave de
// lo que hacen. `{x}` es una tecla con nombre, que va por el catálogo.
type filaAtajo struct {
	teclas      []string
	descripcion string
}

var nombresDeTecla = map[string]string{
	"{space}":    "slides.present.key.space",
	"{enter}":    "slides.present.key.enter",
	"{pageDown}": "slides.present.key.pageDown",
	"{pageUp}":   "slides.present.key.pageUp",
	"{home}":     "slides.present.key.home",
	"{end}":      "slides.present.key.end",
	"{esc}":      "slides.present.key.esc",
	"{tab}":      "sheet.shortcuts.key.tab",
	"{delete}":   "sheet.shortcuts.key.delete",
}

// gruposHoja es `GRUPOS_HOJA`, fila a fila y en el mismo orden.
var gruposHoja = []struct {
	titulo string
	filas  []filaAtajo
}{
	{"sheet.shortcuts.group.move", []filaAtajo{
		{[]string{"←", "↑", "→", "↓"}, "sheet.shortcuts.move"},
		{[]string{"Ctrl+→"}, "sheet.shortcuts.jump"},
		{[]string{"{home}", "Ctrl+{home}"}, "sheet.shortcuts.home"},
		{[]string{"{end}", "Ctrl+{end}"}, "sheet.shortcuts.end"},
		{[]string{"{pageDown}", "{pageUp}"}, "sheet.shortcuts.page"},
		{[]string{"{tab}", "Shift+{tab}"}, "sheet.shortcuts.tab"},
		{[]string{"{esc}"}, "sheet.shortcuts.escape"},
		{[]string{"Ctrl+G", "F5"}, "sheet.shortcuts.goTo"},
	}},
	{"sheet.shortcuts.group.select", []filaAtajo{
		{[]string{"Shift+→"}, "sheet.shortcuts.extend"},
		{[]string{"Shift+{space}"}, "sheet.shortcuts.row"},
		{[]string{"Control+{space}"}, "sheet.shortcuts.column"},
		{[]string{"Ctrl+A"}, "sheet.shortcuts.all"},
	}},
	{"sheet.shortcuts.group.edit", []filaAtajo{
		{[]string{"{enter}", "F2"}, "sheet.shortcuts.edit"},
		{[]string{"{delete}"}, "sheet.ctx.clear"},
		{[]string{"Ctrl+D"}, "sheet.shortcuts.fillDown"},
		{[]string{"Ctrl+R"}, "sheet.shortcuts.fillRight"},
		{[]string{"Ctrl+;"}, "sheet.shortcuts.today"},
		{[]string{"Ctrl+C"}, "cmd.copy"},
		{[]string{"Ctrl+X"}, "cmd.cut"},
		{[]string{"Ctrl+V"}, "cmd.paste"},
		{[]string{"Ctrl+Shift+V"}, "cmd.pasteValues"},
		{[]string{"Ctrl+Z"}, "action.undo"},
		{[]string{"Ctrl+Y"}, "action.redo"},
		{[]string{"Ctrl+B"}, "format.bold"},
		{[]string{"Ctrl+I"}, "format.italic"},
		{[]string{"Alt+{enter}"}, "sheet.shortcuts.link"},
		{[]string{"Shift+F10"}, "sheet.shortcuts.contextMenu"},
		{[]string{"Ctrl+/"}, "sheet.shortcuts.help"},
	}},
}

// teclaDeHoja es `teclaDeHoja`: los nombres de tecla por el catálogo y los modificadores
// como los escribe el sistema.
func teclaDeHoja(combinacion string, t func(string) string) string {
	return teclaDeHojaPara(runtime.GOOS, combinacion, t)
}

// teclaDeHojaPara es teclaDeHoja con el sistema como argumento, para probar el del Mac.
// «Control+» es la tecla Control también allí (⌃): ⌘+Espacio es de Spotlight.
func teclaDeHojaPara(goos, combinacion string, t func(string) string) string {
	for marca, clave := range nombresDeTecla {
		combinacion = strings.ReplaceAll(combinacion, marca, t(clave))
	}
	if resto, ok := strings.CutPrefix(combinacion, "Control+"); ok {
		if goos == "darwin" || goos == "ios" {
			return "⌃" + resto
		}
		combinacion = "Ctrl+" + resto
	}
	return editor.TextoAtajoPara(goos, combinacion, t)
}

// GruposDeAtajosHoja es `gruposDeAtajosHoja`: la hoja de atajos en el idioma de `t`.
func GruposDeAtajosHoja(t func(string) string) []ui.GrupoDeAtajos {
	return gruposDeAtajosHoja(t, nil)
}

// AtajosPropios es `atajosPropiosDeHoja`: lo que la hoja del editor añade en Calcus, los
// de la rejilla sin repetir los que ya salen del marco o del registro (copiar, pegar,
// deshacer, negrita, la propia Ctrl+/).
func AtajosPropios(t func(string) string) []ui.GrupoDeAtajos {
	yaEstan := map[string]bool{}
	for _, f := range editor.AtajosDelMarco {
		yaEstan[f.Keys] = true
	}
	for _, g := range editor.AtajosDelRegistro(office.Hoja) {
		for _, f := range g.Filas {
			yaEstan[f.Keys] = true
		}
	}
	return gruposDeAtajosHoja(t, yaEstan)
}

// gruposDeAtajosHoja monta la hoja sin las filas cuyas combinaciones están TODAS en omitir.
func gruposDeAtajosHoja(t func(string) string, omitir map[string]bool) []ui.GrupoDeAtajos {
	out := make([]ui.GrupoDeAtajos, 0, len(gruposHoja))
	for _, g := range gruposHoja {
		grupo := ui.GrupoDeAtajos{Titulo: t(g.titulo)}
		for _, f := range g.filas {
			if todasEn(f.teclas, omitir) {
				continue
			}
			teclas := make([]string, len(f.teclas))
			for i, k := range f.teclas {
				teclas[i] = teclaDeHoja(k, t)
			}
			grupo.Atajos = append(grupo.Atajos, ui.AtajoDeTeclado{Teclas: teclas, Descripcion: t(f.descripcion)})
		}
		if len(grupo.Atajos) > 0 {
			out = append(out, grupo)
		}
	}
	return out
}

func todasEn(teclas []string, conjunto map[string]bool) bool {
	if len(conjunto) == 0 {
		return false
	}
	for _, k := range teclas {
		if !conjunto[k] {
			return false
		}
	}
	return true
}

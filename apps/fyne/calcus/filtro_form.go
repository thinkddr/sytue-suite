// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Filtrar por condición…» del desplegable de una columna del autofiltro
// (`FiltroCondicion.tsx`): el operador y sus operandos —valor, rango, N o color— y
// «Aplicar», «Quitar condición» y «Volver a los valores». El modelo y el comando
// (`filtercond`) ya estaban (filtro_cond.go); faltaba desde dónde pedirlo.

// AnchoFormFiltro es el `w-64` del formulario.
var AnchoFormFiltro = tema.Espacio(64)

// formFiltro es el borrador del formulario: lo que se teclea, sin sanear todavía.
type formFiltro struct {
	col     int
	op      string
	a, b, n string
	porcien bool
	color   string
	// actual es la condición que ya tiene la columna, si la tiene: sin ella no se ofrece
	// «Quitar condición».
	actual *CondFiltro
}

// nuevoFormFiltro arranca el borrador desde la condición de la columna, o desde «Mayor
// que» si no tiene ninguna (`current?.op ?? 'gt'`, y N = 10).
func nuevoFormFiltro(col int, actual *CondFiltro) *formFiltro {
	f := &formFiltro{col: col, op: "gt", n: "10", actual: actual}
	if actual != nil {
		f.op, f.a, f.b, f.porcien, f.color = actual.Op, actual.A, actual.B, actual.Percent, actual.Color
		if actual.N > 0 {
			f.n = strconv.Itoa(actual.N)
		}
	}
	return f
}

// cond es el borrador saneado por el MISMO `sanitizeFilterCond` que lee el documento:
// false si le falta algo, y entonces «Aplicar» está apagado.
func (f *formFiltro) cond() (CondFiltro, bool) {
	return sanearCondFiltro(map[string]any{
		"op": f.op, "a": f.a, "b": f.b, "n": f.n, "percent": f.porcien, "color": f.color,
	})
}

// ColoresFiltro son los colores manuales de relleno (o de texto) que hay en el cuerpo de
// la columna, en el orden de sus filas y sin repetir (`coloresDeColumna`).
func (l *Libro) ColoresFiltro(col int, relleno bool) []string {
	af := l.Autofiltro()
	if af == nil {
		return nil
	}
	var filas []int
	for cel := range l.h().estilos {
		if cel.C == col && cel.F > af.Row {
			filas = append(filas, cel.F)
		}
	}
	slices.Sort(filas)
	var out []string
	for _, f := range filas {
		e := l.Estilo(col, f)
		color := e.Tinta
		if relleno {
			color = e.Relleno
		}
		if color = strings.ToLower(color); color != "" && !slices.Contains(out, color) {
			out = append(out, color)
		}
	}
	return out
}

// abrirFiltroCond abre el formulario donde estaba el desplegable de la columna. Se vuelve a
// abrir —cerrando el anterior— cada vez que el operador cambia los campos que hacen falta,
// como el gestor de formato condicional.
func (c *Lienzo) abrirFiltroCond(f *formFiltro) {
	var em *ui.Emergente
	rehacer := func() {
		em.Cerrar()
		c.abrirFiltroCond(f)
	}
	cerrar := func() { em.Cerrar() }
	em = ui.MostrarEmergente(c.anclaFiltro, c.formularioFiltro(f, rehacer, cerrar), 0)
}

// formularioFiltro compone el formulario (`flex w-64 flex-col gap-1.5 px-2 py-1.5`).
func (c *Lienzo) formularioFiltro(f *formFiltro, rehacer, cerrar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := AnchoFormFiltro - 2*tema.Espacio(2)
	ops := make([]editor.Opcion, len(OpsFiltro))
	for i, o := range OpsFiltro {
		ops[i] = editor.Opcion{Valor: o, Rotulo: c.t("filtro.op."+o, nil)}
	}
	op := editor.NuevoSelectorCinta(c.t("filtro.byCondition", nil), ancho, ops, f.op, func(s string) {
		if s != f.op {
			f.op, f.color = s, ""
			rehacer()
		}
	})
	filas := []fyne.CanvasObject{ui.NuevoParrafo(ui.Versalitas(c.t("filtro.byCondition", nil)), tema.Texto2XS, ui.SemiNegrita, "text-subtle", v), op}

	aplicar := func() {
		if cond, ok := f.cond(); ok {
			c.ponerCondFiltro(f.col, &cond)
			cerrar()
		}
	}
	// «Aplicar» se enciende en cuanto el borrador vale, sin rehacer el formulario:
	// rehacerlo a cada letra se llevaría el foco del campo.
	ap := botonPop(c.t("action.apply", nil), false, aplicar)
	encender := func() {
		_, puede := f.cond()
		ap.Inerte = !puede
		ap.Refresh()
	}
	campo := func(valor string, poner func(string)) *ui.Entrada {
		e := ui.NuevaEntrada(c.t("common.value", nil)).ConPropia(campoCinta)
		e.PonerTexto(valor)
		e.AlCambiar(func(s string) { poner(s); encender() })
		e.AlTeclear = func(k *fyne.KeyEvent) bool { // Intro aplica, como `onKey` de la web
			if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
				aplicar()
				return true
			}
			return false
		}
		return e
	}
	k := OperandosFiltro(f.op)
	if k >= 1 {
		filas = append(filas, campo(f.a, func(s string) { f.a = s }))
	}
	if k == 2 {
		y := ui.NuevoParrafo(c.t("filtro.and", nil), tema.TextoXS, ui.Normal, "text-muted", v)
		filas = append(filas, container.New(fila{hueco: tema.Espacio(1.5), elastico: 1}, y, campo(f.b, func(s string) { f.b = s })))
	}
	if f.op == "top" || f.op == "bottom" {
		n := campo(f.n, func(s string) { f.n = s })
		modo := "n"
		if f.porcien {
			modo = "p"
		}
		tipo := editor.NuevoSelectorCinta(c.t("filtro.count", nil), anchoSelectorOp, []editor.Opcion{
			{Valor: "n", Rotulo: c.t("filtro.count", nil)}, {Valor: "p", Rotulo: c.t("filtro.percent", nil)},
		}, modo, func(s string) { f.porcien = s == "p"; encender() })
		filas = append(filas, container.New(fila{hueco: tema.Espacio(1.5), elastico: 0}, n, tipo))
	}
	if f.op == "fill" || f.op == "fontColor" {
		colores := c.Libro.ColoresFiltro(f.col, f.op == "fill")
		filas = append(filas, muestras(colores, f.color, c.t("filtro.color", nil), func(s string) { f.color = s; rehacer() }))
	}

	encender()
	botones := []fyne.CanvasObject{ap}
	if f.actual != nil {
		botones = append(botones, botonPop(c.t("filtro.clear", nil), false, func() {
			c.ponerCondFiltro(f.col, nil)
			cerrar()
		}))
	}
	botones = append(botones, botonPop(c.t("filtro.back", nil), false, func() {
		cerrar()
		c.abrirFiltro(f.col)
	}))
	filas = append(filas, container.New(fila{hueco: tema.Espacio(1.5), elastico: 0}, botones...))
	return container.New(ui.Margen{Izq: tema.Espacio(2), Der: tema.Espacio(2), Arr: tema.Espacio(1.5), Abj: tema.Espacio(1.5)},
		container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...))
}

// ponerCondFiltro manda `filtercond` con la condición de la columna (nil la quita).
func (c *Lienzo) ponerCondFiltro(col int, cond *CondFiltro) {
	b, _ := json.Marshal(map[string]any{"col": col, "cond": cond})
	c.Mandar(CmdCondFiltro, string(b))
}

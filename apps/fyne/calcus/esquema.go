// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El esquema (agrupar y contraer filas y columnas) y el asistente de Subtotales, como
// los comandos `outline` y `subtotal` de SheetCanvas.tsx. La lógica —qué nivel lleva un
// grupo, qué se oculta, dónde van los subtotales— es la de `apps/nucleo/formula/analisis`,
// la misma que la web con los mismos vectores; aquí solo se aplica al libro.

// Los comandos del esquema, con el nombre y el valor de la web.
const (
	CmdEsquema  editor.Comando = "outline"  // JSON {axis, op, start?, end?, level?}
	CmdSubtotal editor.Comando = "subtotal" // JSON {range, groupCol, fn, cols?, remove?}
)

// GruposFilas y GruposCols son el esquema de la hoja activa.
func (l *Libro) GruposFilas() []analisis.Grupo { return l.h().grupFilas }
func (l *Libro) GruposCols() []analisis.Grupo  { return l.h().grupCols }

// ColumnasOcultas son las del detalle de un grupo de columnas contraído; nil si ninguna.
func (l *Libro) ColumnasOcultas() []bool {
	var out []bool
	for c := range analisis.Ocultas(l.h().grupCols) {
		if c < Columnas {
			if out == nil {
				out = make([]bool, Columnas)
			}
			out[c] = true
		}
	}
	return out
}

// sincronizarOcultas le cuenta al motor qué filas no se ven en cada hoja, que es lo que
// SUBTOTAL y AGGREGATE necesitan: las del filtro y las de un grupo contraído. Es lo que
// hace `buildEngine` en la web al montar el motor. Se llama desde Valor y solo trabaja
// si el libro cambió; al motor solo se le avisa si cambió lo oculto, porque avisarle
// vacía su caché.
func (l *Libro) sincronizarOcultas() {
	if l.ocultas.version == l.version && l.ocultas.por != nil {
		return
	}
	l.ocultas.version = l.version // antes de leer valores: filtradasDe los lee
	if l.ocultas.por == nil {
		l.ocultas.por = map[string]string{}
	}
	l.ocultas.listas = map[string][2][]int{}
	for i, h := range l.f.hojas {
		filtradas := l.filtradasDe(i)
		var manuales []int
		for f := range analisis.Ocultas(h.grupFilas) {
			manuales = append(manuales, f)
		}
		slices.Sort(manuales)
		if len(filtradas)+len(manuales) > 0 {
			l.ocultas.listas[h.nombre] = [2][]int{filtradas, manuales}
		}
		clave := fmt.Sprint(filtradas, manuales)
		if antes, ok := l.ocultas.por[h.nombre]; ok && antes == clave || !ok && len(filtradas)+len(manuales) == 0 {
			l.ocultas.por[h.nombre] = clave
			continue
		}
		l.ocultas.por[h.nombre] = clave
		l.motor.SetHiddenRows(h.nombre, filtradas, manuales)
	}
}

// peticionEsquema es el valor del comando `outline`.
type peticionEsquema struct {
	Axis  string `json:"axis"`
	Op    string `json:"op"`
	Start any    `json:"start"`
	End   any    `json:"end"`
	Level any    `json:"level"`
}

// Esquema es el comando `outline`: agrupa o desagrupa las filas o columnas del rango,
// contrae o expande un grupo, enseña hasta un nivel o lo quita todo. Devuelve si cambió
// algo; `sinRango` dice que se pidió agrupar una sola fila o columna, que no agrupa nada
// y la web lo avisa.
func (l *Libro) Esquema(valor string, c1, f1, c2, f2 int) (cambio, sinRango bool) {
	var req peticionEsquema
	if json.Unmarshal([]byte(valor), &req) != nil {
		return false, false
	}
	h := l.h()
	grupos, tope, a, b := h.grupFilas, Filas, f1, f2
	if req.Axis == "col" {
		grupos, tope, a, b = h.grupCols, Columnas, c1, c2
	}
	var nuevos []analisis.Grupo
	switch req.Op {
	case "group":
		if a == b {
			return false, true
		}
		nuevos = analisis.Agrupar(grupos, a, b, tope)
	case "ungroup":
		nuevos = analisis.Desagrupar(grupos, a, b)
	case "clear":
	case "level":
		n := enteroJS(req.Level)
		if n < 1 {
			return false, false
		}
		nuevos = analisis.HastaNivel(grupos, n)
	case "toggle":
		nuevos = analisis.Conmutar(grupos, enteroJS(req.Start), enteroJS(req.End), enteroJS(req.Level))
	default:
		return false, false
	}
	if len(nuevos) == 0 {
		nuevos = nil // vacío es «sin esquema»: no se guarda
	}
	l.anotar()
	l.version++
	if req.Axis == "col" {
		h.grupCols = nuevos
	} else {
		h.grupFilas = nuevos
	}
	return true, false
}

// peticionSubtotal es el valor del comando `subtotal`.
type peticionSubtotal struct {
	Range    string `json:"range"`
	GroupCol any    `json:"groupCol"` //nolint:tagliatelle // el nombre lo fija la web
	Fn       string `json:"fn"`
	Cols     []any  `json:"cols"`
	Remove   bool   `json:"remove"`
}

// Subtotales es el comando `subtotal`: el asistente entero, o «Quitar todos». Devuelve
// el resultado (para la selección y el aviso) o el error de la web ("no-data",
// "no-room"); "" y cero si el valor no vale.
func (l *Libro) Subtotales(valor string, rotulos func(clave string, vars i18n.Vars) string) (analisis.ResultadoSubtotal, analisis.PeticionSubtotal, string) {
	var req peticionSubtotal
	if json.Unmarshal([]byte(valor), &req) != nil {
		return analisis.ResultadoSubtotal{}, analisis.PeticionSubtotal{}, "invalid"
	}
	r, ok := formula.ParseRangeRef(req.Range)
	if !ok || r.C1 >= Columnas || r.R1 >= Filas {
		return analisis.ResultadoSubtotal{}, analisis.PeticionSubtotal{}, "invalid"
	}
	c2 := min(r.C2, Columnas-1)
	grupo, fn := 0, "sum"
	if !req.Remove {
		grupo, fn = enteroJS(req.GroupCol), req.Fn
	}
	if _, ok := analisis.NumeroSubtotal[analisis.Agregacion(fn)]; !ok || grupo < 0 || grupo > c2-r.C1 {
		return analisis.ResultadoSubtotal{}, analisis.PeticionSubtotal{}, "invalid"
	}
	p := analisis.PeticionSubtotal{
		C1: r.C1, F1: r.R1, C2: c2, F2: r.R2, ColGrupo: r.C1 + grupo, Fn: analisis.Agregacion(fn),
		Quitar: req.Remove, Filas: Filas, UltimaUsada: -1,
		RotuloTotal: rotulos("sheet.total", nil), RotuloGrande: rotulos("sheet.total.grand", nil),
		RotuloDe: func(k string) string { return rotulos("sheet.total.of", i18n.Vars{"key": k}) },
	}
	if req.Cols != nil {
		p.Cols = []int{}
		for _, x := range req.Cols {
			if c := r.C1 + enteroJS(x); c >= r.C1 && c <= c2 {
				p.Cols = append(p.Cols, c)
			}
		}
	}
	h := l.h()
	for k := range h.celdas {
		p.UltimaUsada = max(p.UltimaUsada, k.F)
	}
	res, fallo := analisis.AplicarSubtotales(p, analisis.Lector{
		Crudo: func(c, f int) string { return textoPlano(h.celdas[Celda{C: c, F: f}]) },
		Clave: l.textoFiltro,
		EsNumero: func(c, f int) bool {
			n, ok := l.Valor(c, f).(float64)
			return ok && !math.IsNaN(n) && !math.IsInf(n, 0)
		},
		TieneAlgo: func(c, f int) bool { return l.textoFiltro(c, f) != "" },
	})
	if fallo != "" {
		return res, p, fallo
	}
	l.anotar()
	l.aplicarSubtotales(res)
	return res, p, ""
}

// aplicarSubtotales mueve cada fila a donde dice el resultado (con su formato),
// escribe las celdas de subtotal y pone el esquema.
func (l *Libro) aplicarSubtotales(res analisis.ResultadoSubtotal) {
	h := l.h()
	viejas, formatos, estilos := h.celdas, h.formatos, h.estilos
	h.celdas, h.formatos, h.estilos = map[Celda]string{}, map[Celda]FormatoNum{}, map[Celda]Estilo{}
	for a, de := range res.Desde {
		if de < 0 {
			continue
		}
		for c := range Columnas {
			k, n := Celda{C: c, F: de}, Celda{C: c, F: a}
			if v, ok := viejas[k]; ok {
				h.celdas[n] = v
			}
			if v, ok := formatos[k]; ok {
				h.formatos[n] = v
			}
			if v, ok := estilos[k]; ok {
				h.estilos[n] = v
			}
		}
	}
	for _, w := range res.Escrituras {
		h.celdas[Celda{C: w.C, F: w.F}] = "<b>" + escaparHTML(w.Texto) + "</b>"
	}
	h.grupFilas = nil
	if len(res.Grupos) > 0 {
		h.grupFilas = res.Grupos // el subtotal DEFINE el esquema de filas
	}
	l.restaurar(l.f)
}

// mandarEsquema ejecuta `outline` y `subtotal`. false si no es uno de ellos.
func (c *Lienzo) mandarEsquema(cmd editor.Comando, valor string, c1, f1, c2, f2 int) bool {
	r := c.Rejilla
	switch cmd {
	case CmdEsquema:
		cambio, sinRango := c.Libro.Esquema(valor, c1, f1, c2, f2)
		if sinRango && c.AlAviso != nil {
			var req peticionEsquema
			_ = json.Unmarshal([]byte(valor), &req)
			det := "sheet.msg.pickRange.rows"
			if req.Axis == "col" {
				det = "sheet.msg.pickRange.cols"
			}
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.pickRange", nil), c.t(det, nil))
		}
		return cambio
	case CmdSubtotal:
		res, p, fallo := c.Libro.Subtotales(valor, c.t)
		switch fallo {
		case "":
		case "no-data", "no-room":
			if c.AlAviso != nil {
				t, d := "sheet.msg.noData", "sheet.msg.noDataRows"
				if fallo == "no-room" {
					t, d = "sheet.msg.noRoom", "sheet.msg.noRoomSubtotals"
				}
				c.AlAviso(ui.AvisoAdvertencia, c.t(t, nil), c.t(d, nil))
			}
			return false
		default:
			return false
		}
		r.sel, r.ext = Celda{C: p.C1, F: p.F1}, &Celda{C: p.C2, F: res.UltimaFila}
		if !p.Quitar && c.AlAviso != nil {
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.subtotalsCreated", nil),
				c.t("sheet.msg.subtotalsCreated.detail", i18n.Vars{"count": res.Bloques, "col": formula.NumToCol(p.ColGrupo)}))
		}
		return true
	default:
		return false
	}
}

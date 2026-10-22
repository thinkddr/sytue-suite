// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los comandos de los objetos de la hoja, con el nombre y el valor de la web.
const (
	CmdTablaAnadir  editor.Comando = "tableadd"     // JSON {range, headers, style}
	CmdTablaQuitar  editor.Comando = "tabledel"     // la tabla de la celda ancla
	CmdTablaEstilo  editor.Comando = "tablestyle"   // el id del estilo
	CmdTablaTotales editor.Comando = "tabletotals"  // pone o quita la fila de totales
	CmdTablaFuncion editor.Comando = "tabletotalfn" // JSON {name, col, fn}
	CmdMiniAnadir   editor.Comando = "sparkadd"     // JSON {range, type, color, cell}
	CmdMiniQuitar   editor.Comando = "sparkdel"     // la celda anfitriona
	CmdGrafico      editor.Comando = "chartadd"     // JSON {type, range}
	CmdSegAnadir    editor.Comando = "sliceradd"    // JSON {range, col}
)

// mandarObjetos son los comandos de tablas, minigráficos, gráficos y segmentaciones.
// false si no es uno de ellos.
//
//nolint:gocyclo // un caso por comando, como el despacho de la web
func (c *Lienzo) mandarObjetos(cmd editor.Comando, valor string) bool {
	l, s := c.Libro, c.Rejilla.sel
	_, idx := l.TablaEn(s.C, s.F)
	switch cmd {
	case CmdTablaAnadir:
		var req struct {
			Range   string `json:"range"`
			Headers *bool  `json:"headers"`
			Style   string `json:"style"`
		}
		if json.Unmarshal([]byte(valor), &req) != nil {
			return false
		}
		t, ok := l.CrearTabla(req.Range, req.Headers == nil || *req.Headers, req.Style)
		if !ok {
			return false
		}
		if r, ok := celdasDeRango(t.Range); ok {
			c.Rejilla.sel, c.Rejilla.ext = Celda{C: r[0], F: r[1]}, &Celda{C: r[2], F: r[3]}
		}
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.tableCreated", nil),
				c.t("sheet.msg.tableCreated.detail", i18n.Vars{"name": t.Name, "range": t.Range}))
		}
	case CmdTablaQuitar:
		l.QuitarTabla(idx)
	case CmdTablaEstilo:
		l.EstiloDeTabla(idx, valor)
	case CmdTablaTotales:
		if !l.AlternarTotales(idx) && c.AlAviso != nil {
			c.AlAviso(ui.AvisoAdvertencia, c.t("sheet.msg.noRoom", nil), c.t("sheet.msg.noRoomTotals", nil))
		}
	case CmdTablaFuncion:
		var req struct {
			Name string `json:"name"`
			Col  any    `json:"col"`
			Fn   string `json:"fn"`
		}
		if json.Unmarshal([]byte(valor), &req) != nil {
			return false
		}
		col, ok := numeroEntero(req.Col)
		return ok && l.FuncionTotal(req.Name, col, req.Fn)
	case CmdMiniAnadir:
		var crudo minigraficoCrudo
		if json.Unmarshal([]byte(valor), &crudo) != nil {
			return false
		}
		if crudo.Cell == nil { // `{ cell: refOf(cur.sel), ...parsed }`
			crudo.Cell = Ref(s.C, s.F)
		}
		return l.AnadirMinigrafico(crudo)
	case CmdMiniQuitar:
		return l.QuitarMinigrafico(valor)
	case CmdGrafico:
		var crudo graficoCrudo
		if json.Unmarshal([]byte(valor), &crudo) != nil {
			return false
		}
		return l.AnadirGrafico(crudo)
	case CmdSegAnadir:
		return c.anadirSegmentacion(valor)
	default:
		return false
	}
	return true
}

// celdasDeRango es un rango A1 como [c1, f1, c2, f2].
func celdasDeRango(rango string) ([4]int, bool) {
	r, ok := formula.ParseRangeRef(rango)
	return [4]int{r.C1, r.R1, r.C2, r.R2}, ok
}

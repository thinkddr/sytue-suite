// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El ANÁLISIS DE HIPÓTESIS en el libro: los comandos `goalseek`, `solverapply`,
// `datatable` y los de escenarios de SheetCanvas.tsx, y las sondas que tantean sobre
// un motor DESECHABLE —el libro no se toca hasta que se aplica—. Los cálculos son los
// de `apps/nucleo/formula/analisis`, los mismos que la web con los mismos vectores.

// Los comandos del análisis de hipótesis, con el nombre y el valor de la web.
const (
	CmdObjetivo      editor.Comando = "goalseek"    // JSON {cell, value}
	CmdSolver        editor.Comando = "solverapply" // JSON {values: {ref: número}}
	CmdTablaDatos    editor.Comando = "datatable"   // JSON {range, rowInput?, colInput?}
	CmdEscAnadir     editor.Comando = "scenadd"     // JSON {name, range}
	CmdEscActualizar editor.Comando = "scenupdate"  // nombre
	CmdEscAplicar    editor.Comando = "scenapply"   // nombre
	CmdEscQuitar     editor.Comando = "scendel"     // nombre
	CmdEscInforme    editor.Comando = "scenreport"  // JSON {target, results}
)

// Escenarios son los de la hoja activa.
func (l *Libro) Escenarios() []analisis.Escenario { return l.h().escenarios }

// motorDesechable es `buildEngine(model)`: un motor nuevo con todo el libro y las filas
// ocultas, para tantear sin tocar el de verdad.
func (l *Libro) motorDesechable() *formula.Engine {
	l.sincronizarOcultas()
	m := formula.New(formula.Options{Sheets: l.Hojas(), ResolveTable: l.resolverTabla})
	for _, h := range l.f.hojas {
		for k, html := range h.celdas {
			m.SetCellAt(h.nombre, k.C, k.F, textoPlano(html))
		}
		if o, ok := l.ocultas.listas[h.nombre]; ok {
			m.SetHiddenRows(h.nombre, o[0], o[1])
		}
	}
	return m
}

// numeroMotor es el valor de una celda como número; false si no lo es.
func numeroMotor(m *formula.Engine, hoja string, c, f int) (float64, bool) {
	n, ok := m.ValueAt(hoja, c, f).(float64)
	return n, ok && !math.IsNaN(n) && !math.IsInf(n, 0)
}

// celdaUnica es `singleCellRef`: la celda de un texto que es UNA celda.
func celdaUnica(texto string) (Celda, bool) {
	r, ok := formula.ParseRangeRef(texto)
	if !ok || r.C1 != r.C2 || r.R1 != r.R2 {
		return Celda{}, false
	}
	return Celda{C: r.C1, F: r.R1}, true
}

// jsNumero es `String(x)`: como la web escribe el número hallado en la celda.
func jsNumero(x float64) string { return formula.NumeroJS(x) }

// SondaObjetivo es `makeGoalSeekProbe`: la función que fija la variable y lee el
// objetivo, y el valor actual de la variable. El motivo es el de la web si no vale.
func (l *Libro) SondaObjetivo(objetivo, variable string) (func(float64) (float64, bool), float64, string) {
	o, ok := celdaUnica(objetivo)
	if !ok {
		return nil, 0, "bad-target"
	}
	v, ok := celdaUnica(variable)
	if !ok {
		return nil, 0, "bad-var"
	}
	if !esFormula(l.HTML(o.C, o.F)) {
		return nil, 0, "target-not-formula"
	}
	if esFormula(l.HTML(v.C, v.F)) {
		return nil, 0, "var-is-formula"
	}
	m, hoja := l.motorDesechable(), l.Hoja()
	actual, _ := numeroMotor(m, hoja, v.C, v.F)
	return func(x float64) (float64, bool) {
		m.SetCellAt(hoja, v.C, v.F, jsNumero(x))
		return numeroMotor(m, hoja, o.C, o.F)
	}, actual, ""
}

// RestriccionTexto es una fila del Solver tal y como se teclea.
type RestriccionTexto struct{ Izq, Rel, Der string }

// SondaSolver es `makeSolverProbe`. El lado derecho es una celda (se mira primero) o un
// número es-ES; se resuelven en cada evaluación, porque una celda puede depender de
// las variables.
func (l *Libro) SondaSolver(objetivo string, variables []string, rs []RestriccionTexto) (func([]float64) analisis.EvaluacionSolver, []float64, string) {
	o, ok := celdaUnica(objetivo)
	if !ok {
		return nil, nil, "bad-target"
	}
	if !esFormula(l.HTML(o.C, o.F)) {
		return nil, nil, "target-not-formula"
	}
	if len(variables) == 0 {
		return nil, nil, "no-vars"
	}
	vars := make([]Celda, 0, len(variables))
	for _, v := range variables {
		c, ok := celdaUnica(v)
		if !ok {
			return nil, nil, "bad-var"
		}
		if esFormula(l.HTML(c.C, c.F)) {
			return nil, nil, "var-is-formula"
		}
		vars = append(vars, c)
	}
	type lado struct {
		celda  *Celda
		numero float64
	}
	izq, der := make([]Celda, 0, len(rs)), make([]lado, 0, len(rs))
	for _, r := range rs {
		c, ok := celdaUnica(r.Izq)
		if !ok {
			return nil, nil, "bad-constraint"
		}
		izq = append(izq, c)
		if d, ok := celdaUnica(r.Der); ok {
			der = append(der, lado{celda: &d})
			continue
		}
		if _, esRango := formula.ParseRangeRef(r.Der); esRango {
			return nil, nil, "bad-constraint"
		}
		n, ok := numeroEsValor(r.Der)
		if !ok {
			return nil, nil, "bad-constraint"
		}
		der = append(der, lado{numero: n})
	}
	m, hoja := l.motorDesechable(), l.Hoja()
	num := func(c Celda) float64 {
		if n, ok := numeroMotor(m, hoja, c.C, c.F); ok {
			return n
		}
		return math.NaN()
	}
	actual := make([]float64, len(vars))
	for i, c := range vars {
		if actual[i] = num(c); math.IsNaN(actual[i]) {
			actual[i] = 0
		}
	}
	return func(x []float64) analisis.EvaluacionSolver {
		for i, c := range vars {
			m.SetCellAt(hoja, c.C, c.F, jsNumero(x[i]))
		}
		ev := analisis.EvaluacionSolver{Objetivo: num(o)}
		for i := range izq {
			d := der[i].numero
			if der[i].celda != nil {
				d = num(*der[i].celda)
			}
			ev.Restricciones = append(ev.Restricciones, [2]float64{num(izq[i]), d})
		}
		return ev
	}, actual, ""
}

// escribirCrudos escribe textos crudos conservando el formato de cada celda
// (`applyScenarioValues`) en UN paso de deshacer.
func (l *Libro) escribirCrudos(valores map[Celda]string) bool {
	cambia := false
	for k, v := range valores {
		if conTexto(l.HTML(k.C, k.F), v) != l.HTML(k.C, k.F) {
			cambia = true
		}
	}
	if !cambia {
		return false
	}
	l.anotar()
	for k, v := range valores {
		l.poner(l.f.activa, k.C, k.F, conTexto(l.HTML(k.C, k.F), v))
	}
	return true
}

// mandarHipotesis ejecuta los comandos del análisis de hipótesis. false si no es uno.
func (c *Lienzo) mandarHipotesis(cmd editor.Comando, valor string) bool {
	switch cmd {
	case CmdObjetivo:
		var req struct {
			Cell  string `json:"cell"`
			Value any    `json:"value"`
		}
		n, okn := 0.0, false
		if json.Unmarshal([]byte(valor), &req) == nil {
			n, okn = req.Value.(float64)
		}
		k, ok := celdaUnica(req.Cell)
		if !ok || !okn || math.IsNaN(n) || math.IsInf(n, 0) {
			return false
		}
		c.Libro.escribirCrudos(map[Celda]string{k: jsNumero(n)})
		c.Rejilla.sel, c.Rejilla.ext = k, nil
		return true
	case CmdSolver:
		var req struct {
			Values map[string]any `json:"values"`
		}
		_ = json.Unmarshal([]byte(valor), &req)
		vals := map[Celda]string{}
		for ref, v := range req.Values {
			n, ok := v.(float64)
			if k, okc := celdaUnica(ref); okc && ok && !math.IsNaN(n) && !math.IsInf(n, 0) {
				vals[k] = jsNumero(n)
			}
		}
		if len(vals) == 0 {
			return false
		}
		return c.Libro.escribirCrudos(vals)
	case CmdTablaDatos:
		return c.tablaDatos(valor)
	default:
		return c.mandarEscenario(cmd, valor)
	}
}

// tablaDatos es el comando `datatable`: fija las entradas a cada cabecera en un motor
// desechable y escribe los resultados como valores.
func (c *Lienzo) tablaDatos(valor string) bool {
	var req struct {
		Range    string `json:"range"`
		RowInput string `json:"rowInput"` //nolint:tagliatelle // el nombre lo fija la web
		ColInput string `json:"colInput"` //nolint:tagliatelle // el nombre lo fija la web
	}
	if json.Unmarshal([]byte(valor), &req) != nil {
		return false
	}
	r, ok := formula.ParseRangeRef(req.Range)
	if !ok || r.C1 >= Columnas || r.R1 >= Filas {
		return false
	}
	fila, conFila := celdaUnica(req.RowInput)
	col, conCol := celdaUnica(req.ColInput)
	plan, fallo := analisis.PlanearTabla(r.C1, r.R1, min(r.C2, Columnas-1), min(r.R2, Filas-1), conFila, conCol)
	if fallo != "" {
		c.aviso(ui.AvisoAdvertencia, "sheet.msg.dataTable", map[string]string{
			"no-input": "sheet.msg.dataTable.noInput", "too-small": "sheet.msg.dataTable.tooSmall", "too-many": "sheet.msg.dataTable.tooMany",
		}[fallo], nil)
		return false
	}
	l := c.Libro
	fc, ff := plan.Formula[0], plan.Formula[1]
	if !esFormula(l.HTML(fc, ff)) {
		c.aviso(ui.AvisoAdvertencia, "sheet.msg.missingFormula", "sheet.msg.missingFormula.detail", i18n.Vars{"ref": Ref(fc, ff)})
		return false
	}
	m, hoja := l.motorDesechable(), l.Hoja()
	valores := map[Celda]string{}
	for _, cp := range plan.Celdas {
		if cp.FilaDe != nil {
			m.SetCellAt(hoja, fila.C, fila.F, l.Crudo(cp.FilaDe[0], cp.FilaDe[1]))
		}
		if cp.ColDe != nil {
			m.SetCellAt(hoja, col.C, col.F, l.Crudo(cp.ColDe[0], cp.ColDe[1]))
		}
		valores[Celda{C: cp.C, F: cp.F}] = formula.Display(m.ValueAt(hoja, fc, ff))
	}
	// Como la web: el resultado sustituye a la celda entera (`escapeHtml(txt)`), sin
	// conservar su envoltorio.
	l.anotar()
	for k, txt := range valores {
		l.poner(l.f.activa, k.C, k.F, escaparHTML(txt))
	}
	c.Rejilla.sel, c.Rejilla.ext = Celda{C: r.C1, F: r.R1}, &Celda{C: min(r.C2, Columnas-1), F: min(r.R2, Filas-1)}
	c.aviso(ui.AvisoInfo, "sheet.msg.dataTable", "sheet.msg.dataTable.detail", i18n.Vars{"count": len(plan.Celdas), "ref": Ref(fc, ff)})
	return true
}

// aviso enseña un aviso con título y detalle del catálogo, si hay quien lo enseñe.
func (c *Lienzo) aviso(tono ui.TonoAviso, titulo, detalle string, vars i18n.Vars) {
	if c.AlAviso != nil {
		c.AlAviso(tono, c.t(titulo, nil), c.t(detalle, vars))
	}
}

// mandarEscenario son los cinco comandos de escenarios.
func (c *Lienzo) mandarEscenario(cmd editor.Comando, valor string) bool {
	l := c.Libro
	h := l.h()
	nombre := strings.TrimSpace(valor)
	i := -1
	for j, e := range h.escenarios {
		if e.Name == nombre {
			i = j
		}
	}
	switch cmd {
	case CmdEscAnadir:
		return c.anadirEscenario(valor)
	case CmdEscActualizar:
		if i < 0 {
			return false
		}
		vals := map[string]string{}
		for ref := range h.escenarios[i].Values {
			if k, ok := celdaUnica(ref); ok {
				vals[ref] = l.Crudo(k.C, k.F)
			}
		}
		l.cambiarEscenarios(func(es []analisis.Escenario) []analisis.Escenario {
			es[i] = analisis.Escenario{Name: nombre, Values: vals}
			return es
		})
		c.aviso(ui.AvisoInfo, "sheet.msg.scenarioUpdated", "sheet.msg.scenarioUpdated.detail", i18n.Vars{"name": nombre})
	case CmdEscAplicar:
		if i < 0 {
			return false
		}
		vals := map[Celda]string{}
		for ref, v := range h.escenarios[i].Values {
			if k, ok := celdaUnica(ref); ok {
				vals[k] = v
			}
		}
		l.escribirCrudos(vals)
		c.aviso(ui.AvisoInfo, "sheet.msg.scenarioApplied", "sheet.msg.scenarioApplied.detail", i18n.Vars{"name": nombre})
	case CmdEscQuitar:
		if i < 0 {
			return false
		}
		l.cambiarEscenarios(func(es []analisis.Escenario) []analisis.Escenario { return append(es[:i:i], es[i+1:]...) })
	case CmdEscInforme:
		return c.informeEscenarios(valor)
	default:
		return false
	}
	return true
}

// cambiarEscenarios sustituye la lista de la hoja activa en un paso de deshacer.
func (l *Libro) cambiarEscenarios(f func([]analisis.Escenario) []analisis.Escenario) {
	l.anotar()
	l.version++
	h := l.h()
	h.escenarios = f(append([]analisis.Escenario(nil), h.escenarios...))
	if len(h.escenarios) == 0 {
		h.escenarios = nil
	}
}

// anadirEscenario es `scenadd`: captura los crudos actuales de las celdas del rango.
func (c *Lienzo) anadirEscenario(valor string) bool {
	var req struct {
		Name  string `json:"name"`
		Range string `json:"range"`
	}
	if json.Unmarshal([]byte(valor), &req) != nil {
		return false
	}
	nombre := strings.TrimSpace(req.Name)
	r, ok := formula.ParseRangeRef(req.Range)
	if nombre == "" || !ok {
		return false
	}
	if (r.C2-r.C1+1)*(r.R2-r.R1+1) > analisis.TopeEscenario {
		c.aviso(ui.AvisoAdvertencia, "sheet.msg.tooManyCells", "sheet.msg.tooManyCells.detail", i18n.Vars{"max": analisis.TopeEscenario})
		return false
	}
	l := c.Libro
	for _, e := range l.h().escenarios {
		if e.Name == nombre {
			c.aviso(ui.AvisoAdvertencia, "sheet.msg.nameTaken", "sheet.msg.nameTaken.detail", i18n.Vars{"name": nombre})
			return false
		}
	}
	vals := map[string]string{}
	for f := r.R1; f <= min(r.R2, Filas-1); f++ {
		for col := r.C1; col <= min(r.C2, Columnas-1); col++ {
			vals[Ref(col, f)] = l.Crudo(col, f)
		}
	}
	l.cambiarEscenarios(func(es []analisis.Escenario) []analisis.Escenario {
		return append(es, analisis.Escenario{Name: nombre, Values: vals})
	})
	c.aviso(ui.AvisoInfo, "sheet.msg.scenarioCreated", "sheet.msg.scenarioCreated.detail", i18n.Vars{"name": nombre, "count": len(vals)})
	return true
}

// informeEscenarios es `scenreport`: el informe resumen desde la celda de destino.
func (c *Lienzo) informeEscenarios(valor string) bool {
	var req struct {
		Target  string   `json:"target"`
		Results []string `json:"results"`
	}
	l := c.Libro
	esc := l.h().escenarios
	if json.Unmarshal([]byte(valor), &req) != nil || len(esc) == 0 {
		return false
	}
	destino, ok := formula.ParseRangeRef(req.Target)
	if !ok {
		return false
	}
	actual := map[string]string{}
	for _, e := range esc {
		for ref := range e.Values {
			if k, okc := celdaUnica(ref); okc {
				actual[ref] = l.Crudo(k.C, k.F)
			}
		}
	}
	hoja := l.Hoja()
	rejilla := analisis.InformeEscenarios(esc, req.Results, func(fijar map[string]string, refs []string) map[string]string {
		m := l.motorDesechable()
		for ref, v := range fijar {
			if k, okc := celdaUnica(ref); okc {
				m.SetCellAt(hoja, k.C, k.F, v)
			}
		}
		out := map[string]string{}
		for _, ref := range refs {
			if k, okc := celdaUnica(ref); okc {
				out[ref] = formula.Display(m.ValueAt(hoja, k.C, k.F))
			}
		}
		return out
	}, analisis.RotulosInforme{
		Titulo: c.t("sheet.scenario.report", nil), Cambiantes: c.t("sheet.scenario.changingCells", nil),
		Resultados: c.t("sheet.scenario.resultCellsLabel", nil), Actual: c.t("sheet.scenario.current", nil),
	}, actual)
	l.escribirRejilla(rejilla, Celda{C: destino.C1, F: destino.R1})
	ancho := 1
	if len(rejilla) > 0 {
		ancho = len(rejilla[0])
	}
	c.Rejilla.sel = Celda{C: destino.C1, F: destino.R1}
	c.Rejilla.ext = &Celda{C: min(Columnas-1, destino.C1+ancho-1), F: min(Filas-1, destino.R1+len(rejilla)-1)}
	c.aviso(ui.AvisoInfo, "sheet.msg.scenarioReport", "sheet.msg.scenarioReport.detail", i18n.Vars{"count": len(esc), "ref": Ref(destino.C1, destino.R1)})
	return true
}

// escribirRejilla escribe una rejilla de texto desde la esquina, con la primera fila y
// la primera columna en negrita, como el informe de la web. Un texto vacío borra.
func (l *Libro) escribirRejilla(rejilla [][]string, esquina Celda) {
	l.anotar()
	for i, fila := range rejilla {
		for j, txt := range fila {
			k := Celda{C: esquina.C + j, F: esquina.F + i}
			if k.C >= Columnas || k.F >= Filas {
				continue
			}
			html := ""
			if txt != "" {
				html = escaparHTML(txt)
				if i == 0 || j == 0 {
					html = "<b>" + html + "</b>"
				}
			}
			l.poner(l.f.activa, k.C, k.F, html)
		}
	}
}

// numeroEsValor es `parseEsNumber`: el número de lo escrito en español (coma decimal si
// es la última), o false.
func numeroEsValor(texto string) (float64, bool) {
	limpio := strings.Map(func(r rune) rune {
		if strings.ContainsRune("0123456789.,-", r) {
			return r
		}
		return -1
	}, texto)
	if limpio == "" || limpio == "-" {
		return 0, false
	}
	if strings.LastIndex(limpio, ",") > strings.LastIndex(limpio, ".") {
		limpio = strings.ReplaceAll(strings.ReplaceAll(limpio, ".", ""), ",", ".")
	} else {
		limpio = strings.ReplaceAll(limpio, ",", "")
	}
	n, err := strconv.ParseFloat(limpio, 64)
	return n, err == nil && !math.IsInf(n, 0)
}

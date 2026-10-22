// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La TABLA DINÁMICA en el libro: los comandos `pivot`, `pivotrefresh` y `pivotdel` de
// la web. El cálculo es `analisis.CalcularDinamica`, el mismo que la web con los mismos
// vectores; aquí se lee el origen del motor y se escribe la rejilla como celdas
// normales, igual que `writePivot` (`sheets/pivot-hoja.ts`).

// Los comandos de la tabla dinámica.
const (
	CmdDinamica         editor.Comando = "pivot"        // JSON de una SheetPivot (sin `name`: una nueva)
	CmdDinamicaAct      editor.Comando = "pivotrefresh" // nombre, o "" para todas las de la hoja
	CmdDinamicaQuitarla editor.Comando = "pivotdel"     // nombre
)

// Dinamicas son las tablas dinámicas de la hoja activa.
func (l *Libro) Dinamicas() []analisis.Dinamica { return l.h().dinamicas }

// DinamicaEn es la tabla dinámica cuya salida cubre la celda, o nil.
func (l *Libro) DinamicaEn(c, f int) *analisis.Dinamica {
	for i, d := range l.h().dinamicas {
		rango := d.Range
		if rango == "" {
			rango = d.Anchor
		}
		if r, ok := formula.ParseRangeRef(rango); ok && c >= r.C1 && c <= r.C2 && f >= r.R1 && f <= r.R2 {
			return &l.h().dinamicas[i]
		}
	}
	return nil
}

// sanearDinamica es `sanitizePivot` con el saneado de formatos de este paquete.
func sanearDinamica(crudo any) (analisis.Dinamica, bool) {
	return analisis.SanearDinamica(crudo, sanearFmtDinamica, func(s string) bool {
		_, ok := formula.ParseRangeRef(s)
		return ok
	})
}

// sanearFmtDinamica es `sanitizeNumFmt` sobre lo que traiga el JSON.
func sanearFmtDinamica(x any) (any, bool) {
	if x == nil {
		return nil, false
	}
	b, _ := json.Marshal(x)
	var c formatoCrudo
	if json.Unmarshal(b, &c) != nil {
		return nil, false
	}
	f, ok := c.sanear()
	return f, ok
}

// formatoValor es el formato de un campo de valores como FormatoNum (nil si no lleva).
func formatoValor(v analisis.ValorDinamica) *FormatoNum {
	switch f := v.Fmt.(type) {
	case FormatoNum:
		return &f
	case nil:
		return nil
	}
	if x, ok := sanearFmtDinamica(v.Fmt); ok {
		if f, es := x.(FormatoNum); es {
			return &f
		}
	}
	return nil
}

// rotulosDinamica son los textos de la tabla con el catálogo, como `pivotLabels`: los
// meses y la fecha, de las máscaras de la hoja (las mismas que la web).
func rotulosDinamica(t func(string, i18n.Vars) string) analisis.RotulosDinamica {
	nombreFn := map[string]string{
		"sum": "query.agg.sum", "count": "pivot.agg.count", "avg": "query.agg.avg", "max": "query.agg.max",
		"min": "query.agg.min", "product": "pivot.agg.product", "countNums": "pivot.agg.countNums",
		"stdev": "pivot.agg.stdev", "stdevp": "pivot.agg.stdevp", "var": "pivot.agg.var", "varp": "pivot.agg.varp",
	}
	return analisis.RotulosDinamica{
		Blank: t("pivot.blank", nil), Grand: t("sheet.total.grand", nil),
		All: t("pivot.all", nil), Multiple: t("pivot.multiple", nil),
		TotalOf: func(k string) string { return t("sheet.total.of", i18n.Vars{"key": k}) },
		ValueName: func(fn, campo string) string {
			return t("pivot.valueOf", i18n.Vars{"fn": t(nombreFn[fn], nil), "field": campo})
		},
		Quarter: func(q int) string { return t("pivot.quarter", i18n.Vars{"n": q}) },
		Month: func(m int) string {
			serie := time.Date(2001, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Sub(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)).Hours() / 24
			s, _ := conMascara("mmm", serie)
			return s
		},
		Day: func(d float64) string {
			s, _ := aplicarFormato(FormatoNum{ID: "date"}, d)
			return s
		},
		Show: func(v formula.Value, _ int) string {
			s, _ := textoConFormato(v, nil)
			return s
		},
	}
}

// porciento es el formato de «mostrar valores como»: porcentaje con dos decimales.
var porciento = func() FormatoNum { d := 2; return FormatoNum{ID: "percent", Decimales: &d} }()

// EscribirDinamica es `writePivot`: calcula la tabla d sobre la hoja activa y la
// escribe. "" si fue bien; si no, el error de la web ("source", "overlap", "room").
// No anota: lo hace quien la llama, una vez por comando.
func (l *Libro) EscribirDinamica(d analisis.Dinamica, t func(string, i18n.Vars) string) string {
	h := l.h()
	fuente := l.hojaOrigen(d)
	rejilla, formatoCampo, ok1 := l.origenDinamica(d)
	rr, _ := formula.ParseRangeRef(d.Source)
	at, ok2 := formula.ParseRangeRef(d.Anchor)
	if !ok1 || !ok2 {
		return "source"
	}
	c2, f2 := min(rr.C2, Columnas-1), min(rr.R2, Filas-1)
	ancho := 0
	if len(rejilla) > 0 {
		ancho = len(rejilla[0])
	}
	valores := slices.DeleteFunc(slices.Clone(d.Values), func(v analisis.ValorDinamica) bool { return v.Field >= ancho })
	out := analisis.CalcularDinamica(rejilla, d.DefDinamica, rotulosConFormato(t, formatoCampo))
	alto, w := len(out), 1
	for _, fila := range out {
		w = max(w, len(fila))
	}
	if at.C1+w > Columnas || at.R1+alto > Filas {
		return "room"
	}
	fin := Celda{C: at.C1 + w - 1, F: at.R1 + alto - 1}
	if fuente == h && at.C1 <= c2 && fin.C >= rr.C1 && at.R1 <= f2 && fin.F >= rr.R1 {
		return "overlap"
	}
	if viejo, ok := formula.ParseRangeRef(d.Range); ok {
		rango(viejo.C1, viejo.R1, viejo.C2, viejo.R2, func(k Celda) { l.borrarCelda(k) })
	}
	l.volcarDinamica(out, Celda{C: at.C1, F: at.R1}, formatoCampo, valores)
	d.Range = Ref(at.C1, at.R1) + ":" + Ref(fin.C, fin.F)
	if i := slices.IndexFunc(h.dinamicas, func(x analisis.Dinamica) bool { return x.Name == d.Name }); i >= 0 {
		h.dinamicas[i] = d
	} else {
		h.dinamicas = append(h.dinamicas, d)
	}
	return ""
}

// origenDinamica es `pivotSource`: la rejilla del origen de d (la fila 0, los
// encabezados) y el formato de cada campo (el de su primera celda de datos con uno).
// false si el origen no se lee. Lo comparten EscribirDinamica y la segmentación de la
// tabla, para que los elementos se llamen igual en las dos.
func (l *Libro) origenDinamica(d analisis.Dinamica) (rejilla [][]formula.Value, formatoCampo func(int) *FormatoNum, ok bool) {
	fuente := l.hojaOrigen(d)
	rr, ok := formula.ParseRangeRef(d.Source)
	if fuente == nil || !ok {
		return nil, nil, false
	}
	f2 := min(rr.R2, Filas-1)
	formatoCampo = func(campo int) *FormatoNum {
		for f := rr.R1 + 1; f <= f2; f++ {
			if x, ok := fuente.formatos[Celda{C: rr.C1 + campo, F: f}]; ok {
				return &x
			}
		}
		return nil
	}
	return l.valoresRango(d.Source, fuente.nombre), formatoCampo, true
}

// rotulosConFormato son los rótulos de la tabla con el texto de cada valor de campo en
// su formato, como `pivotSource`.
func rotulosConFormato(t func(string, i18n.Vars) string, formatoCampo func(int) *FormatoNum) analisis.RotulosDinamica {
	r := rotulosDinamica(t)
	r.Show = func(v formula.Value, campo int) string {
		var f *FormatoNum
		if campo >= 0 {
			f = formatoCampo(campo)
		}
		s, _ := textoConFormato(v, f)
		return s
	}
	return r
}

// hojaOrigen es la hoja de la que lee la tabla: la suya, u otra por su nombre. nil si
// esa otra ya no existe.
func (l *Libro) hojaOrigen(d analisis.Dinamica) *hojaLibro {
	if d.SourceSheet == "" {
		return l.h()
	}
	for i := range l.f.hojas {
		if l.f.hojas[i].nombre == d.SourceSheet {
			return &l.f.hojas[i]
		}
	}
	return nil
}

// volcarDinamica escribe la rejilla calculada desde la esquina: cada casilla como HTML
// (en negrita si toca) con su formato de número, y vacía lo que la tabla deja en blanco.
func (l *Libro) volcarDinamica(out [][]*analisis.CasillaDinamica, esquina Celda,
	formatoCampo func(int) *FormatoNum, valores []analisis.ValorDinamica,
) {
	h := l.h()
	for i, fila := range out {
		for j, cel := range fila {
			k := Celda{C: esquina.C + j, F: esquina.F + i}
			l.borrarCelda(k)
			crudo := crudoCasilla(cel)
			if crudo == "" {
				continue
			}
			html := escaparHTML(crudo)
			if cel.Negrita {
				html = "<b>" + html + "</b>"
			}
			h.celdas[k] = html
			var fm *FormatoNum
			switch {
			case cel.Fmt == nil:
			case cel.Fmt.Pct:
				fm = &porciento
			case cel.Fmt.Campo >= 0:
				fm = formatoCampo(cel.Fmt.Campo)
			case cel.Fmt.Valor < len(valores):
				fm = formatoValor(valores[cel.Fmt.Valor])
			}
			if fm != nil {
				h.formatos[k] = *fm
			}
		}
	}
}

// crudoCasilla es el texto crudo de una casilla: el número con FormatNumber, el lógico
// en mayúsculas o el texto. "" si va vacía.
func crudoCasilla(cel *analisis.CasillaDinamica) string {
	if cel == nil {
		return ""
	}
	switch v := cel.V.(type) {
	case float64:
		return formula.FormatNumber(v)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case string:
		return v
	}
	return ""
}

// borrarCelda vacía una celda de la hoja activa: su contenido y su formato de número.
func (l *Libro) borrarCelda(k Celda) {
	delete(l.h().celdas, k)
	delete(l.h().formatos, k)
}

// mandarDinamica ejecuta los tres comandos. false si no es uno de ellos.
func (c *Lienzo) mandarDinamica(cmd editor.Comando, valor string) bool {
	var objetivos []analisis.Dinamica
	switch cmd {
	case CmdDinamica:
		d, ok := c.dinamicaPedida(valor)
		if !ok {
			return false
		}
		objetivos = []analisis.Dinamica{d}
	case CmdDinamicaQuitarla:
		return c.Libro.QuitarDinamica(valor)
	case CmdDinamicaAct:
		for _, d := range c.Libro.Dinamicas() {
			if valor == "" || d.Name == valor {
				objetivos = append(objetivos, d)
			}
		}
		if len(objetivos) == 0 {
			return false
		}
	default:
		return false
	}
	if fallo := c.Libro.escribirDinamicas(objetivos, c.t); fallo != "" {
		c.avisarFalloDinamica(fallo)
		return false
	}
	if cmd == CmdDinamica {
		c.seleccionarDinamica(objetivos[0].Name)
	}
	return true
}

// avisarFalloDinamica dice por qué no se pudo escribir una tabla dinámica.
func (c *Lienzo) avisarFalloDinamica(fallo string) {
	det := map[string]string{"overlap": "sheet.msg.pivotOverlap", "room": "sheet.msg.pivotNoRoom"}[fallo]
	if det == "" {
		det = "sheet.msg.pivotNoSource"
	}
	c.avisar2(ui.AvisoAdvertencia, "sheet.msg.pivotFailed", det)
}

// dinamicaPedida es la definición del valor de `pivot`, saneada: con nombre (el
// siguiente libre si no trae) y con el rango que ya ocupaba, para vaciarlo.
func (c *Lienzo) dinamicaPedida(valor string) (analisis.Dinamica, bool) {
	var crudo map[string]any
	if json.Unmarshal([]byte(valor), &crudo) != nil {
		return analisis.Dinamica{}, false
	}
	if _, antiguo := crudo["rowField"]; antiguo {
		crudo = dinamicaAntigua(crudo)
	}
	existentes := c.Libro.Dinamicas()
	nombre, _ := crudo["name"].(string)
	if nombre == "" {
		usados := make([]string, 0, len(existentes))
		for _, d := range existentes {
			usados = append(usados, d.Name)
		}
		nombre = analisis.SiguienteNombre(c.t("pivot.namePrefix", nil), usados)
	}
	crudo["name"] = nombre
	delete(crudo, "range")
	for _, d := range existentes {
		if d.Name == nombre && d.Range != "" {
			crudo["range"] = d.Range
		}
	}
	return sanearDinamica(crudo)
}

// escribirDinamicas escribe esas tablas en un solo paso de deshacer. Si una falla, el
// libro vuelve a como estaba y se devuelve su error.
func (l *Libro) escribirDinamicas(ds []analisis.Dinamica, t func(string, i18n.Vars) string) string {
	l.anotar()
	l.version++
	for _, d := range ds {
		fallo := l.EscribirDinamica(d, t)
		if fallo == "" {
			continue
		}
		antes := l.pasado[len(l.pasado)-1]
		l.pasado = l.pasado[:len(l.pasado)-1]
		l.restaurar(antes)
		return fallo
	}
	l.restaurar(l.f)
	return ""
}

// QuitarDinamica es `pivotdel`: vacía lo que ocupaba la tabla y la quita. false si no
// hay ninguna con ese nombre.
func (l *Libro) QuitarDinamica(nombre string) bool {
	i := slices.IndexFunc(l.h().dinamicas, func(d analisis.Dinamica) bool { return d.Name == nombre })
	if i < 0 {
		return false
	}
	l.anotar()
	l.version++
	h := l.h()
	if r, ok := formula.ParseRangeRef(h.dinamicas[i].Range); ok {
		rango(r.C1, r.R1, r.C2, r.R2, func(k Celda) { l.borrarCelda(k) })
	}
	h.dinamicas = slices.Delete(slices.Clone(h.dinamicas), i, i+1)
	if len(h.dinamicas) == 0 {
		h.dinamicas = nil
	}
	l.restaurar(l.f)
	return true
}

// seleccionarDinamica selecciona la tabla recién escrita y lo avisa, como la web.
func (c *Lienzo) seleccionarDinamica(nombre string) {
	for _, x := range c.Libro.Dinamicas() {
		r, ok := formula.ParseRangeRef(x.Range)
		if x.Name != nombre || !ok {
			continue
		}
		c.Rejilla.sel, c.Rejilla.ext = Celda{C: r.C1, F: r.R1}, &Celda{C: r.C2, F: r.R2}
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.pivotCreated", nil),
				c.t("sheet.msg.pivotCreated.detail", i18n.Vars{"count": r.R2 - r.R1, "ref": x.Anchor}))
		}
	}
}

// dinamicaAntigua es `legacyPivot`: el `pivot` de antes, el de las macros grabadas.
func dinamicaAntigua(r map[string]any) map[string]any {
	agg := "sum"
	if a, _ := r["agg"].(string); a == "count" || a == "avg" {
		agg = a
	}
	cols := []any{}
	if c, ok := r["colField"]; ok && c != nil && c != "" {
		cols = append(cols, map[string]any{"field": c})
	}
	return map[string]any{
		"source": r["source"], "anchor": r["target"], "rows": []any{map[string]any{"field": r["rowField"]}},
		"cols": cols, "values": []any{map[string]any{"field": r["valueField"], "fn": agg}},
	}
}

// valoresRango son los VALORES calculados de un rango (de la hoja activa, o de `hoja`):
// `readSheetValues` de la web.
func (l *Libro) valoresRango(texto, hoja string) [][]formula.Value {
	r, ok := formula.ParseRangeRef(strings.TrimSpace(texto))
	if !ok {
		return nil
	}
	nombre := l.Hoja()
	if hoja != "" {
		nombre = hoja
	}
	l.sincronizarOcultas()
	var out [][]formula.Value
	for f := r.R1; f <= min(r.R2, Filas-1); f++ {
		var fila []formula.Value
		for c := r.C1; c <= min(r.C2, Columnas-1); c++ {
			fila = append(fila, l.motor.ValueAt(nombre, c, f))
		}
		out = append(out, fila)
	}
	return out
}

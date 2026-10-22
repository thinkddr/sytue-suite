// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// # Las tablas con formato: `sheets/table.ts`
//
// Una tabla (el ListObject de Excel) es un rango con nombre único en el libro, cabecera
// con los nombres de columna, bandas y, si se pide, fila de totales. Los DATOS siguen en
// `cells`: la tabla solo pone formato y comportamiento —crecer al escribir al lado, las
// referencias `Tabla[Columna]`—. La fila de totales NO son celdas: se calcula al pintar
// sobre las filas visibles, que es la semántica de SUBTOTAL.
//
// Todo lo de aquí es el puerto de las funciones puras de la web, con sus mismos casos
// en `testdata` compartidos con ella (ver `tablas_test.go`).

// TablaFmt es `SheetTable` tal y como se guarda: solo lo que no es el valor por defecto.
type TablaFmt struct {
	Name  string `json:"name"`
	Range string `json:"range"`
	Style string `json:"style,omitempty"`
	// Headers solo se guarda cuando es false (la ausencia es «con cabecera»).
	Headers *bool `json:"headers,omitempty"`
	// El contrato es el SheetData de la web, que va en camelCase.
	TotalsRow bool              `json:"totalsRow,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Totals    map[string]string `json:"totals,omitempty"`
}

// tablaCruda es una tabla tal y como llega, sin sanear.
type tablaCruda struct {
	Name      any `json:"name"`
	Range     any `json:"range"`
	Style     any `json:"style"`
	Headers   any `json:"headers"`
	TotalsRow any `json:"totalsRow"` //nolint:tagliatelle // el nombre lo fija la web
	Totals    any `json:"totals"`
}

// EstiloTabla es `TableStyle`: cabecera sólida, banda tenue y contorno fino.
type EstiloTabla struct {
	ID, Clave                 string
	Cabecera, TintaCab, Banda string
	Borde                     string
}

// EstilosTabla son los cinco de `TABLE_STYLES`, en su orden. La clave es la del
// catálogo con la que la web los nombra en el selector.
var EstilosTabla = []EstiloTabla{
	{ID: "slate", Clave: "table.style.slate", Cabecera: "#475569", TintaCab: "#FFFFFF", Banda: "#F1F5F9", Borde: "#CBD5E1"},
	{ID: "blue", Clave: "table.style.blue", Cabecera: "#2563EB", TintaCab: "#FFFFFF", Banda: "#EFF6FF", Borde: "#BFDBFE"},
	{ID: "green", Clave: "table.style.green", Cabecera: "#16A34A", TintaCab: "#FFFFFF", Banda: "#F0FDF4", Borde: "#BBF7D0"},
	{ID: "amber", Clave: "table.style.amber", Cabecera: "#B45309", TintaCab: "#FFFFFF", Banda: "#FFFBEB", Borde: "#FDE68A"},
	{ID: "plain", Clave: "table.style.plain", Cabecera: "#E2E8F0", TintaCab: "#0A0A0A", Banda: "#F8FAFC", Borde: "#CBD5E1"},
}

// estiloTablaDe es `tableStyleOf`: el de ese id, o el primero.
func estiloTablaDe(id string) EstiloTabla {
	for _, e := range EstilosTabla {
		if e.ID == id {
			return e
		}
	}
	return EstilosTabla[0]
}

// FuncionesTotal son `TABLE_TOTAL_FNS`: el valor y la clave de su rótulo.
var FuncionesTotal = []struct{ Valor, Clave string }{
	{"none", "table.total.none"},
	{"sum", "query.agg.sum"},
	{"average", "query.agg.avg"},
	{"count", "table.total.count"},
	{"counta", "table.total.counta"},
	{"min", "query.agg.min"},
	{"max", "query.agg.max"},
}

func esFuncionTotal(v string) bool {
	for _, f := range FuncionesTotal {
		if f.Valor == v {
			return true
		}
	}
	return false
}

// sanear es `sanitizeTable`: nil si no vale.
//
//nolint:gocyclo // un campo por rama, como `sanitizeTable`
func (t tablaCruda) sanear() *TablaFmt {
	nombre, ok := t.Name.(string)
	if !ok || jsTrimCalc(nombre) == "" {
		return nil
	}
	rango, ok := t.Range.(string)
	if !ok {
		return nil
	}
	if _, ok := formula.ParseRangeRef(rango); !ok {
		return nil
	}
	out := &TablaFmt{Name: jsTrimCalc(nombre), Range: rango}
	if s, ok := t.Style.(string); ok && estiloTablaDe(s).ID == s {
		out.Style = s
	}
	if b, ok := t.Headers.(bool); ok && !b {
		out.Headers = &b
	}
	if b, ok := t.TotalsRow.(bool); ok && b {
		out.TotalsRow = true
	}
	if m, ok := t.Totals.(map[string]any); ok {
		for k, v := range m {
			n, err := 0.0, error(nil)
			if jsTrimCalc(k) != "" { // `Number('')` es 0
				n, err = strconv.ParseFloat(jsTrimCalc(k), 64)
			}
			fn, esTexto := v.(string)
			if err != nil || n != math.Trunc(n) || n < 0 || !esTexto || !esFuncionTotal(fn) || fn == "none" {
				continue
			}
			if out.Totals == nil {
				out.Totals = map[string]string{}
			}
			out.Totals[k] = fn
		}
	}
	return out
}

// jsTrimCalc es `String.prototype.trim`.
func jsTrimCalc(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0xA0 || r == 0xFEFF ||
			r == 0x1680 || (r >= 0x2000 && r <= 0x200A) || r == 0x2028 || r == 0x2029 || r == 0x202F || r == 0x205F || r == 0x3000
	})
}

// ---- geometría ----

// tablaCompilada es `CompiledTable`: el rango ya leído y el cuerpo precalculado.
type tablaCompilada struct {
	rect             formula.Rect
	t                TablaFmt
	cab, tot         bool
	cuerpo1, cuerpo2 int // filas de datos; cuerpo2 < cuerpo1 si no hay ninguna
}

// compilarTabla es `compileTable`.
func compilarTabla(t TablaFmt) (tablaCompilada, bool) {
	r, ok := formula.ParseRangeRef(t.Range)
	if !ok {
		return tablaCompilada{}, false
	}
	ct := tablaCompilada{rect: r, t: t, cab: t.Headers == nil || *t.Headers, tot: t.TotalsRow}
	ct.cuerpo1, ct.cuerpo2 = r.R1, r.R2
	if ct.cab {
		ct.cuerpo1++
	}
	if ct.tot {
		ct.cuerpo2--
	}
	return ct, true
}

// compilarTablas es `compileTables`: las de rango inválido fuera, el orden se queda.
func compilarTablas(lista []TablaFmt) []tablaCompilada {
	out := make([]tablaCompilada, 0, len(lista))
	for _, t := range lista {
		if ct, ok := compilarTabla(t); ok {
			out = append(out, ct)
		}
	}
	return out
}

// Los papeles de una celda dentro de su tabla.
const (
	papelCabecera = "header"
	papelCuerpo   = "body"
	papelTotal    = "total"
)

// infoTabla es `TableCellInfo`.
type infoTabla struct {
	ct    tablaCompilada
	papel string
	banda bool // solo en el cuerpo: fila con relleno alterno
	col   int  // índice de la columna dentro de la tabla
}

// infoTablaEn es `tableCellInfo`: la ÚLTIMA tabla que cubre la celda gana.
func infoTablaEn(tablas []tablaCompilada, c, f int) (infoTabla, bool) {
	var out infoTabla
	hay := false
	for _, ct := range tablas {
		r := ct.rect
		if c < r.C1 || c > r.C2 || f < r.R1 || f > r.R2 {
			continue
		}
		papel := papelCuerpo
		switch {
		case ct.cab && f == r.R1:
			papel = papelCabecera
		case ct.tot && f == r.R2:
			papel = papelTotal
		}
		out, hay = infoTabla{ct: ct, papel: papel, banda: papel == papelCuerpo && (f-ct.cuerpo1)%2 == 1, col: c - r.C1}, true
	}
	return out, hay
}

// crecerPorEdicion es `expandRangeForEdit`: el rectángulo nuevo si escribir en (c, f)
// amplía la tabla por su borde derecho o por debajo (esto último solo sin fila de
// totales).
func crecerPorEdicion(ct tablaCompilada, c, f, maxC, maxF int) (formula.Rect, bool) {
	r := ct.rect
	if c == r.C2+1 && c <= maxC && f >= r.R1 && f <= r.R2 {
		return formula.Rect{C1: r.C1, R1: r.R1, C2: c, R2: r.R2}, true
	}
	if !ct.tot && f == r.R2+1 && f <= maxF && c >= r.C1 && c <= r.C2 {
		return formula.Rect{C1: r.C1, R1: r.R1, C2: r.C2, R2: f}, true
	}
	return formula.Rect{}, false
}

// valorTotal es `tableTotalValue`: la agregación de la fila de totales sobre los valores
// de las filas VISIBLES. "" es la celda en blanco.
func valorTotal(fn string, vals []formula.Value) formula.Value {
	if fn == "none" {
		return ""
	}
	if fn == "counta" {
		n := 0
		for _, v := range vals {
			if _, err := v.(formula.Error); !err && v != "" {
				n++
			}
		}
		return float64(n)
	}
	var nums []float64
	for _, v := range vals {
		if x, ok := v.(float64); ok && !math.IsNaN(x) && !math.IsInf(x, 0) {
			nums = append(nums, x)
		}
	}
	if fn == "count" {
		return float64(len(nums))
	}
	if len(nums) == 0 {
		if fn == "sum" {
			return 0.0
		}
		return ""
	}
	switch fn {
	case "sum", "average":
		s := 0.0
		for _, x := range nums {
			s += x
		}
		if fn == "average" {
			return s / float64(len(nums))
		}
		return s
	case "min":
		return slices.Min(nums)
	case "max":
		return slices.Max(nums)
	}
	return ""
}

// ---- nombres ----

// nombreTablaLibre es `uniqueTableName`: «Tabla1», «Tabla2»… sin distinguir mayúsculas.
func nombreTablaLibre(existentes []string) string {
	usados := map[string]bool{}
	for _, n := range existentes {
		usados[strings.ToLower(n)] = true
	}
	for i := 1; ; i++ {
		n := "Tabla" + strconv.Itoa(i)
		if !usados[strings.ToLower(n)] {
			return n
		}
	}
}

// nombresCabecera es `uniqueHeaderNames`, compartido con el servicio.
var nombresCabecera = formula.NombresCabecera

// ---- referencias estructuradas ----

// tablaResoluble es `ResolvableTable`: vive en el motor porque las descargas del servicio
// resuelven `Tabla[Columna]` igual que el lienzo.
type tablaResoluble = formula.TablaResoluble

// tablasResolubles es `collectResolvableTables`. `cabecera` da el texto crudo de una
// celda de cualquier hoja.
func tablasResolubles(hojas []hojaLibro, cabecera func(hoja, c, f int) string) []tablaResoluble {
	var out []tablaResoluble
	for i, h := range hojas {
		for _, t := range h.tablas {
			tr, ok := formula.NuevaTablaResoluble(t.Name, h.nombre, t.Range, t.Headers == nil || *t.Headers, t.TotalsRow,
				func(c, f int) string { return cabecera(i, c, f) })
			if ok {
				out = append(out, tr)
			}
		}
	}
	return out
}

// resolverEstructurada es `resolveStructuredRef`.
var resolverEstructurada = formula.ResolverEstructurada

// ---- en el libro ----

// enCabeceraDeTabla dice si (c, f) es una celda de cabecera de alguna de esas tablas.
func enCabeceraDeTabla(lista []TablaFmt, c, f int) bool {
	for _, ct := range compilarTablas(lista) {
		if ct.cab && f == ct.rect.R1 && c >= ct.rect.C1 && c <= ct.rect.C2 {
			return true
		}
	}
	return false
}

// Tablas son las tablas con formato de la hoja activa.
func (l *Libro) Tablas() []TablaFmt { return l.h().tablas }

// tablasActivas son las de la hoja activa ya compiladas.
//
// ponytail: se compilan en cada llamada (un regexp por tabla). Son pocas y el pintado
// las pide una vez por pasada, no por celda; si una hoja llevara cientos, se guardan con
// la versión como `decorado`.
func (l *Libro) tablasActivas() []tablaCompilada { return compilarTablas(l.h().tablas) }

// TablaEn es `anchorIdx`: la ÚLTIMA tabla de la hoja activa que cubre la celda, con su
// índice; -1 si ninguna.
func (l *Libro) TablaEn(c, f int) (TablaFmt, int) {
	idx := -1
	for i, t := range l.h().tablas {
		if r, ok := formula.ParseRangeRef(t.Range); ok && c >= r.C1 && c <= r.C2 && f >= r.R1 && f <= r.R2 {
			idx = i
		}
	}
	if idx < 0 {
		return TablaFmt{}, -1
	}
	return l.h().tablas[idx], idx
}

// resolverTabla es el `resolveTable` del motor: las tablas del libro, rehechas solo
// cuando algo ha cambiado.
func (l *Libro) resolverTabla(q formula.TableQuery, aqui formula.Here) (*formula.RefRange, formula.Error) {
	return resolverEstructurada(l.tablasResolubles(), q, aqui)
}

func (l *Libro) tablasResolubles() []tablaResoluble {
	if l.resolubles.lista == nil || l.resolubles.version != l.version {
		l.resolubles.version = l.version
		l.resolubles.lista = tablasResolubles(l.f.hojas, func(hoja, c, f int) string {
			return textoPlano(l.f.hojas[hoja].celdas[Celda{C: c, F: f}])
		})
		if l.resolubles.lista == nil {
			l.resolubles.lista = []tablaResoluble{}
		}
	}
	return l.resolubles.lista
}

// ResolverParaResaltar es el `resolveStructuredRect` del lienzo: resuelve una
// estructurada desde la celda ancla para enseñarla en la rejilla.
func (l *Libro) ResolverParaResaltar(ancla Celda) formula.ResolutorEstructurada {
	if len(l.tablasResolubles()) == 0 {
		return nil
	}
	return func(q formula.TableQuery) (string, formula.Rect, bool) {
		rr, err := resolverEstructurada(l.tablasResolubles(), q, formula.Here{Sheet: l.Hoja(), Col: ancla.C, Row: ancla.F})
		if err != "" || rr == nil {
			return "", formula.Rect{}, false
		}
		return rr.Sheet, formula.Rect{C1: rr.C1, R1: rr.R1, C2: rr.C2, R2: rr.R2}, true
	}
}

// ColumnasDeTabla son los nombres de columna de la tabla que nombra un corchete (la
// implícita: la que contiene la celda ancla). Es `columnsForBracket`.
func (l *Libro) ColumnasDeTabla(tabla string, ancla Celda) []string {
	for _, t := range l.tablasResolubles() {
		if tabla == "" {
			if t.Hoja == l.Hoja() && ancla.C >= t.Rect.C1 && ancla.C <= t.Rect.C2 && ancla.F >= t.Rect.R1 && ancla.F <= t.Rect.R2 {
				return t.Columnas
			}
			continue
		}
		if strings.EqualFold(t.Nombre, tabla) {
			return t.Columnas
		}
	}
	return nil
}

// NombresDeTabla son los nombres de todas las tablas del libro, en orden.
func (l *Libro) NombresDeTabla() []string {
	var out []string
	for _, h := range l.f.hojas {
		for _, t := range h.tablas {
			out = append(out, t.Name)
		}
	}
	return out
}

// cambiarTablas pone otra lista de tablas en la hoja activa y rehace el motor: las
// estructuradas dependen de los rangos.
func (l *Libro) cambiarTablas(lista []TablaFmt) {
	if len(lista) == 0 {
		lista = nil
	}
	l.h().tablas = lista
	l.rehacerMotor()
}

// crecerTablas es `grownTables`: las tablas con el rango extendido si (c, f) cae en su
// borde derecho o inferior; nil si ninguna crece.
func crecerTablas(lista []TablaFmt, c, f int) []TablaFmt {
	var out []TablaFmt
	for i, t := range lista {
		ct, ok := compilarTabla(t)
		if !ok {
			continue
		}
		r, crece := crecerPorEdicion(ct, c, f, Columnas-1, Filas-1)
		if !crece {
			continue
		}
		if out == nil {
			out = slices.Clone(lista)
		}
		out[i].Range = Ref(r.C1, r.R1) + ":" + Ref(r.C2, r.R2)
	}
	return out
}

// CrearTabla es el comando `tableadd`: una tabla sobre el rango, con nombres de columna
// únicos en la cabecera y el autofiltro puesto en ella (los botones de filtro y orden).
// Devuelve la tabla creada.
func (l *Libro) CrearTabla(rango string, cabeceras bool, estilo string) (TablaFmt, bool) {
	r, ok := formula.ParseRangeRef(rango)
	if !ok || r.C1 >= Columnas || r.R1 >= Filas {
		return TablaFmt{}, false // ancla fuera de la rejilla: no se crea una tabla fantasma
	}
	c2, f2 := min(r.C2, Columnas-1), min(r.R2, Filas-1)
	t := TablaFmt{Name: nombreTablaLibre(l.NombresDeTabla()), Range: Ref(r.C1, r.R1) + ":" + Ref(c2, f2)}
	if estilo != "" {
		t.Style = estiloTablaDe(estilo).ID
	}
	if !cabeceras {
		no := false
		t.Headers = &no
	}
	l.anotar()
	h := l.h()
	if cabeceras {
		crudos := make([]string, c2-r.C1+1)
		for i := range crudos {
			crudos[i] = textoPlano(h.celdas[Celda{C: r.C1 + i, F: r.R1}])
		}
		for i, n := range nombresCabecera(crudos) {
			k := Celda{C: r.C1 + i, F: r.R1}
			p := leerCelda(h.celdas[k]) // conserva negrita, cursiva y alineación
			p.dentro = escaparHTML(n)
			if html := p.html(); html != h.celdas[k] {
				l.poner(l.f.activa, k.C, k.F, html)
			}
		}
		h.filtro = &Autofiltro{Row: r.R1, C1: r.C1, C2: c2}
	}
	l.cambiarTablas(append(slices.Clone(h.tablas), t))
	return t, true
}

// QuitarTabla es `tabledel`: fuera la tabla i, y con ella el autofiltro si cubría justo
// su cabecera.
func (l *Libro) QuitarTabla(i int) {
	h := l.h()
	if i < 0 || i >= len(h.tablas) {
		return
	}
	l.anotar()
	if r, ok := formula.ParseRangeRef(h.tablas[i].Range); ok && h.filtro != nil &&
		h.filtro.Row == r.R1 && h.filtro.C1 == r.C1 && h.filtro.C2 == r.C2 {
		h.filtro = nil
	}
	l.cambiarTablas(slices.Delete(slices.Clone(h.tablas), i, i+1))
}

// EstiloDeTabla es `tablestyle`.
func (l *Libro) EstiloDeTabla(i int, estilo string) {
	h := l.h()
	if i < 0 || i >= len(h.tablas) {
		return
	}
	l.anotar()
	lista := slices.Clone(h.tablas)
	lista[i].Style = estiloTablaDe(estilo).ID
	l.cambiarTablas(lista)
}

// AlternarTotales es `tabletotals`: pone la fila de totales debajo (con la suma en la
// última columna, como Excel) o la quita. false si no cabe: la tabla toca el fondo.
func (l *Libro) AlternarTotales(i int) bool {
	h := l.h()
	if i < 0 || i >= len(h.tablas) {
		return true
	}
	t := h.tablas[i]
	r, ok := formula.ParseRangeRef(t.Range)
	if !ok {
		return true
	}
	poner := !t.TotalsRow
	if poner && r.R2+1 >= Filas {
		return false
	}
	nueva := t
	if poner {
		nueva.TotalsRow = true
		nueva.Range = Ref(r.C1, r.R1) + ":" + Ref(r.C2, r.R2+1)
		nueva.Totals = map[string]string{}
		for k, v := range t.Totals {
			nueva.Totals[k] = v
		}
		nueva.Totals[strconv.Itoa(r.C2-r.C1)] = "sum"
	} else {
		nueva.TotalsRow, nueva.Totals = false, nil
		nueva.Range = Ref(r.C1, r.R1) + ":" + Ref(r.C2, r.R2-1)
	}
	l.anotar()
	lista := slices.Clone(h.tablas)
	lista[i] = nueva
	l.cambiarTablas(lista)
	return true
}

// FuncionTotal es `tabletotalfn`: la función de la fila de totales de una columna.
func (l *Libro) FuncionTotal(nombre string, col int, fn string) bool {
	h := l.h()
	i := slices.IndexFunc(h.tablas, func(t TablaFmt) bool { return t.Name == nombre })
	if i < 0 || col < 0 || !esFuncionTotal(fn) {
		return false
	}
	t := h.tablas[i]
	totales := map[string]string{}
	for k, v := range t.Totals {
		totales[k] = v
	}
	if fn == "none" {
		delete(totales, strconv.Itoa(col))
	} else {
		totales[strconv.Itoa(col)] = fn
	}
	t.Totals = totales
	if len(totales) == 0 {
		t.Totals = nil
	}
	l.anotar()
	lista := slices.Clone(h.tablas)
	lista[i] = t
	l.cambiarTablas(lista)
	return true
}

// totalDeTabla es `tableTotalHtml` sin el HTML: el texto de una celda de la fila de
// totales, que va en semi-negrita. La primera columna sin función dice «Total».
func (l *Libro) totalDeTabla(info infoTabla, ocultas []bool, total string) string {
	fn := info.ct.t.Totals[strconv.Itoa(info.col)]
	if fn == "" || fn == "none" {
		if info.col == 0 {
			return total
		}
		return ""
	}
	col := info.ct.rect.C1 + info.col
	var vals []formula.Value
	for f := info.ct.cuerpo1; f <= info.ct.cuerpo2; f++ {
		if f < len(ocultas) && ocultas[f] {
			continue
		}
		vals = append(vals, l.Valor(col, f))
	}
	h := l.h()
	fm, ok := h.formatos[Celda{C: col, F: info.ct.rect.R2}]
	if !ok {
		fm, ok = h.formatos[Celda{C: col, F: info.ct.cuerpo2}]
	}
	var pf *FormatoNum
	if ok {
		pf = &fm
	}
	texto, _ := textoConFormato(valorTotal(fn, vals), pf)
	return texto
}

// pintarMarcosTabla es `tableOverlays`: el contorno de 1 px de cada tabla sobre la banda
// de filas que se ven, y el filo de 1,5 px bajo su cabecera, en el color del estilo.
// Como en la web, van en las coordenadas del CONTENIDO (se desplazan con él aunque haya
// paneles inmovilizados) y por encima de las celdas, bajo las cabeceras.
func (rr *rejillaRenderer) pintarMarcosTabla(vw vista, _ fyne.Size) {
	n := 0
	tomar := func(k int) []*canvas.Rectangle {
		for len(rr.marcosTabla) < n+k {
			rr.marcosTabla = append(rr.marcosTabla, canvas.NewRectangle(color.NRGBA{}))
			rr.recomponer()
		}
		out := rr.marcosTabla[n : n+k]
		n += k
		return out
	}
	x := func(c int) float32 { return vw.ox() + float32(c)*vw.ancho() - vw.desX }
	y := func(puesto int) float32 { return vw.oy() + float32(puesto)*vw.alto() - vw.desY }
	for _, ct := range rr.tablas {
		arriba, abajo := -1, -1
		for f := ct.rect.R1; f <= ct.rect.R2; f++ {
			if !vw.filas.oculta(f) {
				if arriba < 0 {
					arriba = vw.filas.puesto(f)
				}
				abajo = vw.filas.puesto(f)
			}
		}
		if abajo < 0 || ct.rect.C1 >= Columnas {
			continue
		}
		col, _ := tokens.ParseColor(estiloTablaDe(ct.t.Style).Borde)
		izq, der := x(ct.rect.C1), x(min(ct.rect.C2, Columnas-1)+1)
		var cuatro [4]*canvas.Rectangle
		copy(cuatro[:], tomar(4))
		marco(cuatro, col, fyne.NewPos(izq, y(arriba)), fyne.NewSize(der-izq, y(abajo+1)-y(arriba)), tema.Borde*vw.z, vw.ox(), vw.oy(), true)
		if ct.cab && !vw.filas.oculta(ct.rect.R1) {
			filo := tomar(1)[0]
			fy := y(vw.filas.puesto(ct.rect.R1) + 1)
			if fy >= vw.oy() {
				caja(filo, col, max(izq, vw.ox()), fy, der-max(izq, vw.ox()), GrosorFiloCabecera*vw.z)
			} else {
				filo.Hide()
			}
		}
	}
	for ; n < len(rr.marcosTabla); n++ {
		rr.marcosTabla[n].Hide()
	}
}

// GrosorFiloCabecera es el `borderTop: 1.5px` bajo la cabecera de una tabla.
const GrosorFiloCabecera float32 = 1.5

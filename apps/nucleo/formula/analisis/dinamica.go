// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// La TABLA DINÁMICA: el puerto de `sheets/pivot.ts` de la web, con sus mismos vectores
// (testdata/dinamica.json). Recibe el origen —los VALORES ya calculados, con la fila de
// encabezados— y la definición, y devuelve la rejilla que se escribe en la hoja, celda
// a celda. El diseño, el orden de los elementos y los subtotales están explicados en
// el fichero de la web; aquí se sigue paso a paso.

// DefDinamica es `PivotDef`. Los nombres JSON son los del documento de la web.
type DefDinamica struct {
	Rows    []CampoDinamica `json:"rows"`
	Cols    []CampoDinamica `json:"cols"`
	Values  []ValorDinamica `json:"values"`
	Filters []CampoDinamica `json:"filters,omitempty"`
	// HiddenFilters filtran sin estar en la tabla: los de una segmentación de un campo
	// que no va en filtros, filas ni columnas. Solo cuentan sus Hidden, y el de un campo
	// que está en la tabla duerme.
	HiddenFilters []CampoDinamica `json:"hiddenFilters,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	GrandRows     *bool           `json:"grandRows,omitempty"`     //nolint:tagliatelle // el nombre lo fija la web
	GrandCols     *bool           `json:"grandCols,omitempty"`     //nolint:tagliatelle // el nombre lo fija la web
	Subtotals     *bool           `json:"subtotals,omitempty"`
}

// CampoDinamica es `PivotFieldRef`.
type CampoDinamica struct {
	Field  int        `json:"field"`
	Date   string     `json:"date,omitempty"`
	Bin    *Intervalo `json:"bin,omitempty"`
	Order  string     `json:"order,omitempty"`
	Hidden []string   `json:"hidden,omitempty"`
}

// Intervalo es la agrupación de números: desde `Start`, hasta `End`, de `Step` en `Step`.
type Intervalo struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Step  float64 `json:"step"`
}

// ValorDinamica es `PivotValueRef`. El formato de número se guarda tal cual: lo aplica
// quien escribe la tabla.
type ValorDinamica struct {
	Field int    `json:"field"`
	Fn    string `json:"fn"`
	Show  string `json:"show,omitempty"`
	Name  string `json:"name,omitempty"`
	Fmt   any    `json:"fmt,omitempty"`
}

// RotulosDinamica es `PivotLabels`.
type RotulosDinamica struct {
	Blank, Grand, All, Multiple string
	TotalOf                     func(item string) string
	ValueName                   func(fn, campo string) string
	Quarter, Month              func(n int) string
	Day                         func(serie float64) string
	Show                        func(v formula.Value, campo int) string
}

// FormatoCasilla es `PivotCell.fmt`: el de una columna del origen, el de un campo de
// valor o el de porcentaje. -1 en los dos índices si no lleva.
type FormatoCasilla struct {
	Campo, Valor int
	Pct          bool
}

// CasillaDinamica es `PivotCell`: su valor (float64, string o bool), la negrita y el
// formato.
type CasillaDinamica struct {
	V       any
	Negrita bool
	Fmt     *FormatoCasilla
}

// ---- elementos ----

type elemento struct {
	clave  string
	rango  int
	num    float64
	texto  string
	rotulo any
	crudo  bool
}

var (
	ordenEsMu sync.Mutex
	ordenEs   = collate.New(language.Spanish, collate.Numeric)
	epoca     = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
)

// compararTexto es `Intl.Collator('es', {numeric: true}).compare`.
func compararTexto(a, b string) int {
	ordenEsMu.Lock()
	defer ordenEsMu.Unlock()
	return ordenEs.CompareString(a, b)
}

func esVacio(v formula.Value) bool {
	s, ok := v.(string)
	return ok && s == ""
}

func elementoDe(v formula.Value, ref CampoDinamica, r RotulosDinamica) elemento {
	if n, ok := v.(float64); ok && !math.IsNaN(n) && !math.IsInf(n, 0) {
		if ref.Date != "" {
			t := epoca.AddDate(0, 0, int(math.Floor(n)))
			y, m := t.Year(), int(t.Month())
			switch ref.Date {
			case "years":
				return elemento{clave: "y" + strconv.Itoa(y), num: float64(y), rotulo: strconv.Itoa(y)}
			case "quarters":
				q := (m + 2) / 3
				return elemento{clave: "q" + strconv.Itoa(q), num: float64(q), rotulo: r.Quarter(q)}
			case "months":
				return elemento{clave: "m" + strconv.Itoa(m), num: float64(m), rotulo: r.Month(m)}
			default:
				d := math.Floor(n)
				return elemento{clave: "d" + formula.FormatNumber(d), num: d, rotulo: r.Day(d)}
			}
		}
		if b := ref.Bin; b != nil && b.Step > 0 {
			// Como en Excel y Calc, el final ENTRA: lo que pasa de él va a «>final».
			switch {
			case n < b.Start:
				return elemento{clave: "<", num: math.Inf(-1), rotulo: "<" + formula.FormatNumber(b.Start)}
			case n > b.End:
				return elemento{clave: ">", num: math.Inf(1), rotulo: ">" + formula.FormatNumber(b.End)}
			}
			i := math.Floor((n - b.Start) / b.Step)
			lo := b.Start + i*b.Step
			hi := lo + b.Step
			if b.Start == math.Trunc(b.Start) && b.Step == math.Trunc(b.Step) {
				hi--
			}
			hi = min(hi, b.End)
			return elemento{clave: "b" + formula.FormatNumber(i), num: lo, rotulo: formula.FormatNumber(lo) + "-" + formula.FormatNumber(hi)}
		}
		return elemento{clave: formula.FormatNumber(n), num: n, rotulo: n, crudo: true}
	}
	switch x := v.(type) {
	case formula.Error:
		return elemento{clave: string(x), rango: 3, texto: string(x), rotulo: string(x)}
	case bool:
		e := elemento{clave: "FALSE", rango: 2, rotulo: x}
		if x {
			e.clave, e.num = "TRUE", 1
		}
		return e
	case string:
		if x != "" {
			return elemento{clave: x, rango: 1, texto: x, rotulo: x}
		}
	}
	return elemento{clave: "", rango: 4, rotulo: r.Blank}
}

// compararElementos es `compareItems`.
func compararElementos(a, b elemento, desc bool) int {
	switch va, vb := a.rango == 4, b.rango == 4; {
	case va && vb:
		return 0
	case va:
		return 1
	case vb:
		return -1
	}
	c := a.rango - b.rango
	if c == 0 {
		switch {
		case a.num < b.num:
			c = -1
		case a.num > b.num:
			c = 1
		default:
			c = compararTexto(a.texto, b.texto)
		}
	}
	if desc {
		return -c
	}
	return c
}

// ---- árbol ----

type nodoD struct {
	el     *elemento
	camino string
	prof   int
	hijos  map[string]*nodoD
	// orden son los hijos en el orden en que aparecen, y luego ordenados: con el mapa de
	// Go solo, dos elementos que el colador diera por iguales saldrían al azar.
	orden []*nodoD
}

const sep = "\x01"

func nuevoNodo(el *elemento, padre *nodoD) *nodoD {
	n := &nodoD{el: el, hijos: map[string]*nodoD{}}
	if padre != nil {
		n.prof = padre.prof + 1
		n.camino = el.clave
		if padre.prof > 0 {
			n.camino = padre.camino + sep + el.clave
		}
	}
	return n
}

func insertar(raiz *nodoD, els []elemento) {
	n := raiz
	for i := range els {
		h, ok := n.hijos[els[i].clave]
		if !ok {
			h = nuevoNodo(&els[i], n)
			n.hijos[els[i].clave] = h
			n.orden = append(n.orden, h)
		}
		n = h
	}
}

func ordenarArbol(n *nodoD, refs []CampoDinamica) {
	desc := n.prof < len(refs) && refs[n.prof].Order == "desc"
	slices.SortStableFunc(n.orden, func(a, b *nodoD) int { return compararElementos(*a.el, *b.el, desc) })
	for _, h := range n.orden {
		ordenarArbol(h, refs)
	}
}

type hueco struct {
	tipo  string // "leaf", "sub" o "grand"
	nodo  *nodoD
	linea []*nodoD
}

func huecosDe(raiz *nodoD, niveles int, subtotales, total bool) []hueco {
	if niveles == 0 {
		return []hueco{{tipo: "grand", nodo: raiz}}
	}
	var out []hueco
	var andar func(n *nodoD, linea []*nodoD)
	andar = func(n *nodoD, linea []*nodoD) {
		for _, h := range n.orden {
			l := append(slices.Clone(linea), h)
			if h.prof == niveles {
				out = append(out, hueco{tipo: "leaf", nodo: h, linea: l})
				continue
			}
			andar(h, l)
			if subtotales {
				out = append(out, hueco{tipo: "sub", nodo: h, linea: l})
			}
		}
	}
	andar(raiz, nil)
	if total {
		out = append(out, hueco{tipo: "grand", nodo: raiz})
	}
	return out
}

// primeroEn es `firstAt`.
func primeroEn(previos []hueco, i int, actual hueco, d int) bool {
	if i == 0 {
		return true
	}
	p := previos[i-1]
	return len(p.linea) <= d || p.linea[d] != actual.linea[d]
}

// ---- resumen ----

type acumulado struct {
	registros, llenos int
	nums              []float64
	err               string
}

func resumir(a *acumulado, fn string) any {
	if a == nil || a.registros == 0 {
		return nil
	}
	if a.err != "" {
		return a.err
	}
	n := a.nums
	suma := func() float64 {
		s := 0.0
		for _, x := range n {
			s += x
		}
		return s
	}
	switch fn {
	case "count":
		return float64(a.llenos)
	case "countNums":
		return float64(len(n))
	case "avg":
		if len(n) == 0 {
			return "#DIV/0!"
		}
		return suma() / float64(len(n))
	case "max", "min":
		if len(n) == 0 {
			return 0.0
		}
		m := n[0]
		for _, x := range n {
			if (fn == "max" && x > m) || (fn == "min" && x < m) {
				m = x
			}
		}
		return m
	case "product":
		if len(n) == 0 {
			return 0.0
		}
		p := 1.0
		for _, x := range n {
			p *= x
		}
		return p
	case "stdev", "stdevp", "var", "varp":
		return dispersion(n, fn)
	}
	return suma()
}

// dispersion es la varianza o la desviación, muestral (stdev, var) o poblacional.
func dispersion(n []float64, fn string) any {
	muestral := fn == "stdev" || fn == "var"
	minimo := 1
	if muestral {
		minimo = 2
	}
	if len(n) < minimo {
		return "#DIV/0!"
	}
	s := 0.0
	for _, x := range n {
		s += x
	}
	media := s / float64(len(n))
	q := 0.0
	for _, x := range n {
		q += (x - media) * (x - media)
	}
	div := float64(len(n))
	if muestral {
		div--
	}
	v := q / div
	if fn == "stdev" || fn == "stdevp" {
		return math.Sqrt(v)
	}
	return v
}

// ---- la tabla ----

// CalcularDinamica es `computePivotTable`.
//
//nolint:gocyclo,cyclop,gocognit // es computePivotTable paso a paso: partirla distinto que allí es perder el hilo al compararlas
func CalcularDinamica(rejilla [][]formula.Value, def DefDinamica, r RotulosDinamica) [][]*CasillaDinamica {
	var cabecera []formula.Value
	if len(rejilla) > 0 {
		cabecera = rejilla[0]
	}
	ancho := len(cabecera)
	validos := func(cs []CampoDinamica) []CampoDinamica {
		var out []CampoDinamica
		for _, c := range cs {
			if c.Field >= 0 && c.Field < ancho {
				out = append(out, c)
			}
		}
		return out
	}
	filas, cols, filtros := validos(def.Rows), validos(def.Cols), validos(def.Filters)
	var valores []ValorDinamica
	for _, v := range def.Values {
		if v.Field >= 0 && v.Field < ancho {
			valores = append(valores, v)
		}
	}
	subtotales := def.Subtotals == nil || *def.Subtotals
	nombre := func(f int) string { return r.Show(cabecera[f], -1) }
	celda := func(fila []formula.Value, f int) formula.Value {
		if f < len(fila) {
			return fila[f]
		}
		return ""
	}

	var cuerpo [][]formula.Value
	if len(rejilla) > 1 {
		for _, fila := range rejilla[1:] {
			if slices.ContainsFunc(fila, func(v formula.Value) bool { return !esVacio(v) }) {
				cuerpo = append(cuerpo, fila)
			}
		}
	}
	var ocultan []CampoDinamica
	enTabla := func(c CampoDinamica) bool {
		return slices.ContainsFunc(slices.Concat(filtros, filas, cols), func(x CampoDinamica) bool { return x.Field == c.Field })
	}
	// El filtro oculto de un campo que está en la tabla duerme: manda el de la tabla.
	sueltos := slices.DeleteFunc(validos(def.HiddenFilters), enTabla)
	for _, c := range slices.Concat(filtros, filas, cols, sueltos) {
		if len(c.Hidden) > 0 {
			ocultan = append(ocultan, c)
		}
	}
	var guardados [][]formula.Value
	for _, fila := range cuerpo {
		if !slices.ContainsFunc(ocultan, func(c CampoDinamica) bool {
			return slices.Contains(c.Hidden, elementoDe(celda(fila, c.Field), c, r).clave)
		}) {
			guardados = append(guardados, fila)
		}
	}

	raizF, raizC := nuevoNodo(nil, nil), nuevoNodo(nil, nil)
	acs := map[string][]*acumulado{}
	acc := func(k string) []*acumulado {
		a, ok := acs[k]
		if !ok {
			a = make([]*acumulado, len(valores))
			for i := range a {
				a[i] = &acumulado{}
			}
			acs[k] = a
		}
		return a
	}
	elementos := func(fila []formula.Value, refs []CampoDinamica) []elemento {
		out := make([]elemento, len(refs))
		for i, ref := range refs {
			out[i] = elementoDe(celda(fila, ref.Field), ref, r)
		}
		return out
	}
	unir := func(els []elemento) string {
		cs := make([]string, len(els))
		for i, e := range els {
			cs[i] = e.clave
		}
		return strings.Join(cs, sep)
	}
	for _, fila := range guardados {
		fi, ci := elementos(fila, filas), elementos(fila, cols)
		insertar(raizF, fi)
		insertar(raizC, ci)
		for a := 0; a <= len(fi); a++ {
			rk := strconv.Itoa(a) + sep + unir(fi[:a])
			for b := 0; b <= len(ci); b++ {
				lista := acc(rk + sep + sep + strconv.Itoa(b) + sep + unir(ci[:b]))
				for i, vr := range valores {
					x, t := celda(fila, vr.Field), lista[i]
					t.registros++
					if !esVacio(x) {
						t.llenos++
					}
					switch y := x.(type) {
					case float64:
						if !math.IsNaN(y) && !math.IsInf(y, 0) {
							t.nums = append(t.nums, y)
						}
					case formula.Error:
						if t.err == "" {
							t.err = string(y)
						}
					}
				}
			}
		}
	}
	ordenarArbol(raizF, filas)
	ordenarArbol(raizC, cols)

	huecosF := huecosDe(raizF, len(filas), subtotales && len(filas) > 1, def.GrandRows == nil || *def.GrandRows)
	huecosC := huecosDe(raizC, len(cols), subtotales && len(cols) > 1, def.GrandCols == nil || *def.GrandCols)
	claveDe := func(n *nodoD) string { return strconv.Itoa(n.prof) + sep + n.camino }
	crudo := func(rs, cs *nodoD, i int) any {
		a := acs[claveDe(rs)+sep+sep+claveDe(cs)]
		if a == nil {
			return nil
		}
		return resumir(a[i], valores[i].Fn)
	}
	padreDe := func(h hueco) *nodoD {
		if h.nodo.prof <= 1 {
			return raizF
		}
		return h.linea[h.nodo.prof-2]
	}
	mostrado := func(rs, cs hueco, i int) *CasillaDinamica {
		v := crudo(rs.nodo, cs.nodo, i)
		if v == nil {
			return nil
		}
		// Sin campos en columnas, la única columna de valores no es un total.
		negrita := rs.tipo != "leaf" || (cs.tipo != "leaf" && len(cols) > 0)
		mos := valores[i].Show
		if s, esTexto := v.(string); mos == "" || mos == "normal" || esTexto {
			if esTexto {
				return &CasillaDinamica{V: s, Negrita: negrita, Fmt: &FormatoCasilla{Campo: -1, Valor: i}}
			}
			return &CasillaDinamica{V: v, Negrita: negrita, Fmt: &FormatoCasilla{Campo: -1, Valor: i}}
		}
		var base any
		switch mos {
		case "pctGrand":
			base = crudo(raizF, raizC, i)
		case "pctCol":
			base = crudo(raizF, cs.nodo, i)
		case "pctRow":
			base = crudo(rs.nodo, raizC, i)
		default:
			base = crudo(padreDe(rs), cs.nodo, i)
		}
		pct := &FormatoCasilla{Campo: -1, Valor: -1, Pct: true}
		b, ok := base.(float64)
		if !ok || b == 0 {
			return &CasillaDinamica{V: "#DIV/0!", Negrita: negrita, Fmt: pct}
		}
		x, _ := v.(float64) // los textos (errores) ya salieron arriba
		return &CasillaDinamica{V: x / b, Negrita: negrita, Fmt: pct}
	}

	var out [][]*CasillaDinamica
	l := max(len(filas), 1)
	nv := len(valores)
	colsValor := 0
	if nv > 0 {
		colsValor = len(huecosC) * nv
	}
	w := l + colsValor
	if len(filtros) > 0 {
		w = max(w, 2)
	}
	vacia := func() []*CasillaDinamica { return make([]*CasillaDinamica, w) }
	texto := func(e *elemento, campo int) string {
		if s, ok := e.rotulo.(string); ok {
			return s
		}
		if !e.crudo {
			campo = -1
		}
		return r.Show(e.rotulo, campo)
	}
	rotuloDe := func(e *elemento, campo int, negrita bool) *CasillaDinamica {
		c := &CasillaDinamica{V: e.rotulo, Negrita: negrita}
		if e.crudo {
			c.Fmt = &FormatoCasilla{Campo: campo, Valor: -1}
		}
		return c
	}
	nombreValor := func(i int) string {
		if valores[i].Name != "" {
			return valores[i].Name
		}
		return r.ValueName(valores[i].Fn, nombre(valores[i].Field))
	}
	fuerte := func(s string) *CasillaDinamica { return &CasillaDinamica{V: s, Negrita: true} }

	// Filtros del informe.
	if len(filtros) > 0 {
		for _, ref := range filtros {
			linea := vacia()
			linea[0] = &CasillaDinamica{V: nombre(ref.Field)}
			vistos := map[string]bool{}
			var visibles []elemento
			for _, fila := range cuerpo {
				e := elementoDe(celda(fila, ref.Field), ref, r)
				if !vistos[e.clave] {
					vistos[e.clave] = true
					if !slices.Contains(ref.Hidden, e.clave) {
						visibles = append(visibles, e)
					}
				}
			}
			switch {
			case len(ref.Hidden) == 0:
				linea[1] = &CasillaDinamica{V: r.All}
			case len(visibles) == 1:
				linea[1] = rotuloDe(&visibles[0], ref.Field, false)
			default:
				linea[1] = &CasillaDinamica{V: r.Multiple}
			}
			out = append(out, linea)
		}
		out = append(out, vacia())
	}

	// Encabezado.
	nombresFila := func(linea []*CasillaDinamica) {
		for i, ref := range filas {
			linea[i] = fuerte(nombre(ref.Field))
		}
	}
	if len(cols) == 0 {
		linea := vacia()
		nombresFila(linea)
		for i := range nv {
			linea[l+i] = fuerte(nombreValor(i))
		}
		out = append(out, linea)
	} else {
		for d := range cols {
			linea := vacia()
			if d == 0 && nv == 1 {
				linea[0] = fuerte(nombreValor(0))
			}
			for s, cs := range huecosC {
				x := l + s*nv
				switch {
				case nv == 0:
				case cs.tipo == "grand":
					if d == 0 {
						linea[x] = fuerte(r.Grand)
					}
				case cs.tipo == "sub" && cs.nodo.prof == d+1:
					linea[x] = fuerte(r.TotalOf(texto(cs.nodo.el, cols[d].Field)))
				case d < len(cs.linea) && primeroEn(huecosC, s, cs, d):
					linea[x] = rotuloDe(cs.linea[d].el, cols[d].Field, true)
				}
			}
			out = append(out, linea)
		}
		linea := vacia()
		nombresFila(linea)
		if nv > 1 {
			for s := range huecosC {
				for i := range nv {
					linea[l+s*nv+i] = fuerte(nombreValor(i))
				}
			}
		}
		out = append(out, linea)
	}

	// Cuerpo.
	for k, rs := range huecosF {
		linea := vacia()
		switch rs.tipo {
		case "grand":
			linea[0] = fuerte(r.Grand)
		case "sub":
			d := rs.nodo.prof - 1
			linea[d] = fuerte(r.TotalOf(texto(rs.nodo.el, filas[d].Field)))
		default:
			for d, n := range rs.linea {
				if primeroEn(huecosF, k, rs, d) {
					linea[d] = rotuloDe(n.el, filas[d].Field, false)
				}
			}
		}
		for s, cs := range huecosC {
			for i := range nv {
				linea[l+s*nv+i] = mostrado(rs, cs, i)
			}
		}
		out = append(out, linea)
	}
	return out
}

// ---- lo que se guarda en la hoja ----

// Dinamica es `SheetPivot`: una tabla dinámica de la hoja, con de dónde lee y dónde
// escribe. Las listas van siempre, aunque estén vacías: la web las escribe así.
type Dinamica struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceSheet string `json:"sourceSheet,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	Anchor      string `json:"anchor"`
	Range       string `json:"range,omitempty"`
	DefDinamica
}

// Las opciones que se entienden, como PIVOT_FNS, PIVOT_SHOWS y PIVOT_DATES.
var (
	FuncionesDinamica = []string{"sum", "count", "avg", "max", "min", "product", "countNums", "stdev", "stdevp", "var", "varp"}
	MostrarDinamica   = []string{"normal", "pctGrand", "pctCol", "pctRow", "pctParent"}
	FechasDinamica    = []string{"years", "quarters", "months", "days"}
)

func entrada(m map[string]any, k string) (int, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	return entero(v)
}

func sanearCampo(crudo any) (CampoDinamica, bool) {
	m, ok := crudo.(map[string]any)
	if !ok {
		return CampoDinamica{}, false
	}
	f, ok := entrada(m, "field")
	if !ok || f < 0 {
		return CampoDinamica{}, false
	}
	c := CampoDinamica{Field: f}
	if d, _ := m["date"].(string); slices.Contains(FechasDinamica, d) {
		c.Date = d
	}
	c.Bin = sanearIntervalo(m["bin"])
	if m["order"] == "desc" {
		c.Order = "desc"
	}
	if lista, ok := m["hidden"].([]any); ok {
		for _, x := range lista {
			if s, ok := x.(string); ok {
				c.Hidden = append(c.Hidden, s)
			}
		}
	}
	return c, true
}

// sanearIntervalo es el `bin` de un campo: tres números finitos, paso positivo y final
// no menor que el inicio. nil si no vale.
func sanearIntervalo(crudo any) *Intervalo {
	b, ok := crudo.(map[string]any)
	if !ok {
		return nil
	}
	var x [3]float64
	for i, k := range []string{"start", "end", "step"} {
		v, hay := b[k]
		n, ok := numeroJS(v)
		if !hay || !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil
		}
		x[i] = n
	}
	if x[2] <= 0 || x[1] < x[0] {
		return nil
	}
	return &Intervalo{Start: x[0], End: x[1], Step: x[2]}
}

func sanearValor(crudo any, sanearFmt func(any) (any, bool)) (ValorDinamica, bool) {
	m, ok := crudo.(map[string]any)
	if !ok {
		return ValorDinamica{}, false
	}
	f, ok := entrada(m, "field")
	if !ok || f < 0 {
		return ValorDinamica{}, false
	}
	v := ValorDinamica{Field: f, Fn: "sum"}
	if fn, _ := m["fn"].(string); slices.Contains(FuncionesDinamica, fn) {
		v.Fn = fn
	}
	if s, _ := m["show"].(string); s != "normal" && slices.Contains(MostrarDinamica, s) {
		v.Show = s
	}
	if n, _ := m["name"].(string); strings.TrimSpace(n) != "" {
		v.Name = cortar(strings.TrimSpace(n), 120)
	}
	if sanearFmt != nil {
		if x, ok := sanearFmt(m["fmt"]); ok {
			v.Fmt = x
		}
	}
	return v, true
}

// cortar es `slice(0, n)` de JavaScript: n unidades UTF-16, sin partir un par.
func cortar(s string, n int) string {
	u := 0
	for i, r := range s {
		ancho := 1
		if r > 0xFFFF {
			ancho = 2
		}
		if u+ancho > n {
			return s[:i]
		}
		u += ancho
	}
	return s
}

// sanearCampos son los campos de la lista que se entienden; con `ocultan`, solo los
// que ocultan algo (los filtros ocultos).
func sanearCampos(lista []any, ocultan bool) []CampoDinamica {
	var out []CampoDinamica
	for _, x := range lista {
		if c, ok := sanearCampo(x); ok && (!ocultan || len(c.Hidden) > 0) {
			out = append(out, c)
		}
	}
	return out
}

// SanearDinamica es `sanitizePivot`: la tabla leída del documento, o false si no vale.
// El formato de número de los valores lo sanea quien sabe de formatos (`sanearFmt`).
func SanearDinamica(crudo any, sanearFmt func(any) (any, bool), rango func(string) bool) (Dinamica, bool) {
	m, ok := crudo.(map[string]any)
	if !ok {
		return Dinamica{}, false
	}
	texto := func(k string) string {
		s, _ := m[k].(string)
		return strings.TrimSpace(s)
	}
	d := Dinamica{Name: texto("name"), Source: strings.ToUpper(texto("source")), Anchor: strings.ToUpper(texto("anchor"))}
	if d.Name == "" || !rango(d.Source) || !rango(d.Anchor) {
		return Dinamica{}, false
	}
	d.Name = cortar(d.Name, 120)
	d.Values = []ValorDinamica{}
	lista := func(k string) []any {
		l, _ := m[k].([]any)
		return l
	}
	d.Rows = append([]CampoDinamica{}, sanearCampos(lista("rows"), false)...)
	d.Cols = append([]CampoDinamica{}, sanearCampos(lista("cols"), false)...)
	d.Filters = sanearCampos(lista("filters"), false)
	d.HiddenFilters = sanearCampos(lista("hiddenFilters"), true) // sin nada oculto no filtra
	for _, x := range lista("values") {
		if v, ok := sanearValor(x, sanearFmt); ok {
			d.Values = append(d.Values, v)
		}
	}
	if s, _ := m["sourceSheet"].(string); s != "" {
		d.SourceSheet = s
	}
	no := false
	if m["grandRows"] == false {
		d.GrandRows = &no
	}
	if m["grandCols"] == false {
		d.GrandCols = &no
	}
	if m["subtotals"] == false {
		d.Subtotals = &no
	}
	if r, _ := m["range"].(string); rango(r) {
		d.Range = strings.ToUpper(r)
	}
	return d, true
}

// SiguienteNombre es `nextPivotName`.
func SiguienteNombre(prefijo string, usados []string) string {
	for i := 1; ; i++ {
		n := prefijo + strconv.Itoa(i)
		if !slices.Contains(usados, n) {
			return n
		}
	}
}

// ElementosDinamica es `pivotItems`: los elementos distintos de un campo del origen, en
// el orden de la tabla, con su clave y su rótulo.
func ElementosDinamica(rejilla [][]formula.Value, ref CampoDinamica, r RotulosDinamica) (claves, rotulos []string) {
	vistos := map[string]bool{}
	var els []*nodoD
	if len(rejilla) > 1 {
		for _, fila := range rejilla[1:] {
			if !slices.ContainsFunc(fila, func(v formula.Value) bool { return !esVacio(v) }) {
				continue
			}
			var v formula.Value = ""
			if ref.Field < len(fila) {
				v = fila[ref.Field]
			}
			e := elementoDe(v, ref, r)
			if !vistos[e.clave] {
				vistos[e.clave] = true
				els = append(els, &nodoD{el: &e})
			}
		}
	}
	slices.SortStableFunc(els, func(a, b *nodoD) int { return compararElementos(*a.el, *b.el, ref.Order == "desc") })
	for _, n := range els {
		claves = append(claves, n.el.clave)
		if s, ok := n.el.rotulo.(string); ok {
			rotulos = append(rotulos, s)
			continue
		}
		campo := -1
		if n.el.crudo {
			campo = ref.Field
		}
		rotulos = append(rotulos, r.Show(n.el.rotulo, campo))
	}
	return claves, rotulos
}

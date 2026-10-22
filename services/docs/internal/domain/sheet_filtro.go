// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/xml"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// El AUTOFILTRO de una hoja de Calcus con sus CONDICIONES (`SheetAutoFilter.cond` y
// `.excl` del editor, sheets/filtro.ts): mayor/menor/entre, contiene/empieza/termina,
// vacías, N superiores/inferiores, por encima/debajo de la media y los valores excluidos.
// Aquí se traducen a lo que escribe Excel (`<autoFilter>` con `<filterColumn>`) y de
// vuelta, y se aplican a las filas (`hidden="1"`): Excel NO reaplica el filtro al abrir,
// así que sin las filas ocultas el libro llegaría con el embudo puesto y todo a la vista.
//
// Lo que no tiene equivalente se queda fuera, sin romper el resto: el filtro por COLOR
// (pide un `dxf` en styles.xml, y el export no escribe estilos de celda todavía).
// Las VISTAS de filtro son personales y no están en el modelo: no viajan aquí.

// sheetAutoFilter es el `SheetAutoFilter` del editor (fila y columnas en base 0).
type sheetAutoFilter struct {
	Row  int                   `json:"row"`
	C1   int                   `json:"c1"`
	C2   int                   `json:"c2"`
	Excl map[string][]string   `json:"excl,omitempty"`
	Cond map[string]filterCond `json:"cond,omitempty"`
}

// filterCond es la `FilterCondition` del editor.
type filterCond struct {
	Op      string `json:"op"`
	A       string `json:"a,omitempty"`
	B       string `json:"b,omitempty"`
	N       int    `json:"n,omitempty"`
	Percent bool   `json:"percent,omitempty"`
	Color   string `json:"color,omitempty"`
}

// parseNumES es `parseEsNumberStrict` del editor: un número tecleado a la española, y
// nada si lleva letras o símbolos («A1», «$50», «12 kg» no son un número).
func parseNumES(s string) (float64, bool) {
	if strings.ContainsFunc(s, func(r rune) bool { return !strings.ContainsRune("0123456789 \t.,+-", r) }) {
		return 0, false
	}
	limpio := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '.' || r == ',' || r == '-' {
			return r
		}
		return -1
	}, s)
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

// celdaFiltro es una celda del cuerpo de una columna filtrada: su valor calculado.
type celdaFiltro struct {
	v     formula.Value
	texto string
}

// columnaFiltro es lo que se calcula UNA vez de una condición y su columna: los operandos
// leídos, el umbral de los N superiores y la media.
type columnaFiltro struct {
	c          filterCond
	a          string // el primer operando, en minúsculas y recortado
	aNum, bNum float64
	aOK, bOK   bool
	nums       int
	umbral     float64
	hayUmbral  bool
	media      float64
}

func nuevaColumnaFiltro(c filterCond, cs []celdaFiltro) columnaFiltro {
	f := columnaFiltro{c: c, a: strings.ToLower(strings.TrimSpace(c.A))}
	f.aNum, f.aOK = parseNumES(c.A)
	f.bNum, f.bOK = parseNumES(c.B)
	var nums []float64
	for _, x := range cs {
		if n, ok := x.v.(float64); ok {
			nums = append(nums, n)
		}
	}
	f.nums = len(nums)
	if len(nums) == 0 {
		return f
	}
	var suma float64
	for _, n := range nums {
		suma += n
	}
	f.media = suma / float64(len(nums))
	if c.Op == "top" || c.Op == "bottom" {
		ord := slices.Clone(nums)
		slices.Sort(ord)
		if c.Op == "top" {
			slices.Reverse(ord)
		}
		cuenta := min(max(c.N, 1), len(ord))
		if c.Percent {
			cuenta = min(max(int(math.Ceil(float64(c.N)/100*float64(len(ord)))), 1), len(ord))
		}
		f.umbral, f.hayUmbral = ord[cuenta-1], true // incluye empates (criterio Excel)
	}
	return f
}

// pasa dice si una celda cumple la condición.
func (f columnaFiltro) pasa(x celdaFiltro) bool {
	n, esNum := x.v.(float64)
	bajo := strings.ToLower(strings.TrimSpace(x.texto))
	switch f.c.Op {
	case "gt", "ge", "lt", "le":
		return f.compara(n, esNum, bajo)
	case "eq", "ne":
		return f.igual(n, esNum, bajo) == (f.c.Op == "eq")
	case "between", "notBetween":
		return f.entre(n, esNum)
	case "contains", "notContains", "begins", "ends":
		return f.texto(bajo)
	case "blank", "notBlank":
		return (x.texto == "") == (f.c.Op == "blank")
	}
	return f.estadistica(n, esNum)
}

// entre: «entre» admite los extremos al revés; «no entre» ignora lo que no es un número (no
// está «fuera» de nada: se oculta, no se cuela).
func (f columnaFiltro) entre(n float64, esNum bool) bool {
	if !f.aOK || !f.bOK || !esNum {
		return false
	}
	dentro := n >= min(f.aNum, f.bNum) && n <= max(f.aNum, f.bNum)
	return dentro == (f.c.Op == "between")
}

func (f columnaFiltro) texto(bajo string) bool {
	switch f.c.Op {
	case "contains":
		return strings.Contains(bajo, f.a)
	case "notContains":
		return !strings.Contains(bajo, f.a)
	case "begins":
		return strings.HasPrefix(bajo, f.a)
	}
	return strings.HasSuffix(bajo, f.a)
}

// estadistica: N superiores/inferiores y sobre/bajo la media; el color no se evalúa aquí
// (ver la cabecera del fichero) y no filtra.
func (f columnaFiltro) estadistica(n float64, esNum bool) bool {
	switch f.c.Op {
	case "top":
		return esNum && f.hayUmbral && n >= f.umbral
	case "bottom":
		return esNum && f.hayUmbral && n <= f.umbral
	case "aboveAvg":
		return esNum && f.nums > 0 && n > f.media
	case "belowAvg":
		return esNum && f.nums > 0 && n < f.media
	}
	return true
}

func (f columnaFiltro) igual(n float64, esNum bool, bajo string) bool {
	if esNum && f.aOK {
		return n == f.aNum
	}
	return bajo == f.a
}

func (f columnaFiltro) compara(n float64, esNum bool, bajo string) bool {
	var cmp int
	switch {
	case esNum && f.aOK:
		cmp = compararNum(n, f.aNum)
	case bajo == "" || esNum != f.aOK:
		return false // texto contra número: no se ordenan
	default:
		cmp = strings.Compare(bajo, f.a)
	}
	switch f.c.Op {
	case "gt":
		return cmp > 0
	case "ge":
		return cmp >= 0
	case "lt":
		return cmp < 0
	}
	return cmp <= 0
}

// pasaCondicion dice, para cada celda, si pasa la condición (true = se ve). Es
// `evalFilterColumn` de sheets/filtro.ts: los N superiores y la media salen de TODA la
// columna del cuerpo, no de lo que dejan pasar otras columnas.
// ponytail: gt/lt entre textos comparan sin acentos ni orden «es»; el editor usa
// localeCompare. Afecta solo a qué filas llegan ocultas en un .xlsx con una columna de
// texto filtrada por «mayor que»; Excel las reaplica al pulsar «Volver a aplicar».
func pasaCondicion(c filterCond, cs []celdaFiltro) []bool {
	f := nuevaColumnaFiltro(c, cs)
	out := make([]bool, len(cs))
	for i, x := range cs {
		out[i] = f.pasa(x)
	}
	return out
}

func compararNum(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// filtroCalculado es lo que el export necesita de un autofiltro ya evaluado.
type filtroCalculado struct {
	ref          rango            // cabecera + cuerpo, base 1
	ocultas      map[int]bool     // filas (base 1) que el filtro esconde
	visibles     map[int][]string // por columna con exclusiones: valores distintos que SE VEN
	vacioVisible map[int]bool     // y si las vacías se ven
}

// ultimaFilaDelFiltro es la última fila (base 1) con algún dato en las columnas del filtro.
func ultimaFilaDelFiltro(af *sheetAutoFilter, sd sheetData) int {
	ultima := af.Row + 1
	for ref := range sd.Cells {
		r, c, ok := ParseA1(ref)
		if ok && c > af.C1 && c <= af.C2+1 && r > ultima {
			ultima = r
		}
	}
	return ultima
}

// calcularFiltro evalúa el autofiltro de una hoja con el motor del libro. `protegidas`
// son filas (base 1) que nunca se ocultan, como el total de una tabla. nil si el
// autofiltro no es válido. `eng` puede ser nil si no hay criterios que evaluar.
func calcularFiltro(af *sheetAutoFilter, sd sheetData, eng *formula.Engine, hoja string, protegidas map[int]bool) *filtroCalculado {
	if af == nil || af.Row < 0 || af.C1 < 0 || af.C2 < af.C1 {
		return nil
	}
	cab, ultima := af.Row+1, ultimaFilaDelFiltro(af, sd)
	fc := &filtroCalculado{
		ref:          rango{c1: af.C1 + 1, r1: cab, c2: af.C2 + 1, r2: ultima},
		ocultas:      map[int]bool{},
		visibles:     map[int][]string{},
		vacioVisible: map[int]bool{},
	}
	cuerpo := func(col int) []celdaFiltro {
		out := make([]celdaFiltro, 0, ultima-cab)
		for r := cab + 1; r <= ultima; r++ {
			v, _ := eng.GetValue(hoja, A1Ref(r, col+1))
			out = append(out, celdaFiltro{v: v, texto: formula.Display(v)})
		}
		return out
	}
	ocultar := func(i int) {
		if r := cab + 1 + i; !protegidas[r] {
			fc.ocultas[r] = true
		}
	}
	for clave, excluidos := range af.Excl {
		if c, ok := columnaDelFiltro(af, clave); ok && len(excluidos) > 0 {
			fc.excluir(c, excluidos, cuerpo(c), ocultar)
		}
	}
	for clave, cond := range af.Cond {
		c, ok := columnaDelFiltro(af, clave)
		if !ok || cond.Op == "fill" || cond.Op == "fontColor" {
			continue
		}
		for i, pasa := range pasaCondicion(cond, cuerpo(c)) {
			if !pasa {
				ocultar(i)
			}
		}
	}
	return fc
}

// columnaDelFiltro lee la clave de una columna y comprueba que cae dentro del autofiltro.
func columnaDelFiltro(af *sheetAutoFilter, clave string) (int, bool) {
	c, err := strconv.Atoi(clave)
	return c, err == nil && c >= af.C1 && c <= af.C2
}

// excluir oculta las filas cuyo valor está excluido y apunta los valores que SE VEN.
func (fc *filtroCalculado) excluir(col int, excluidos []string, cuerpo []celdaFiltro, ocultar func(int)) {
	var distintos []string
	for i, x := range cuerpo {
		switch {
		case slices.Contains(excluidos, x.texto):
			ocultar(i)
		case x.texto == "":
			fc.vacioVisible[col] = true
		case !slices.Contains(distintos, x.texto):
			distintos = append(distintos, x.texto)
		}
	}
	fc.visibles[col] = distintos
}

// valorXlsx es un operando como lo lee Excel: el número en notación plana, o el texto.
func valorXlsx(s string) string {
	if n, ok := parseNumES(s); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return strings.TrimSpace(s)
}

// comodinesXlsx escapa lo que Excel lee como comodín (`~`, `*`, `?`).
var comodinesXlsx = strings.NewReplacer("~", "~~", "*", "~*", "?", "~?")

func clienteXlsx(op, val string) string {
	o := ""
	if op != "" {
		o = ` operator="` + op + `"`
	}
	return `<customFilter` + o + ` val="` + xmlEscape(val) + `"/>`
}

var operadorCmpXlsx = map[string]string{
	"gt": "greaterThan", "ge": "greaterThanOrEqual", "lt": "lessThan", "le": "lessThanOrEqual",
}

// criterioXlsx es lo que va dentro del `<filterColumn>` para una condición, o "" si Excel no
// tiene equivalente (el color).
func criterioXlsx(cond filterCond) string {
	a, b := valorXlsx(cond.A), valorXlsx(cond.B)
	txt := comodinesXlsx.Replace(a)
	custom := func(items ...string) string { return `<customFilters>` + strings.Join(items, "") + `</customFilters>` }
	switch cond.Op {
	case "gt", "ge", "lt", "le":
		return custom(clienteXlsx(operadorCmpXlsx[cond.Op], a))
	case "eq":
		return custom(clienteXlsx("", txt))
	case "ne":
		return custom(clienteXlsx("notEqual", txt))
	case "between":
		return `<customFilters and="1">` + clienteXlsx("greaterThanOrEqual", a) + clienteXlsx("lessThanOrEqual", b) + `</customFilters>`
	case "notBetween":
		return custom(clienteXlsx("lessThan", a), clienteXlsx("greaterThan", b))
	case "contains":
		return custom(clienteXlsx("", "*"+txt+"*"))
	case "notContains":
		return custom(clienteXlsx("notEqual", "*"+txt+"*"))
	case "begins":
		return custom(clienteXlsx("", txt+"*"))
	case "ends":
		return custom(clienteXlsx("", "*"+txt))
	case "blank":
		return `<filters blank="1"/>`
	case "notBlank":
		return custom(clienteXlsx("notEqual", " "))
	case "top", "bottom":
		return `<top10 top="` + bit(cond.Op == "top") + `" percent="` + bit(cond.Percent) + `" val="` + strconv.Itoa(cond.N) + `"/>`
	case "aboveAvg":
		return `<dynamicFilter type="aboveAverage"/>`
	case "belowAvg":
		return `<dynamicFilter type="belowAverage"/>`
	}
	return ""
}

func bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// valoresXlsx es el criterio de una columna con valores excluidos: Excel guarda los que SE VEN.
func valoresXlsx(visibles []string, vacio bool) string {
	var b strings.Builder
	b.WriteString(`<filters`)
	if vacio {
		b.WriteString(` blank="1"`)
	}
	b.WriteString(`>`)
	for _, v := range visibles {
		b.WriteString(`<filter val="` + xmlEscape(v) + `"/>`)
	}
	b.WriteString(`</filters>`)
	return b.String()
}

// autoFilterXlsx es el `<autoFilter>` de una hoja (o de una tabla, con su propio `ref`).
// `ref` es el rango que cubre; los `colId` van relativos a él.
func autoFilterXlsx(af *sheetAutoFilter, fc *filtroCalculado, ref rango) string {
	if af == nil || fc == nil {
		return `<autoFilter ref="` + ref.a1() + `"/>`
	}
	var cols strings.Builder
	for c := af.C1; c <= af.C2; c++ {
		id := c + 1 - ref.c1
		if id < 0 || c+1 > ref.c2 {
			continue
		}
		clave := strconv.Itoa(c)
		criterio := ""
		if len(af.Excl[clave]) > 0 {
			criterio = valoresXlsx(fc.visibles[c], fc.vacioVisible[c])
		} else if cond, ok := af.Cond[clave]; ok {
			criterio = criterioXlsx(cond)
		}
		if criterio != "" {
			cols.WriteString(`<filterColumn colId="` + strconv.Itoa(id) + `">` + criterio + `</filterColumn>`)
		}
	}
	if cols.Len() == 0 {
		return `<autoFilter ref="` + ref.a1() + `"/>`
	}
	return `<autoFilter ref="` + ref.a1() + `">` + cols.String() + `</autoFilter>`
}

// ---- Importar ----

type xlsxCustomFilters struct {
	And   string `xml:"and,attr"`
	Items []struct {
		Op  string `xml:"operator,attr"`
		Val string `xml:"val,attr"`
	} `xml:"customFilter"`
}

type xlsxFilterCol struct {
	ID      int `xml:"colId,attr"`
	Filters *struct {
		Blank  string `xml:"blank,attr"`
		Filter []struct {
			Val string `xml:"val,attr"`
		} `xml:"filter"`
		Fechas []struct{} `xml:"dateGroupItem"`
	} `xml:"filters"`
	Custom *xlsxCustomFilters `xml:"customFilters"`
	Top10  *struct {
		Top     string  `xml:"top,attr"`
		Percent string  `xml:"percent,attr"`
		Val     float64 `xml:"val,attr"`
	} `xml:"top10"`
	Dynamic *struct {
		Type string `xml:"type,attr"`
	} `xml:"dynamicFilter"`
}

// xlsxAutoFilter es un `<autoFilter>` de una hoja o de una tabla.
type xlsxAutoFilter struct {
	Ref  string          `xml:"ref,attr"`
	Cols []xlsxFilterCol `xml:"filterColumn"`
}

var reComodinXlsx = regexp.MustCompile(`[*?]`)

// desescaparXlsx deshace el escape de comodines (`~*` → `*`).
func desescaparXlsx(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '~' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// textoDeCliente interpreta el `val` de un customFilter con comodines.
// ok=false si el patrón es más complejo que «contiene / empieza / termina».
func textoDeCliente(val string, noIgual bool) (filterCond, bool) {
	ini := strings.HasPrefix(val, "*")
	fin := strings.HasSuffix(val, "*") && !strings.HasSuffix(val, "~*")
	nucleo := strings.TrimPrefix(val, "*")
	if fin {
		nucleo = strings.TrimSuffix(nucleo, "*")
	}
	// Un comodín sin escapar dentro (o un patrón vacío) no tiene equivalente.
	sinEsc := strings.NewReplacer("~*", "", "~?", "", "~~", "").Replace(nucleo)
	if reComodinXlsx.MatchString(sinEsc) || nucleo == "" && (ini || fin) {
		return filterCond{}, false
	}
	op := map[[3]bool]string{
		{false, false, false}: "eq", {false, false, true}: "ne",
		{true, true, false}: "contains", {true, true, true}: "notContains",
		{false, true, false}: "begins", {true, false, false}: "ends",
	}[[3]bool{ini, fin, noIgual}]
	if op == "" {
		return filterCond{}, false
	}
	return filterCond{Op: op, A: desescaparXlsx(nucleo)}, true
}

// condicionDeXlsx traduce las condiciones de UNA columna; ok=false si no hay equivalente.
func condicionDeXlsx(col *xlsxCustomFilters) (filterCond, bool) {
	cmp := map[string]string{"greaterThan": "gt", "greaterThanOrEqual": "ge", "lessThan": "lt", "lessThanOrEqual": "le"}
	it := col.Items
	switch len(it) {
	case 1:
		if it[0].Op == "notEqual" && it[0].Val == " " {
			return filterCond{Op: "notBlank"}, true
		}
		if o, ok := cmp[it[0].Op]; ok {
			return filterCond{Op: o, A: it[0].Val}, true
		}
		if it[0].Op == "" || it[0].Op == "equal" || it[0].Op == "notEqual" {
			return textoDeCliente(it[0].Val, it[0].Op == "notEqual")
		}
	case 2:
		if verdadero(col.And) && it[0].Op == "greaterThanOrEqual" && it[1].Op == "lessThanOrEqual" {
			return filterCond{Op: "between", A: it[0].Val, B: it[1].Val}, true
		}
		if !verdadero(col.And) && it[0].Op == "lessThan" && it[1].Op == "greaterThan" {
			return filterCond{Op: "notBetween", A: it[0].Val, B: it[1].Val}, true
		}
	}
	return filterCond{}, false
}

// condicionDeColumna es la condición de una `<filterColumn>` que no es una lista de valores.
func condicionDeColumna(c xlsxFilterCol) (filterCond, bool) {
	switch {
	case c.Filters != nil && c.Filters.Blank == "1" && len(c.Filters.Filter) == 0 && len(c.Filters.Fechas) == 0:
		return filterCond{Op: "blank"}, true
	case c.Custom != nil:
		return condicionDeXlsx(c.Custom)
	case c.Top10 != nil && c.Top10.Val >= 1:
		op := "top"
		if c.Top10.Top == "0" || c.Top10.Top == "false" {
			op = "bottom"
		}
		return filterCond{Op: op, N: int(c.Top10.Val), Percent: c.Top10.Percent == "1" || c.Top10.Percent == "true"}, true
	case c.Dynamic != nil && c.Dynamic.Type == "aboveAverage":
		return filterCond{Op: "aboveAvg"}, true
	case c.Dynamic != nil && c.Dynamic.Type == "belowAverage":
		return filterCond{Op: "belowAvg"}, true
	}
	return filterCond{}, false
}

// excluidosDeColumna son los valores literales de la columna que la lista de Excel NO deja
// ver. El valor de una fórmula no se conoce al importar, y una columna de fechas (que Excel
// agrupa en `dateGroupItem`) no se toca.
func excluidosDeColumna(c xlsxFilterCol, ref rango, col int, cells map[string]string) []string {
	if c.Filters == nil || len(c.Filters.Fechas) > 0 || len(c.Filters.Filter) == 0 {
		return nil
	}
	visibles := map[string]bool{}
	for _, f := range c.Filters.Filter {
		visibles[f.Val] = true
	}
	var excl []string
	for r := ref.r1 + 1; r <= ref.r2; r++ {
		t := cellRaw(cells[A1Ref(r, col+1)])
		if strings.HasPrefix(t, "=") || visibles[t] || t == "" && c.Filters.Blank == "1" || slices.Contains(excl, t) {
			continue
		}
		excl = append(excl, t)
	}
	return excl
}

// autoFilterDeXlsx convierte un `<autoFilter>` en el del editor. Una condición sin
// equivalente se descarta (la columna queda sin filtrar); los valores se convierten en
// EXCLUIDOS comparándolos con los que hay en la columna.
func autoFilterDeXlsx(x xlsxAutoFilter, cells map[string]string) *sheetAutoFilter {
	ref, ok := leerRango(x.Ref)
	if !ok {
		return nil
	}
	af := &sheetAutoFilter{Row: ref.r1 - 1, C1: ref.c1 - 1, C2: ref.c2 - 1}
	for _, c := range x.Cols {
		col := ref.c1 - 1 + c.ID
		if col < af.C1 || col > af.C2 {
			continue
		}
		clave := strconv.Itoa(col)
		if excl := excluidosDeColumna(c, ref, col, cells); len(excl) > 0 {
			if af.Excl == nil {
				af.Excl = map[string][]string{}
			}
			af.Excl[clave] = excl
		} else if cond, ok := condicionDeColumna(c); ok {
			if af.Cond == nil {
				af.Cond = map[string]filterCond{}
			}
			af.Cond[clave] = cond
		}
	}
	return af
}

// ---- Pegamento del export a .xlsx ----

// motorParaFiltros es el motor del libro, solo si alguna hoja tiene autofiltro con algo
// que evaluar (valores excluidos o condiciones): un libro sin filtros no paga el cálculo.
func motorParaFiltros(m *sheetModel) (*formula.Engine, []string) {
	for _, sd := range m.Sheets {
		if af := sd.AutoFilter; af != nil && (len(af.Excl) > 0 || len(af.Cond) > 0) {
			return motorDelLibro(m)
		}
	}
	// Sin criterios no hay filas que ocultar, pero el autofiltro se exporta igual: el
	// motor solo hace falta para calcular, y un filtro sin criterios no calcula nada.
	return nil, nil
}

// filasDeTotales son las filas (base 1) que el filtro no oculta nunca: el total de una tabla.
func filasDeTotales(sd sheetData) map[int]bool {
	out := map[int]bool{}
	for _, t := range sd.Tables {
		if r, ok := leerRango(t.Range); ok && t.TotalsRow {
			out[r.r2] = true
		}
	}
	return out
}

// atributoOculta es el ` hidden="1"` de una fila que el autofiltro esconde.
func atributoOculta(sd sheetData, fila int) string {
	if sd.filtro != nil && sd.filtro.ocultas[fila] {
		return ` hidden="1"`
	}
	return ""
}

// autoFilterHoja es el `<autoFilter>` de la hoja (justo tras `sheetData`, donde lo pide el
// esquema), o "" si no hay o si cae sobre una tabla: ahí el filtro lo lleva la propia tabla
// y un segundo `<autoFilter>` solapado le haría a Excel «reparar» el libro.
func autoFilterHoja(sd sheetData) string {
	af := sd.AutoFilter
	if af == nil || sd.filtro == nil {
		return ""
	}
	for _, t := range sd.Tables {
		if r, ok := leerRango(t.Range); ok && r.r1 <= sd.filtro.ref.r2 && r.r2 >= sd.filtro.ref.r1 &&
			r.c1 <= sd.filtro.ref.c2 && r.c2 >= sd.filtro.ref.c1 {
			return ""
		}
	}
	return autoFilterXlsx(af, sd.filtro, sd.filtro.ref)
}

// autoFilterTabla es el `<autoFilter>` de una tabla con cabecera: con las condiciones del
// autofiltro de la hoja si éste es el de su cabecera, y sin ellas si no.
func autoFilterTabla(sd sheetData, cuerpo rango) string {
	if af := sd.AutoFilter; af != nil && sd.filtro != nil && af.Row+1 == cuerpo.r1 {
		return autoFilterXlsx(af, sd.filtro, cuerpo)
	}
	return `<autoFilter ref="` + cuerpo.a1() + `"/>`
}

// autoFilterDeTabla es el filtro de una tabla, solo si trae criterios: una tabla con sus
// flechas y nada filtrado no deja un autofiltro en la hoja (como hasta ahora).
func autoFilterDeTabla(raw []byte, cells map[string]string) *sheetAutoFilter {
	var t struct {
		AutoFilter *xlsxAutoFilter `xml:"autoFilter"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &t) != nil || t.AutoFilter == nil || len(t.AutoFilter.Cols) == 0 {
		return nil
	}
	af := autoFilterDeXlsx(*t.AutoFilter, cells)
	if af == nil || len(af.Excl) == 0 && len(af.Cond) == 0 {
		return nil
	}
	return af
}

// baseDeFiltroXlsx es el nombre que Excel reserva para el rango de un autofiltro de hoja
// (`_xlnm._FilterDatabase`, oculto). Excel lo escribe siempre; LibreOffice lo USA para
// saber dónde está el autofiltro, y sin él abre la hoja sin embudos.
func baseDeFiltroXlsx(hoja int, nombre string, ref rango) string {
	return `<definedName name="_xlnm._FilterDatabase" localSheetId="` + strconv.Itoa(hoja) + `" hidden="1">` +
		xmlEscape(ref.absoluto(nombre)) + `</definedName>`
}

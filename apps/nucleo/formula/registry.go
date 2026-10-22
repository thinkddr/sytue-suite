// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// RefRange es un rango resuelto: rectángulo [C1..C2] × [R1..R2] de una hoja, 0-based.
type RefRange struct {
	Sheet          string
	C1, R1, C2, R2 int
}

// Here es la celda que se está evaluando (0-based).
type Here struct {
	Sheet    string
	Col, Row int
}

// TableResolver es el resolutor INYECTABLE de tablas (lo provee el editor, dueño del
// mapa tabla→rango). Devuelve el rango, o un error concreto (#VALUE! si `@` cae fuera,
// #REF! si la columna no existe), o (nil, "") y el motor asume #REF!.
type TableResolver func(q TableQuery, here Here) (*RefRange, Error)

// Arg es un argumento de función, PEREZOSO: el motor no lo evalúa hasta que la función
// lo pide, y eso es lo que permite el cortocircuito de IF/AND/OR.
type Arg struct {
	e   *Engine
	n   *nodo
	env *entorno

	sv         Value
	mv         Matrix
	ref        *RefRange
	svOK, mvOK bool
	refOK      bool
}

// Value da el argumento como escalar: un rango 1×1 colapsa a su valor y uno de varias
// celdas da #VALUE!. Memorizado.
func (a *Arg) Value() Value {
	if !a.svOK {
		a.sv, a.svOK = collapse(a.e.evalNodeMatrix(a.n, a.env)), true
	}
	return a.sv
}

// Matrix da el argumento como matriz; un escalar se envuelve en 1×1. Memorizado aparte
// de Value, como en el original: pedir los dos evalúa dos veces.
func (a *Arg) Matrix() Matrix {
	if !a.mvOK {
		a.mv, a.mvOK = a.e.evalNodeMatrix(a.n, a.env), true
	}
	return a.mv
}

// Literal dice si el argumento es un VALOR escrito en la fórmula —un texto entre comillas o
// el resultado de un operador— y no una referencia ni una llamada: es lo que Excel convierte
// a número en SUM, AVERAGE, COUNT… (Arg.literal de registry.ts).
func (a *Arg) Literal() bool {
	if a.n == nil {
		return false
	}
	c := a.n.clase
	return c == nStr || c == nBinary || c == nUnary || c == nPercent
}

// numeroDeLiteral es el número de un texto escrito como argumento: como CoerceNumber, pero
// el texto vacío no es un cero sino #VALUE!.
func numeroDeLiteral(s string) (float64, Error) {
	if jsTrim(s) == "" {
		return 0, ErrValue
	}
	return CoerceNumber(s)
}

// Ref da el rango si el argumento es una referencia LITERAL (A1, A1:B2, Hoja1!A1 o una
// tabla que resuelve), y nil si no.
func (a *Arg) Ref() *RefRange {
	if a.refOK {
		return a.ref
	}
	a.refOK = true
	if a.n == nil {
		return nil
	}
	switch a.n.clase {
	case nRef:
		rr := toRefRange(a.n.a, a.env.hoja)
		a.ref = &rr
	case nRange:
		rr := a.e.rangeToRefRange(a.n, a.env.hoja)
		a.ref = &rr
	case nStructured:
		if rr, err := a.e.resolveTableRef(a.n, Here{a.env.hoja, a.env.col, a.env.fila}); err == "" {
			a.ref = rr
		}
	default: // el argumento no es una referencia: se queda en nil
	}
	return a.ref
}

// scalarArg envuelve un valor ya calculado (para difundir una función celda a celda).
func scalarArg(v Value) *Arg {
	return &Arg{sv: v, svOK: true, mv: Matrix{{v}}, mvOK: true, refOK: true}
}

// FnCtx es el contexto que el motor pasa a cada función.
type FnCtx struct {
	e    *Engine
	Here Here
}

// Now es el reloj inyectable (NOW/TODAY).
func (c *FnCtx) Now() time.Time { return c.e.clock() }

// Random es la fuente de azar inyectable (RAND/RANDBETWEEN).
func (c *FnCtx) Random() float64 { return c.e.rng() }

// Resolve lee un rango como matriz de valores.
func (c *FnCtx) Resolve(rr RefRange) Matrix { return c.e.resolveRange(rr) }

// ParseRef parsea «Hoja1!A1:B2» a rango (INDIRECT); nil si no es una referencia.
func (c *FnCtx) ParseRef(text string) *RefRange { return c.e.parseRefText(text, c.Here.Sheet) }

// IsFormula dice si la celda superior izquierda del rango es una fórmula.
func (c *FnCtx) IsFormula(rr RefRange) bool {
	cel := c.e.cells[celdaK(rr.Sheet, rr.C1, rr.R1)]
	return cel != nil && (cel.ast != nil || cel.rota)
}

// FormulaText es el texto de la fórmula de la celda superior izquierda del rango, tal como
// se escribió, o false si no es una fórmula (FORMULATEXT).
func (c *FnCtx) FormulaText(rr RefRange) (string, bool) {
	cel := c.e.cells[celdaK(rr.Sheet, rr.C1, rr.R1)]
	if cel == nil || (cel.ast == nil && !cel.rota) {
		return "", false
	}
	return cel.raw, true
}

// SheetNames son los nombres de las hojas del libro, en orden (SHEET, SHEETS).
func (c *FnCtx) SheetNames() []string { return c.e.SheetNames() }

// ResolveTable resuelve una referencia estructurada anclada en la celda actual.
func (c *FnCtx) ResolveTable(q TableQuery) (*RefRange, Error) {
	if c.e.tableResolver == nil {
		return nil, ""
	}
	return c.e.tableResolver(q, c.Here)
}

// SubtotalCell dice si la celda es un SUBTOTAL o un AGGREGATE, o los lleva dentro: lo
// que esas dos funciones se saltan para no contar dos veces lo que ya resume otra.
func (c *FnCtx) SubtotalCell(sheet string, col, row int) bool {
	cel := c.e.cells[celdaK(sheet, col, row)]
	if cel == nil || cel.ast == nil {
		return false
	}
	if cel.sub == 0 {
		cel.sub = 2
		if llevaResumen(cel.ast) {
			cel.sub = 1
		}
	}
	return cel.sub == 1
}

// RowHidden es cómo está la fila: 0 se ve, 1 oculta a mano (un grupo contraído) y 2
// oculta por el filtro.
func (c *FnCtx) RowHidden(sheet string, row int) uint8 { return c.e.ocultas[sheet][row] }

// llevaResumen dice si el árbol llama a SUBTOTAL o a AGGREGATE en algún punto.
func llevaResumen(n *nodo) bool {
	if n == nil {
		return false
	}
	if n.clase == nCall && (strings.EqualFold(n.str, "SUBTOTAL") || strings.EqualFold(n.str, "AGGREGATE")) {
		return true
	}
	if llevaResumen(n.izq) || llevaResumen(n.der) {
		return true
	}
	return slices.ContainsFunc(n.args, llevaResumen)
}

// fn es la firma de TODA función de librería: devuelve un Value o una Matrix.
type fn = func(c *FnCtx, args []*Arg) any

// registro es la tabla nombre→implementación, en mayúsculas.
var registro = map[string]fn{}

// registrar fusiona una familia de funciones. Lo llaman los init() de cada fichero.
func registrar(tabla map[string]fn) {
	for nombre, f := range tabla {
		registro[strings.ToUpper(nombre)] = f
	}
}

// funcionesDelMotor son las que despacha el motor y no el registro: reciben el AST (un
// LAMBDA o un nombre) o necesitan el entorno (los parámetros del LAMBDA en curso). Es
// FUNCIONES_DEL_MOTOR de catalogo.ts.
var funcionesDelMotor = []string{
	"LET", "LAMBDA", "MAP", "REDUCE", "SCAN", "MAKEARRAY", "BYROW", "BYCOL",
	"ISOMITTED", "GROUPBY", "PIVOTBY",
}

// FunctionNames son los nombres (en mayúsculas) de todas las funciones que se pueden
// escribir en una fórmula —las del registro y las del motor—, ordenados. Para el
// autocompletado del editor (es nombresDeFunciones de catalogo.ts).
func FunctionNames() []string {
	out := make([]string, 0, len(registro)+len(funcionesDelMotor))
	for n := range registro {
		out = append(out, n)
	}
	out = append(out, funcionesDelMotor...)
	sort.Strings(out)
	return out
}

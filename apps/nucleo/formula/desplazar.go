// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"regexp"
	"strconv"
	"strings"
)

// ShiftRefs es `shiftRefs` de `packages/office/src/editor/sheets/format.ts`: desplaza las
// referencias RELATIVAS de una fórmula `dc` columnas y `dr` filas —las ancladas con
// «$» no se mueven—. Es lo que hacen copiar y pegar y el autorelleno: «=A1+1» copiada
// una fila más abajo es «=A2+1».
//
// Va por el parser y un serializador, NUNCA por expresiones regulares: un «A1» dentro
// de una cadena o de un nombre de hoja no es una referencia. Si la fórmula no parsea se
// devuelve tal cual; si alguna referencia se sale por arriba o por la izquierda, el
// literal «#REF!» sin «=», como la web.
func ShiftRefs(formula string, dc, dr int) string {
	n, err := parseFormula(formula)
	if err != nil {
		return formula
	}
	ok := true
	mover := func(r *CellRef, col, fila bool) {
		if col && !r.ColAbs {
			r.Col += dc
		}
		if fila && !r.RowAbs {
			r.Row += dr
		}
		if (col && r.Col < 0) || (fila && r.Row < 0) {
			ok = false
		}
	}
	var andar func(*nodo)
	andar = func(n *nodo) {
		switch n.clase {
		case nRef:
			mover(&n.a, true, true)
		case nRange:
			// Un rango de eje entero mueve SOLO su eje: una columna entera no baja al
			// arrastrar en vertical, y su fila fantasma no debe dar #REF!.
			col, fila := n.eje != ejeFila, n.eje != ejeCol
			mover(&n.a, col, fila)
			mover(&n.z, col, fila)
		case nUnary, nPercent:
			andar(n.izq)
		case nBinary:
			andar(n.izq)
			andar(n.der)
		case nCall:
			for _, a := range n.args {
				andar(a)
			}
		case nNum, nStr, nBool, nName, nStructured, nVacio, nArray:
		}
	}
	andar(n)
	if !ok {
		return "#REF!"
	}
	return "=" + serializar(n)
}

// Precedencias de `format.ts` (`BIN_PREC`, `PREC_UNARY`…): deciden dónde hacen falta
// paréntesis para que el texto vuelva a dar el MISMO árbol.
var precBinaria = map[string]int{
	"=": 1, "<>": 1, "<": 1, ">": 1, "<=": 1, ">=": 1,
	"&": 2, "+": 3, "-": 3, "*": 4, "/": 4, "^": 5,
}

const (
	precUnaria     = 6
	precPorcentaje = 7
	precAtomo      = 8
)

func precedencia(n *nodo) int {
	switch n.clase {
	case nBinary:
		if p, ok := precBinaria[n.str]; ok {
			return p
		}
	case nUnary:
		return precUnaria
	case nPercent:
		return precPorcentaje
	case nNum, nStr, nBool, nRef, nRange, nName, nStructured, nCall, nVacio, nArray:
	}
	return precAtomo
}

var hojaSimple = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.$]*$`)

// prefijoHoja es «Hoja1!» o «'Mi hoja'!»; nada si la referencia no lleva hoja.
func prefijoHoja(hoja string) string {
	if hoja == "" {
		return ""
	}
	if hojaSimple.MatchString(hoja) {
		return hoja + "!"
	}
	return "'" + strings.ReplaceAll(hoja, "'", "''") + "'!"
}

func anclaje(abs bool) string {
	if abs {
		return "$"
	}
	return ""
}

func textoRef(r CellRef, conHoja bool) string {
	s := ""
	if conHoja {
		s = prefijoHoja(r.Sheet)
	}
	return s + anclaje(r.ColAbs) + NumToCol(r.Col) + anclaje(r.RowAbs) + strconv.Itoa(r.Row+1)
}

func textoEje(r CellRef, eje int, conHoja bool) string {
	s := ""
	if conHoja {
		s = prefijoHoja(r.Sheet)
	}
	if eje == ejeCol {
		return s + anclaje(r.ColAbs) + NumToCol(r.Col)
	}
	return s + anclaje(r.RowAbs) + strconv.Itoa(r.Row+1)
}

func envolver(n *nodo, minimo int) string {
	s := serializar(n)
	if precedencia(n) < minimo {
		return "(" + s + ")"
	}
	return s
}

// serializar es `serialize`: el árbol como texto, con paréntesis solo donde la
// precedencia los exige y «;» entre argumentos (el parser admite los dos).
func serializar(n *nodo) string {
	switch n.clase {
	case nNum:
		return FormatNumber(n.num)
	case nStr:
		return `"` + strings.ReplaceAll(n.str, `"`, `""`) + `"`
	case nBool:
		if n.b {
			return "TRUE"
		}
		return "FALSE"
	case nRef:
		return textoRef(n.a, true)
	case nRange:
		// El extremo derecho nunca lleva hoja («Hoja1!A1:B2»): así lo exige el parser.
		if n.eje != ejeNinguno {
			return textoEje(n.a, n.eje, true) + ":" + textoEje(n.z, n.eje, false)
		}
		return textoRef(n.a, true) + ":" + textoRef(n.z, false)
	case nName:
		return prefijoHoja(n.hoja) + n.str
	case nUnary:
		return n.str + envolver(n.izq, precUnaria)
	case nPercent:
		return envolver(n.izq, precPorcentaje) + "%"
	case nBinary:
		p := precedencia(n)
		// «^» asocia por la derecha y el resto por la izquierda.
		izq, der := p, p+1
		if n.str == "^" {
			izq, der = p+1, p
		}
		return envolver(n.izq, izq) + n.str + envolver(n.der, der)
	case nCall:
		args := make([]string, len(n.args))
		for i, a := range n.args {
			args[i] = serializar(a)
		}
		return n.str + "(" + strings.Join(args, ";") + ")"
	case nStructured:
		return serializarEstructurada(n.tabla)
	case nVacio:
		return "" // un argumento omitido se escribe como nada: «F(a;;c)»
	case nArray:
		return serializarMatriz(n.matriz)
	}
	return ""
}

// serializarEstructurada es `serializeStructured`, con sus mismas cuatro formas.
func serializarEstructurada(q TableQuery) string {
	area := map[TableArea]string{AreaAll: "#Todo", AreaHeaders: "#Cabeceras", AreaTotals: "#Totales"}
	col := q.Column
	if q.HasColumn && (strings.HasPrefix(col, "@") || strings.HasPrefix(col, "#")) {
		col = "'" + col
	}
	var dentro string
	switch {
	case q.ThisRow && !q.HasColumn:
		dentro = "#Esta fila"
	case q.ThisRow:
		dentro = "@" + col
	case q.Area == AreaData && !q.HasColumn:
		dentro = "#Datos"
	case q.Area == AreaData:
		dentro = col
	case !q.HasColumn:
		dentro = area[q.Area]
	default:
		dentro = "[" + area[q.Area] + "],[" + col + "]"
	}
	return q.Table + "[" + dentro + "]"
}

// serializarMatriz escribe una constante matricial: «,» entre columnas y «;» entre filas,
// siempre (la constante no tiene variante regional).
func serializarMatriz(m Matrix) string {
	filas := make([]string, len(m))
	for i, f := range m {
		cols := make([]string, len(f))
		for k, v := range f {
			switch x := v.(type) {
			case Error:
				cols[k] = string(x)
			case string:
				if x != "" {
					cols[k] = `"` + strings.ReplaceAll(x, `"`, `""`) + `"`
				}
			case bool:
				cols[k] = map[bool]string{true: "TRUE", false: "FALSE"}[x]
			case float64:
				cols[k] = FormatNumber(x)
			}
		}
		filas[i] = strings.Join(cols, ",")
	}
	return "{" + strings.Join(filas, ";") + "}"
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"strings"
)

// Agregados condicionales y el PARSER DE CRITERIOS de hoja de cálculo
// (functions/statsif.ts).
//
// Un criterio es un escalar: número o booleano (igualdad directa), o texto con un
// operador opcional al inicio (">=", "<=", "<>", ">", "<", "=") y un operando. El operando
// es numérico si parsea; si no es texto y, para «=» y «<>», admite los comodines «*», «?»
// y «~» de escape. El texto compara sin distinguir mayúsculas.

const (
	critNum = iota
	critTexto
	critVacio
	critBool
)

type criterio struct {
	op    string
	clase int
	n     float64
	b     bool
	texto string
	regex *regexp.Regexp // patrón de comodines, solo para «=» y «<>» textual
}

// wildcardToRegex compila un patrón con comodines de hoja, sin distinguir mayúsculas.
func wildcardToRegex(patron string) *regexp.Regexp {
	var re strings.Builder
	re.WriteString(`(?i)^`)
	runas := []rune(patron)
	for i := 0; i < len(runas); i++ {
		switch ch := runas[i]; {
		case ch == '~' && i+1 < len(runas) && strings.ContainsRune("*?~", runas[i+1]):
			i++
			re.WriteString(regexp.QuoteMeta(string(runas[i])))
		case ch == '*':
			re.WriteString(`.*`)
		case ch == '?':
			re.WriteString(`.`)
		default:
			re.WriteString(regexp.QuoteMeta(string(ch))) // también el «~» suelto
		}
	}
	re.WriteString(`$`)
	return regexp.MustCompile(re.String())
}

// parseCriterion parsea un valor-criterio. Lo reutilizan las funciones de base de datos.
func parseCriterion(v Value) (criterio, Error) {
	var s string
	switch x := v.(type) {
	case Error:
		return criterio{}, x
	case float64:
		return criterio{op: "=", clase: critNum, n: x}, ""
	case bool:
		return criterio{op: "=", clase: critBool, b: x}, ""
	case string:
		s = x
	}
	op, resto := "=", s
	switch {
	case strings.HasPrefix(s, ">="), strings.HasPrefix(s, "<="), strings.HasPrefix(s, "<>"):
		op, resto = s[:2], s[2:]
	case strings.HasPrefix(s, ">"), strings.HasPrefix(s, "<"), strings.HasPrefix(s, "="):
		op, resto = s[:1], s[1:]
	}
	if resto == "" {
		return criterio{op: op, clase: critVacio}, ""
	}
	if jsTrim(resto) != "" {
		if n, err := CoerceNumber(resto); err == "" {
			return criterio{op: op, clase: critNum, n: n, texto: resto}, ""
		}
	}
	c := criterio{op: op, clase: critTexto, texto: resto}
	if op == "=" || op == "<>" {
		switch jsUpper(resto) {
		case "TRUE":
			return criterio{op: op, clase: critBool, b: true, texto: resto}, ""
		case "FALSE":
			return criterio{op: op, clase: critBool, texto: resto}, ""
		}
		c.regex = wildcardToRegex(resto)
	}
	return c, ""
}

// orden aplica un operador de orden a una comparación -1/0/1.
func orden(op string, cmp int) bool {
	switch op {
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	}
	return false
}

// matches: ¿la celda cumple el criterio? Las celdas de error y en blanco nunca cuentan
// (salvo el blanco con operando vacío en «=»). El orden numérico solo aplica a números;
// los comodines, solo a texto; «<>» cuenta todo lo no igual, menos el blanco.
func matches(celda Value, c criterio) bool {
	if IsError(celda) {
		return false
	}
	blanco := celda == ""
	if c.clase == critVacio {
		return (c.op == "=" && blanco) || (c.op == "<>" && !blanco)
	}
	if blanco {
		return false
	}
	switch c.clase {
	case critBool:
		b, ok := celda.(bool)
		return (ok && b == c.b) != (c.op == "<>")
	case critNum:
		n, ok := celda.(float64)
		switch {
		case c.op == "=":
			return ok && n == c.n
		case c.op == "<>":
			return !(ok && n == c.n)
		case !ok:
			return false
		}
		return ordenNum(c.op, n, c.n)
	}
	s, ok := celda.(string)
	if c.op == "=" || c.op == "<>" {
		if !ok {
			return c.op == "<>" // lo que no es texto nunca es «=» y siempre es «<>»
		}
		return c.regex.MatchString(s) == (c.op == "=")
	}
	// Orden con operando textual: Excel segrega por tipo y solo compara contra texto.
	if !ok {
		return false
	}
	cmp, _ := CompareValues(s, c.texto)
	return orden(c.op, cmp)
}

// ordenNum aplica un operador de orden a dos números; con NaN ninguno es cierto.
func ordenNum(op string, a, b float64) bool {
	switch op {
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	}
	return false
}

// malla son los pares criterio-rango ya resueltos, todos del mismo tamaño.
type malla struct {
	crits      []criterio
	rangos     []Matrix
	filas, col int
}

// parseGrid parsea los pares (rango, criterio) desde `inicio`, de dos en dos. Rangos de
// distinto tamaño → #VALUE!.
func parseGrid(args []*Arg, inicio int) (malla, Error) {
	g := malla{filas: -1, col: -1}
	for i := inicio; i+1 < len(args); i += 2 {
		m := args[i].Matrix()
		r, c := dims(m)
		if g.filas == -1 {
			g.filas, g.col = r, c
		} else if r != g.filas || c != g.col {
			return g, ErrValue
		}
		crit, err := parseCriterion(args[i+1].Value())
		if err != "" {
			return g, err
		}
		g.rangos, g.crits = append(g.rangos, m), append(g.crits, crit)
	}
	return g, ""
}

func (g malla) allMatch(r, c int) bool {
	for i, crit := range g.crits {
		if !matches(en(g.rangos[i], r, c, ""), crit) {
			return false
		}
	}
	return true
}

// contarSi es COUNTIF y COUNTIFS.
func contarSi(aridad func(n int) bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if !aridad(len(args)) {
			return ErrValue
		}
		g, err := parseGrid(args, 0)
		if err != "" {
			return err
		}
		n := 0.0
		for r := 0; r < g.filas; r++ {
			for c := 0; c < g.col; c++ {
				if g.allMatch(r, c) {
					n++
				}
			}
		}
		return n
	}
}

// alignRange alinea el rango de valores al tamaño del de criterios, anclando en su esquina
// superior izquierda (rareza de SUMIF y AVERAGEIF). Si no coincide y no es una referencia
// literal, #VALUE!.
func alignRange(c *FnCtx, a *Arg, filas, cols int) (Matrix, Error) {
	m := a.Matrix()
	if r, k := dims(m); r == filas && k == cols {
		return m, ""
	}
	ref := a.Ref()
	if ref == nil {
		return nil, ErrValue
	}
	return c.Resolve(RefRange{ref.Sheet, ref.C1, ref.R1, ref.C1 + cols - 1, ref.R1 + filas - 1}), ""
}

// agregado acumula los números que cumplen y remata con `fin`.
type agregado struct {
	suma, n, mejor float64
	minimo, visto  bool
}

func (a *agregado) toma(v float64) {
	a.suma += v
	a.n++
	switch {
	case !a.visto:
		a.mejor, a.visto = v, true
	case a.minimo:
		a.mejor = math.Min(a.mejor, v)
	default:
		a.mejor = math.Max(a.mejor, v)
	}
}

// recorrer aplica `cumple` a cada posición y acumula el valor numérico correspondiente.
func recorrer(valores Matrix, filas, cols int, cumple func(r, c int) bool, a *agregado) Error {
	for r := 0; r < filas; r++ {
		for c := 0; c < cols; c++ {
			if !cumple(r, c) {
				continue
			}
			switch x := en(valores, r, c, "").(type) {
			case Error:
				return x
			case float64:
				a.toma(x)
			}
		}
	}
	return ""
}

// siUno es SUMIF y AVERAGEIF: un solo criterio y rango de valores opcional.
func siUno(fin func(a *agregado) Value) fn {
	return func(ctx *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		rango := args[0].Matrix()
		crit, err := parseCriterion(args[1].Value())
		if err != "" {
			return err
		}
		filas, cols := dims(rango)
		deValores := args[0]
		if len(args) == 3 {
			deValores = args[2]
		}
		valores, err := alignRange(ctx, deValores, filas, cols)
		if err != "" {
			return err
		}
		a := &agregado{}
		if err := recorrer(valores, filas, cols, func(r, c int) bool { return matches(en(rango, r, c, ""), crit) }, a); err != "" {
			return err
		}
		return fin(a)
	}
}

// siVarios es SUMIFS, AVERAGEIFS, MINIFS y MAXIFS: el rango de valores va PRIMERO.
func siVarios(minimo bool, fin func(a *agregado) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args)%2 != 1 {
			return ErrValue
		}
		valores := args[0].Matrix()
		g, err := parseGrid(args, 1)
		if err != "" {
			return err
		}
		if r, c := dims(valores); r != g.filas || c != g.col {
			return ErrValue
		}
		a := &agregado{minimo: minimo}
		if err := recorrer(valores, g.filas, g.col, g.allMatch, a); err != "" {
			return err
		}
		return fin(a)
	}
}

func finSuma(a *agregado) Value { return a.suma }

func finMedia(a *agregado) Value {
	if a.n == 0 {
		return ErrDiv0
	}
	return a.suma / a.n
}

// finMejor: sin coincidencias numéricas → 0, como Excel.
func finMejor(a *agregado) Value { return a.mejor }

func init() {
	registrar(map[string]fn{
		"COUNTIF":    contarSi(func(n int) bool { return n == 2 }),
		"COUNTIFS":   contarSi(func(n int) bool { return n >= 2 && n%2 == 0 }),
		"SUMIF":      siUno(finSuma),
		"AVERAGEIF":  siUno(finMedia),
		"SUMIFS":     siVarios(false, finSuma),
		"AVERAGEIFS": siVarios(false, finMedia),
		"MINIFS":     siVarios(true, finMejor),
		"MAXIFS":     siVarios(false, finMejor),
	})
}

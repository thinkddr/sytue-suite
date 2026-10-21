// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Funciones matemáticas básicas (functions/math.ts).

// numArg coacciona el argumento i a número; #VALUE! si el argumento no existe.
func numArg(args []*Arg, i int) (float64, Error) {
	if i >= len(args) {
		return 0, ErrValue
	}
	return CoerceNumber(args[i].Value())
}

// numArgs coacciona los n primeros argumentos EN ORDEN y corta en el primer error, que es
// lo que hace el original con un `if (isError(x)) return x` tras cada uno.
func numArgs(args []*Arg, n int) ([]float64, Error) {
	out := make([]float64, n)
	for i := range out {
		v, err := numArg(args, i)
		if err != "" {
			return nil, err
		}
		out[i] = v
	}
	return out, ""
}

// numOpc es un argumento numérico opcional: def si no está.
func numOpc(args []*Arg, i int, def float64) (float64, Error) {
	if i >= len(args) {
		return def, ""
	}
	return numArg(args, i)
}

// con1 envuelve una función de un número: coacciona, propaga el error y aplica f.
func con1(f func(x float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		return f(x)
	}
}

// con2 es con1 para dos números.
func con2(f func(a, b float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		return f(v[0], v[1])
	}
}

// eachNumber recorre los números de los argumentos con el criterio de Excel: el texto y los
// blancos de un rango (o de lo que devuelve una función) se ignoran, un booleano cuenta solo
// como argumento escalar directo, un texto ESCRITO como argumento («2») se convierte a número
// (#VALUE! si no lo es), y el primer #error corta.
func eachNumber(args []*Arg, cb func(n float64)) Error {
	for _, a := range args {
		esRango, literal := a.Ref() != nil, a.Literal()
		for _, fila := range a.Matrix() {
			for _, v := range fila {
				switch x := v.(type) {
				case Error:
					return x
				case float64:
					cb(x)
				case bool:
					if !esRango {
						cb(b2f(x))
					}
				case string:
					if !literal {
						continue
					}
					n, err := numeroDeLiteral(x)
					if err != "" {
						return err
					}
					cb(n)
				}
			}
		}
	}
	return ""
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// eachCellNum recorre todos los valores COACCIONÁNDOLOS (blanco ignorado, texto numérico
// convertido, otro texto → #VALUE!). Para GCD y LCM. cb puede abortar con un error.
func eachCellNum(args []*Arg, cb func(n float64) Error) Error {
	for _, a := range args {
		for _, fila := range a.Matrix() {
			for _, v := range fila {
				if v == "" {
					continue
				}
				n, err := CoerceNumber(v)
				if err != "" {
					return err
				}
				if err := cb(n); err != "" {
					return err
				}
			}
		}
	}
	return ""
}

const (
	modoRedondear = iota
	modoArriba
	modoAbajo
)

// scaled redondea a `digits` decimales limpiando el ruido binario: escala, limpia a 15
// cifras (lo que arregla ROUND(2.15;1)=2.2) y aplica el modo sobre la MAGNITUD.
func scaled(x, digits float64, modo int) float64 {
	if x == 0 {
		return 0
	}
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	signo := 1.0
	if x < 0 {
		signo = -1
	}
	p := jsPow(10, digits)
	s := prec15(math.Abs(x) * p)
	var r float64
	switch modo {
	case modoArriba:
		r = math.Ceil(s)
	case modoAbajo:
		r = math.Floor(s)
	default:
		r = jsRound(s)
	}
	return signo * prec15(r/p)
}

// toMultiple redondea al múltiplo de s limpiando el ruido binario.
func toMultiple(x, s float64, techo bool) float64 {
	q := prec15(x / s)
	if techo {
		q = math.Ceil(q)
	} else {
		q = math.Floor(q)
	}
	return prec15(s * q)
}

// preciseRound es CEILING.PRECISE / ISO.CEILING / FLOOR.PRECISE: respecto a la recta, con
// el signo del múltiplo ignorado. Múltiplo 0 → 0.
func preciseRound(techo bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		s, err := numOpc(args, 1, 1)
		if err != "" {
			return err
		}
		if s == 0 {
			return 0.0
		}
		return toMultiple(x, math.Abs(s), techo)
	}
}

// mathRound es CEILING.MATH / FLOOR.MATH: como el anterior más el `modo`, que solo actúa
// sobre negativos (distinto de cero invierte la dirección).
func mathRound(techo bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		s, err := numOpc(args, 1, 1)
		if err != "" {
			return err
		}
		if s == 0 {
			return 0.0
		}
		modo, err := numOpc(args, 2, 0)
		if err != "" {
			return err
		}
		if x < 0 && modo != 0 {
			return toMultiple(x, math.Abs(s), !techo)
		}
		return toMultiple(x, math.Abs(s), techo)
	}
}

func gcd2(a, b float64) float64 {
	a, b = math.Abs(a), math.Abs(b)
	for b != 0 && !math.IsNaN(b) {
		a, b = b, math.Mod(a, b)
	}
	return a
}

// redondeo es ROUND / ROUNDUP / ROUNDDOWN.
func redondeo(modo int) fn {
	return con2(func(n, d float64) Value { return scaled(n, math.Trunc(d), modo) })
}

// clasico es CEILING / FLOOR: múltiplo 0 → `cero`; número positivo con múltiplo
// negativo → #NUM!.
func clasico(techo bool, cero Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		s, err := numOpc(args, 1, 1)
		if err != "" {
			return err
		}
		if s == 0 {
			return cero
		}
		if x > 0 && s < 0 {
			return ErrNum
		}
		return toMultiple(x, s, techo)
	}
}

// funcionesMath es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesMath = map[string]fn{
	"SUM": func(_ *FnCtx, args []*Arg) any {
		acc := 0.0
		if err := eachNumber(args, func(n float64) { acc += n }); err != "" {
			return err
		}
		return acc
	},
	// PERCENTOF(subconjunto, todos): la parte que el subconjunto es del total, como razón
	// (0,25 = 25 %). Es el agregador que se le pasa a GROUPBY y PIVOTBY.
	"PERCENTOF": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		parte, todo := 0.0, 0.0
		if err := eachNumber(args[:1], func(n float64) { parte += n }); err != "" {
			return err
		}
		if err := eachNumber(args[1:], func(n float64) { todo += n }); err != "" {
			return err
		}
		if todo == 0 {
			return ErrDiv0
		}
		return parte / todo
	},
	// Sin ningún número → 0, como una hoja real.
	"PRODUCT": func(_ *FnCtx, args []*Arg) any {
		acc, cuenta := 1.0, 0
		if err := eachNumber(args, func(n float64) { acc *= n; cuenta++ }); err != "" {
			return err
		}
		if cuenta == 0 {
			return 0.0
		}
		return acc
	},
	"SUMSQ": func(_ *FnCtx, args []*Arg) any {
		acc := 0.0
		if err := eachNumber(args, func(n float64) { acc += n * n }); err != "" {
			return err
		}
		return acc
	},
	"SUMPRODUCT": sumproduct,
	"ABS":        con1(func(n float64) Value { return math.Abs(n) }),
	"SIGN": con1(func(n float64) Value {
		switch {
		case n > 0:
			return 1.0
		case n < 0:
			return -1.0
		}
		return 0.0
	}),
	"SQRT": con1(func(n float64) Value {
		if n < 0 {
			return ErrNum
		}
		return math.Sqrt(n)
	}),
	"POWER": con2(func(b, e float64) Value { return finito(jsPow(b, e)) }),
	// Resto con el signo del divisor: n − d·FLOOR(n/d).
	"MOD": con2(func(n, d float64) Value {
		if d == 0 {
			return ErrDiv0
		}
		return n - d*math.Floor(n/d)
	}),
	"QUOTIENT": con2(func(n, d float64) Value {
		if d == 0 {
			return ErrDiv0
		}
		return math.Trunc(n / d)
	}),
	"INT": con1(func(n float64) Value { return math.Floor(n) }),
	"TRUNC": func(_ *FnCtx, args []*Arg) any {
		n, err := numArg(args, 0)
		if err != "" {
			return err
		}
		d, err := numOpc(args, 1, 0)
		if err != "" {
			return err
		}
		return scaled(n, math.Trunc(d), modoAbajo)
	},
	"ROUND":     redondeo(modoRedondear),
	"ROUNDUP":   redondeo(modoArriba),
	"ROUNDDOWN": redondeo(modoAbajo),
	// Al múltiplo más cercano; signos opuestos → #NUM!.
	"MROUND": con2(func(n, m float64) Value {
		if m == 0 || n == 0 {
			return 0.0
		}
		if (n < 0) != (m < 0) {
			return ErrNum
		}
		return m * scaled(n/m, 0, modoRedondear)
	}),
	"CEILING": clasico(true, 0.0),
	// ponytail: FLOOR con múltiplo 0 da #DIV/0! como Excel; Google Sheets daría 0.
	"FLOOR":           clasico(false, ErrDiv0),
	"CEILING.MATH":    mathRound(true),
	"FLOOR.MATH":      mathRound(false),
	"CEILING.PRECISE": preciseRound(true),
	"ISO.CEILING":     preciseRound(true),
	"FLOOR.PRECISE":   preciseRound(false),
	"GCD": func(_ *FnCtx, args []*Arg) any {
		g := 0.0
		err := eachCellNum(args, func(n float64) Error {
			t := math.Trunc(n)
			if t < 0 {
				return ErrNum
			}
			g = gcd2(g, t)
			return ""
		})
		if err != "" {
			return err
		}
		return g
	},
	"LCM": func(_ *FnCtx, args []*Arg) any {
		l, cero := 1.0, false
		err := eachCellNum(args, func(n float64) Error {
			t := math.Trunc(n)
			switch {
			case t < 0:
				return ErrNum
			case t == 0:
				cero = true
			default:
				l = (l / gcd2(l, t)) * t
			}
			return ""
		})
		if err != "" {
			return err
		}
		if cero {
			return 0.0
		}
		return finito(l)
	},
}

func init() { registrar(funcionesMath) }

// sumproduct suma los productos componente a componente de matrices del mismo tamaño.
// Los no-números cuentan 0; tamaños distintos → #VALUE!.
func sumproduct(_ *FnCtx, args []*Arg) any {
	if len(args) == 0 {
		return ErrValue
	}
	mats := make([]Matrix, len(args))
	for i, a := range args {
		mats[i] = a.Matrix()
	}
	filas, cols := dims(mats[0])
	for _, m := range mats {
		if len(m) != filas {
			return ErrValue
		}
		for _, fila := range m {
			if len(fila) != cols {
				return ErrValue
			}
		}
	}
	suma := 0.0
	for r := 0; r < filas; r++ {
		for c := 0; c < cols; c++ {
			prod := 1.0
			for _, m := range mats {
				switch x := m[r][c].(type) {
				case Error:
					return x
				case float64:
					prod *= x
				default:
					prod *= 0 // y no `= 0`: un NaN o un infinito previos siguen siéndolo
				}
			}
			suma += prod
		}
	}
	return suma
}

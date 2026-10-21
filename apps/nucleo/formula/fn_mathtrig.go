// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"math/big"
	"strings"
)

// Trigonometría, exponenciales, logaritmos y combinatoria (functions/mathtrig.ts).

// arg1 coacciona el ÚNICO argumento a número; #VALUE! si la aridad no es 1.
func arg1(args []*Arg) (float64, Error) {
	if len(args) != 1 {
		return 0, ErrValue
	}
	return CoerceNumber(args[0].Value())
}

// map1 aplica una función real de un argumento; lo no finito (fuera de dominio o
// desbordamiento: ASIN(2), LN(0), EXP(1000)) es #NUM!.
func map1(f func(x float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := arg1(args)
		if err != "" {
			return err
		}
		return finito(f(x))
	}
}

// reciproca es 1/f(x) con el polo (f(x) = 0) dando #DIV/0! en vez de infinito: una hoja
// distingue «dividiste por cero» de «te saliste del dominio».
func reciproca(f func(x float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := arg1(args)
		if err != "" {
			return err
		}
		d := f(x)
		if d == 0 {
			return ErrDiv0
		}
		return finito(1 / d)
	}
}

// aParidad redondea al entero de la paridad pedida ALEJÁNDOSE de cero (EVEN y ODD). Se
// trabaja sobre el valor absoluto para que -1.5 y 1.5 no redondeen en sentidos distintos.
func aParidad(par bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		x, err := arg1(args)
		if err != "" {
			return err
		}
		signo, a := 1.0, math.Abs(x)
		if x < 0 {
			signo = -1
		}
		if par {
			return signo * (math.Ceil(a/2) * 2)
		}
		return signo * (math.Ceil((a+1)/2)*2 - 1)
	}
}

// combinaciones es C(n, k) por producto acumulado: no pasa por el factorial, que
// desborda mucho antes.
func combinaciones(n, k float64) float64 {
	if k < 0 || n < 0 || k > n {
		return math.NaN()
	}
	m, r := math.Min(k, n-k), 1.0
	for i := 1.0; i <= m && !math.IsInf(r, 0); i++ { // desbordado ya no vuelve: no hace falta seguir
		r = (r * (n - m + i)) / i
	}
	return jsRound(r)
}

const digitosBase = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// simbolosRomanos y valoresRomanos son los siete símbolos y su valor, de mayor a menor.
var (
	simbolosRomanos = []string{"M", "D", "C", "L", "X", "V", "I"}
	valoresRomanos  = []float64{1000, 500, 100, 50, 10, 5, 1}
)

// aRomano pasa un entero (1–3999) a numeración romana en la forma 0 (clásica) a 4
// (simplificada). Se recorren los cuatro lugares (M, C, X, I): un dígito 4 o 9 se escribe con
// resta —el símbolo del lugar delante del de 5 o 10 veces su valor— y las formas 1 a 4 dejan
// que el que resta baje hasta `forma` símbolos más, mientras lo restado no pase de lo que
// queda: 499 es CDXCIX, LDVLIV, XDIX, VDIV e ID. Es `aRomano` de mathtrig.ts.
func aRomano(n float64, forma int) string {
	resto := n
	var out strings.Builder
	for i := 0; i <= 6; i += 2 {
		v := valoresRomanos[i]
		d := math.Floor(resto / v)
		if i > 0 && math.Mod(d, 5) == 4 {
			mayor := i - 2 // el símbolo de 10·v
			if d == 4 {
				mayor = i - 1 // el de 5·v
			}
			j := i
			for pasos := 0; pasos < forma && j < 6; pasos++ {
				if valoresRomanos[mayor]-valoresRomanos[j+1] > resto {
					break
				}
				j++
			}
			out.WriteString(simbolosRomanos[j] + simbolosRomanos[mayor])
			resto -= valoresRomanos[mayor] - valoresRomanos[j]
			continue
		}
		cuantos := d
		if i > 0 && d >= 5 {
			out.WriteString(simbolosRomanos[i-1])
			cuantos -= 5
		}
		out.WriteString(strings.Repeat(simbolosRomanos[i], int(cuantos)))
		resto -= d * v
	}
	return out.String()
}

var valorRomano = map[rune]float64{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}

// enBase es `N.toString(B).toUpperCase()` para un entero no negativo.
func enBase(n float64, base int) string {
	if base == 10 || !esFinito(n) {
		return strings.ToUpper(jsNumero(n)) // también «NAN» e «INFINITY», que big.Float no admite
	}
	i, _ := new(big.Float).SetFloat64(n).Int(nil)
	return strings.ToUpper(i.Text(base))
}

// dosTruncados lee dos argumentos exactos, los trunca y exige 0 ≤ k ≤ n (COMBIN, PERMUT).
func dosTruncados(args []*Arg) (n, k float64, err Error) {
	if len(args) != 2 {
		return 0, 0, ErrValue
	}
	v, err := numArgs(args, 2)
	if err != "" {
		return 0, 0, err
	}
	n, k = math.Trunc(v[0]), math.Trunc(v[1])
	if n < 0 || k < 0 || k > n {
		return 0, 0, ErrNum
	}
	return n, k, ""
}

// funcionesMathTrig es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesMathTrig = map[string]fn{
	"PI": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 0 {
			return ErrValue
		}
		return math.Pi
	},
	"SIN":  map1(math.Sin),
	"COS":  map1(math.Cos),
	"TAN":  map1(math.Tan),
	"ASIN": map1(math.Asin),
	"ACOS": map1(math.Acos),
	"ATAN": map1(math.Atan),
	// ¡Orden de Excel! ATAN2(x; y) = arctan(y/x). En el origen el ángulo no existe.
	"ATAN2": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		if v[0] == 0 && v[1] == 0 {
			return ErrDiv0
		}
		return math.Atan2(v[1], v[0])
	},
	"SINH":    map1(math.Sinh),
	"COSH":    map1(math.Cosh),
	"TANH":    map1(math.Tanh),
	"DEGREES": map1(func(x float64) float64 { return (x * 180) / math.Pi }),
	"RADIANS": map1(func(x float64) float64 { return (x * math.Pi) / 180 }),
	"EXP":     map1(math.Exp),
	"LN":      map1(math.Log),
	"LOG10":   map1(math.Log10),
	"LOG": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		base, err := numOpc(args, 1, 10)
		switch {
		case err != "":
			return err
		case x <= 0 || base <= 0:
			return ErrNum
		case base == 1:
			return ErrDiv0 // ln(1) = 0
		}
		return math.Log(x) / math.Log(base)
	},
	"FACT": func(_ *FnCtx, args []*Arg) any {
		n, err := arg1(args)
		if err != "" {
			return err
		}
		if n < 0 {
			return ErrNum
		}
		r := 1.0
		for i := 2.0; i <= math.Trunc(n) && !math.IsInf(r, 0); i++ {
			r *= i
		}
		return finito(r) // FACT(171) → ∞ → #NUM!
	},
	// FACTDOUBLE(-1) = 1 por convención; por debajo, no.
	"FACTDOUBLE": func(_ *FnCtx, args []*Arg) any {
		n, err := arg1(args)
		if err != "" {
			return err
		}
		if n < -1 {
			return ErrNum
		}
		r := 1.0
		for i := math.Trunc(n); i > 1 && !math.IsInf(r, 0); i -= 2 {
			r *= i
		}
		return finito(r)
	},
	// Forma multiplicativa con el factor menor: C(n,k) = ∏ (n-i)/(i+1).
	"COMBIN": func(_ *FnCtx, args []*Arg) any {
		n, k, err := dosTruncados(args)
		if err != "" {
			return err
		}
		r := 1.0
		for i := 0.0; i < math.Min(k, n-k) && !math.IsInf(r, 0); i++ {
			r = (r * (n - i)) / (i + 1)
		}
		return finito(jsRound(r))
	},
	"PERMUT": func(_ *FnCtx, args []*Arg) any {
		n, k, err := dosTruncados(args)
		if err != "" {
			return err
		}
		r := 1.0
		for i := 0.0; i < k && !math.IsInf(r, 0); i++ {
			r *= n - i
		}
		return finito(r)
	},
	// Volátiles con el azar inyectable. ponytail: el motor cachea el valor; refrescarlo
	// en cada recálculo es cosa del motor (volatilidad), no de la función.
	"RAND": func(c *FnCtx, args []*Arg) any {
		if len(args) != 0 {
			return ErrValue
		}
		return c.Random()
	},
	"RANDBETWEEN": func(c *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		lo, hi := math.Ceil(v[0]), math.Floor(v[1])
		if lo > hi {
			return ErrNum
		}
		return lo + math.Floor(c.Random()*(hi-lo+1))
	},
	"SEC":   map1(func(x float64) float64 { return 1 / math.Cos(x) }),
	"CSC":   reciproca(math.Sin),
	"COT":   reciproca(math.Tan),
	"SECH":  map1(func(x float64) float64 { return 1 / math.Cosh(x) }),
	"CSCH":  reciproca(math.Sinh),
	"COTH":  reciproca(math.Tanh),
	"ASINH": map1(math.Asinh),
	"ACOSH": map1(math.Acosh),
	"ATANH": map1(math.Atanh),
	// NO es atan(1/x): para x negativo daría el cuadrante equivocado. Rango (0, π).
	"ACOT":   map1(func(x float64) float64 { return math.Pi/2 - math.Atan(x) }),
	"ACOTH":  map1(func(x float64) float64 { return math.Log((x+1)/(x-1)) / 2 }),
	"EVEN":   aParidad(true),
	"ODD":    aParidad(false),
	"SQRTPI": map1(func(x float64) float64 { return math.Sqrt(x * math.Pi) }), // negativo → NaN → #NUM!
	// Combinaciones CON repetición: C(n+k−1, k).
	"COMBINA": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		n, k := math.Trunc(v[0]), math.Trunc(v[1])
		switch {
		case n < 0 || k < 0:
			return ErrNum
		case n == 0 && k == 0:
			return 1.0
		}
		return combinaciones(n+k-1, k)
	},
	// (Σxᵢ)! / Πxᵢ!, como producto de binomiales: MULTINOMIAL(20,20,20) desborda el
	// factorial de 60 aunque el resultado quepa de sobra.
	"MULTINOMIAL": func(_ *FnCtx, args []*Arg) any {
		total, acc := 0.0, 1.0
		err := eachNumber(args, func(x float64) {
			n := math.Trunc(x)
			if n < 0 {
				acc = math.NaN()
				return
			}
			total += n
			acc *= combinaciones(total, n)
		})
		if err != "" {
			return err
		}
		return finito(acc)
	},
	// Σ coefᵢ · x^(n + i·m): el ORDEN de los coeficientes define su exponente.
	"SERIESSUM": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 4 {
			return ErrValue
		}
		v, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		acc, i := 0.0, 0.0
		if err := eachNumber(args[3:], func(c float64) { acc += c * jsPow(v[0], v[1]+i*v[2]); i++ }); err != "" {
			return err
		}
		return finito(acc)
	},
	// Entero NO negativo → texto en base 2–36, rellenado con ceros.
	"BASE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		v, err := numArgs(args, 2)
		if err != "" {
			return err
		}
		ancho, err := numOpc(args, 2, 0)
		if err != "" {
			return err
		}
		ancho = math.Trunc(ancho)
		if ancho < 0 || ancho > 255 {
			return ErrNum
		}
		n, b := math.Trunc(v[0]), math.Trunc(v[1])
		if n < 0 || b < 2 || b > 36 || math.IsInf(n, 0) {
			return ErrNum
		}
		return padStart(enBase(n, int(b)), int(ancho))
	},
	// Inversa de BASE. Se valida CADA dígito: parseInt se tragaría «1Z» en base 2.
	"DECIMAL": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		b, err := numArg(args, 1)
		if err != "" {
			return err
		}
		base := math.Trunc(b)
		if base < 2 || base > 36 {
			return ErrNum
		}
		acc := 0.0
		for _, ch := range jsUpper(jsTrim(t)) {
			d := float64(strings.IndexRune(digitosBase, ch))
			if d < 0 || d >= base {
				return ErrNum
			}
			acc = acc*base + d
		}
		return acc
	},
	"ROMAN": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		n, err := numArg(args, 0)
		if err != "" {
			return err
		}
		forma := 0.0
		if len(args) == 2 {
			if forma, err = numArg(args, 1); err != "" {
				return err
			}
			if forma = math.Trunc(forma); forma < 0 || forma > 4 {
				return ErrNum
			}
		}
		resto := math.Trunc(n)
		if resto < 0 || resto > 3999 {
			return ErrNum
		}
		if resto == 0 {
			return ""
		}
		return aRomano(resto, int(forma))
	},
	// Un símbolo menor ANTES de uno mayor resta (IX = 9): de derecha a izquierda,
	// comparando con el máximo visto.
	"ARABIC": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		cuerpo := []rune(jsUpper(jsTrim(t)))
		negativo := len(cuerpo) > 0 && cuerpo[0] == '-'
		if negativo {
			cuerpo = cuerpo[1:]
		}
		acc, mayor := 0.0, 0.0
		for i := len(cuerpo) - 1; i >= 0; i-- {
			v, ok := valorRomano[cuerpo[i]]
			switch {
			case !ok:
				return ErrValue
			case v < mayor:
				acc -= v
			default:
				acc += v
				mayor = v
			}
		}
		if negativo {
			return -acc
		}
		return acc
	},
}

func init() { registrar(funcionesMathTrig) }

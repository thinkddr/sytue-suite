// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"sort"
)

// Estadística descriptiva (functions/stats.ts, primera mitad). Las distribuciones y las
// regresiones van en fn_stats_dist.go y fn_stats_reg.go.

// collectNumbers recoge los números con el criterio de SUM: texto y blancos fuera, un
// booleano solo si es argumento escalar directo, y el primer #error corta.
func collectNumbers(args []*Arg) ([]float64, Error) {
	out := []float64{}
	err := eachNumber(args, func(n float64) { out = append(out, n) })
	return out, err
}

// collectA es el criterio de las «A» (AVERAGEA, MAXA…): los booleanos cuentan siempre y
// el texto no vacío cuenta como 0.
func collectA(args []*Arg) ([]float64, Error) {
	out := []float64{}
	for _, a := range args {
		for _, fila := range a.Matrix() {
			for _, v := range fila {
				switch x := v.(type) {
				case Error:
					return nil, x
				case float64:
					out = append(out, x)
				case bool:
					out = append(out, b2f(x))
				case string:
					if x != "" {
						out = append(out, 0)
					}
				}
			}
		}
	}
	return out, ""
}

func sumSqDev(nums []float64) float64 {
	m, ss := sumar(nums)/float64(len(nums)), 0.0
	for _, v := range nums {
		d := v - m
		ss += d * d
	}
	return ss
}

// ordenados es `[...nums].sort((a, b) => a - b)`: estable, como el de JavaScript.
func ordenados(nums []float64, descendente bool) []float64 {
	s := append([]float64(nil), nums...)
	sort.SliceStable(s, func(i, j int) bool {
		if descendente {
			return s[i] > s[j]
		}
		return s[i] < s[j]
	})
	return s
}

// percentile es el percentil INCLUSIVO por interpolación lineal.
func percentile(nums []float64, p float64) float64 {
	s := ordenados(nums, false)
	rango := p * float64(len(s)-1)
	lo := math.Floor(rango)
	base := s[aInt(lo)]
	sig := base
	if aInt(lo)+1 < len(s) {
		sig = s[aInt(lo)+1]
	}
	return base + (rango-lo)*(sig-base)
}

// percentileExc es el percentil EXCLUSIVO; fuera de [1, n] en posición 1-based es #NUM!.
func percentileExc(nums []float64, p float64) Value {
	s := ordenados(nums, false)
	n := float64(len(s))
	rango := p * (n + 1)
	if rango < 1 || rango > n || math.IsNaN(rango) {
		return ErrNum
	}
	lo := math.Floor(rango)
	base := s[aInt(lo)-1]
	sig := base
	if aInt(lo) < len(s) {
		sig = s[aInt(lo)]
	}
	return base + (rango-lo)*(sig-base)
}

// truncSig trunca a `sig` cifras significativas.
func truncSig(x, sig float64) float64 {
	if x == 0 {
		return 0
	}
	m := jsPow(10, sig-1-math.Floor(math.Log10(math.Abs(x))))
	return math.Trunc(x*m) / m
}

func percentRank(nums []float64, x, sig float64, exclusivo bool) Value {
	s := ordenados(nums, false)
	n := len(s)
	if n == 0 || x < s[0] || x > s[n-1] {
		return ErrNA
	}
	lo := 0
	for lo < n && s[lo] < x {
		lo++ // cuántos valores son estrictamente menores que x
	}
	pos := float64(lo)
	if lo >= n || s[lo] != x {
		if lo == 0 || lo >= n {
			return ErrNA // solo con NaN: ninguna comparación de arriba lo frenó
		}
		pos = float64(lo-1) + (x-s[lo-1])/(s[lo]-s[lo-1])
	}
	switch {
	case exclusivo:
		return truncSig((pos+1)/float64(n+1), sig)
	case n == 1:
		return truncSig(0, sig)
	}
	return truncSig(pos/float64(n-1), sig)
}

func percentRankFn(exclusivo bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		nums, err := collectNumbers(args[:1])
		if err != "" {
			return err
		}
		x, err := numArg(args, 1)
		if err != "" {
			return err
		}
		sig, err := intOpc(args, 2, 3)
		if err != "" {
			return err
		}
		if sig < 1 {
			return ErrNum
		}
		if len(nums) == 0 {
			return ErrNA
		}
		return percentRank(nums, x, sig, exclusivo)
	}
}

// deNumeros envuelve las que solo necesitan la lista de números de TODOS sus argumentos.
func deNumeros(recoge func([]*Arg) ([]float64, Error), f func(nums []float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		nums, err := recoge(args)
		if err != "" {
			return err
		}
		return f(nums)
	}
}

// listaYNumero envuelve las de (rango, k): LARGE, SMALL, PERCENTILE, QUARTILE….
func listaYNumero(f func(nums []float64, k float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 {
			return ErrValue
		}
		nums, err := collectNumbers(args[:1])
		if err != "" {
			return err
		}
		k, err := numArg(args, 1)
		if err != "" {
			return err
		}
		return f(nums, k)
	}
}

// contar es COUNT, COUNTA y COUNTBLANK: no propagan errores, solo cuentan.
func contar(cuenta func(v Value, a *Arg) bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		n := 0.0
		for _, a := range args {
			for _, fila := range a.Matrix() {
				for _, v := range fila {
					if cuenta(v, a) {
						n++
					}
				}
			}
		}
		return n
	}
}

// maximoMinimo es MAX, MAXA, MIN y MINA: sin números, 0.
func maximoMinimo(recoge func([]*Arg) ([]float64, Error), mejor func(v, m float64) bool) fn {
	return deNumeros(recoge, func(nums []float64) Value {
		if len(nums) == 0 {
			return 0.0
		}
		m := nums[0]
		for _, v := range nums {
			if mejor(v, m) {
				m = v
			}
		}
		return m
	})
}

// varianza es VAR, VARP, STDEV y STDEVP con sus alias modernos: la muestral divide por
// n−1 y necesita dos valores; la poblacional divide por n y se conforma con uno.
func varianza(muestral, raiz bool) fn {
	return deNumeros(collectNumbers, func(nums []float64) Value {
		n := float64(len(nums))
		if muestral {
			n--
		}
		if n < 1 {
			return ErrDiv0
		}
		v := sumSqDev(nums) / n
		if raiz {
			return math.Sqrt(v)
		}
		return v
	})
}

// momento es SKEW, SKEW.P y KURT: Σ((v − media)/sd)^k sobre la desviación muestral o
// poblacional, rematado por `fin(n, s)`.
func momento(minimo int, poblacional bool, k float64, fin func(n, s float64) float64) fn {
	return deNumeros(collectNumbers, func(nums []float64) Value {
		n := float64(len(nums))
		if len(nums) < minimo {
			return ErrDiv0
		}
		m, divisor := sumar(nums)/n, n-1
		if poblacional {
			divisor = n
		}
		sd := math.Sqrt(sumSqDev(nums) / divisor)
		if sd == 0 {
			return ErrDiv0
		}
		s := 0.0
		for _, v := range nums {
			s += jsPow((v-m)/sd, k)
		}
		return fin(n, s)
	})
}

// esNumero es lo que COUNT cuenta: un número; un booleano solo si viene suelto y no dentro
// de un rango; y un texto numérico solo si está escrito como argumento (COUNT("1";1) = 2).
func esNumero(v Value, a *Arg) bool {
	switch x := v.(type) {
	case float64:
		return true
	case bool:
		return a.Ref() == nil
	case string:
		if a.Literal() {
			_, err := numeroDeLiteral(x)
			return err == ""
		}
	}
	return false
}

// media es la media aritmética; sin números, #DIV/0! como una hoja de verdad.
func media(nums []float64) Value {
	if len(nums) == 0 {
		return ErrDiv0
	}
	return sumar(nums) / float64(len(nums))
}

// percentil y cuartil comparten cuerpo con PERCENTILE y QUARTILE y sus variantes .INC.
var (
	percentil = listaYNumero(func(nums []float64, p float64) Value {
		if len(nums) == 0 || p < 0 || p > 1 || math.IsNaN(p) {
			return ErrNum
		}
		return percentile(nums, p)
	})
	cuartil = listaYNumero(func(nums []float64, q float64) Value {
		qi := math.Trunc(q)
		if len(nums) == 0 || qi < 0 || qi > 4 || math.IsNaN(qi) {
			return ErrNum
		}
		return percentile(nums, qi/4)
	})
)

// funcionesEstadistica es el registro de este fichero. Es un MAPA y no un init(): el
// cuerpo de cada función vive dentro del literal, y medido como UNA función lo que sale
// es la suma de todos ellos, que no es una medida de nada.
var funcionesEstadistica = map[string]fn{
	"AVERAGE":    deNumeros(collectNumbers, media),
	"AVERAGEA":   deNumeros(collectA, media),
	"COUNT":      contar(esNumero),
	"COUNTA":     contar(func(v Value, _ *Arg) bool { return v != "" }),
	"COUNTBLANK": contar(func(v Value, _ *Arg) bool { return v == "" }),
	"MAX":        maximoMinimo(collectNumbers, func(v, m float64) bool { return v > m }),
	"MAXA":       maximoMinimo(collectA, func(v, m float64) bool { return v > m }),
	"MIN":        maximoMinimo(collectNumbers, func(v, m float64) bool { return v < m }),
	"MINA":       maximoMinimo(collectA, func(v, m float64) bool { return v < m }),
	"MEDIAN": deNumeros(collectNumbers, func(nums []float64) Value {
		n := len(nums)
		if n == 0 {
			return ErrNum
		}
		s := ordenados(nums, false)
		if n%2 == 1 {
			return s[n/2]
		}
		return (s[n/2-1] + s[n/2]) / 2
	}),
	// La moda es el primer valor EN ORDEN DE APARICIÓN que alcanza la cuenta máxima.
	"MODE":      deNumeros(collectNumbers, moda),
	"MODE.SNGL": deNumeros(collectNumbers, moda),
	"STDEV":     varianza(true, true),
	"STDEV.S":   varianza(true, true),
	"STDEVP":    varianza(false, true),
	"STDEV.P":   varianza(false, true),
	"VAR":       varianza(true, false),
	"VAR.S":     varianza(true, false),
	"VARP":      varianza(false, false),
	"VAR.P":     varianza(false, false),
	"LARGE": listaYNumero(func(nums []float64, k float64) Value {
		if r := math.Trunc(k); r >= 1 && r <= float64(len(nums)) {
			return ordenados(nums, true)[int(r)-1]
		}
		return ErrNum
	}),
	"SMALL": listaYNumero(func(nums []float64, k float64) Value {
		if r := math.Trunc(k); r >= 1 && r <= float64(len(nums)) {
			return ordenados(nums, false)[int(r)-1]
		}
		return ErrNum
	}),
	"RANK":            rank(false),
	"RANK.EQ":         rank(false),
	"RANK.AVG":        rank(true),
	"PERCENTRANK":     percentRankFn(false),
	"PERCENTRANK.INC": percentRankFn(false),
	"PERCENTRANK.EXC": percentRankFn(true),
	"PERCENTILE":      percentil,
	"PERCENTILE.INC":  percentil,
	"QUARTILE":        cuartil,
	"QUARTILE.INC":    cuartil,
	"GEOMEAN": deNumeros(collectNumbers, func(nums []float64) Value {
		if len(nums) == 0 {
			return ErrNum
		}
		prod := 1.0
		for _, v := range nums {
			if v <= 0 {
				return ErrNum
			}
			prod *= v // ponytail (del original): producto directo; exp(Σln)/n evitaría el desbordamiento
		}
		return jsPow(prod, 1/float64(len(nums)))
	}),
	"HARMEAN": deNumeros(collectNumbers, func(nums []float64) Value {
		if len(nums) == 0 {
			return ErrNum
		}
		acc := 0.0
		for _, v := range nums {
			if v <= 0 {
				return ErrNum
			}
			acc += 1 / v
		}
		return float64(len(nums)) / acc
	}),
	"DEVSQ": deNumeros(collectNumbers, func(nums []float64) Value {
		if len(nums) == 0 {
			return 0.0
		}
		return sumSqDev(nums)
	}),
	"AVEDEV": deNumeros(collectNumbers, func(nums []float64) Value {
		n := float64(len(nums))
		if n == 0 {
			return ErrNum
		}
		m, s := sumar(nums)/n, 0.0
		for _, v := range nums {
			s += math.Abs(v - m)
		}
		return s / n
	}),
	"SKEW":   momento(3, false, 3, func(n, s float64) float64 { return (n / ((n - 1) * (n - 2))) * s }),
	"SKEW.P": momento(1, true, 3, func(n, s float64) float64 { return s / n }),
	"KURT": momento(4, false, 4, func(n, s float64) float64 {
		return (n*(n+1)*s)/((n-1)*(n-2)*(n-3)) - (3*jsPow(n-1, 2))/((n-2)*(n-3))
	}),
}

func init() { registrar(funcionesEstadistica) }

func moda(nums []float64) Value {
	cuentas := map[float64]int{}
	mejor, mejorCuenta := 0.0, 0
	for _, v := range nums {
		cuentas[v]++
	}
	for _, v := range nums { // en orden de aparición, como el Map del original
		if c := cuentas[v]; c > mejorCuenta {
			mejor, mejorCuenta = v, c
		}
	}
	if mejorCuenta < 2 {
		return ErrNA
	}
	return mejor
}

// rank es RANK y RANK.EQ (los empates comparten puesto) y RANK.AVG (puesto medio).
func rank(promedio bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 {
			return ErrValue
		}
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		nums, err := collectNumbers(args[1:2])
		if err != "" {
			return err
		}
		orden, err := numOpc(args, 2, 0)
		if err != "" {
			return err
		}
		delante, iguales := 0.0, 0.0
		for _, v := range nums {
			switch {
			case v == x:
				iguales++
			case (orden != 0 && v < x) || (orden == 0 && v > x):
				delante++
			}
		}
		if iguales == 0 {
			return ErrNA
		}
		if promedio {
			return delante + (iguales+1)/2
		}
		return delante + 1
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Regresión, correlación y contrastes de hipótesis (functions/stats.ts, tercera parte).

// collectPairs son los pares (y, x) alineados por posición: longitudes distintas → #N/A, y
// se descarta el par si alguna de las dos celdas no es número.
func collectPairs(aY, aX *Arg) (ys, xs []float64, err Error) { return pairedNums(aY, aX) }

// numerosDe son TODAS las celdas no vacías de un argumento coaccionadas a número.
func numerosDe(a *Arg) ([]float64, Error) {
	out := []float64{}
	for _, fila := range a.Matrix() {
		for _, v := range fila {
			if v == "" {
				continue
			}
			n, err := CoerceNumber(v)
			if err != "" {
				return nil, err
			}
			out = append(out, n)
		}
	}
	return out, ""
}

type regresion struct{ n, mx, my, sxx, syy, sxy float64 }

func regress(ys, xs []float64) regresion {
	r := regresion{n: float64(len(xs))}
	r.mx, r.my = sumar(xs)/r.n, sumar(ys)/r.n
	for i := range xs {
		dx, dy := xs[i]-r.mx, ys[i]-r.my
		r.sxx += dx * dx
		r.syy += dy * dy
		r.sxy += dx * dy
	}
	return r
}

// dePares envuelve las de dos rangos emparejados (CORREL, SLOPE…): aridad 2 y al menos
// `minimo` pares, o #DIV/0!.
func dePares(minimo int, f func(r regresion, n float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		ys, xs, err := collectPairs(args[0], args[1])
		if err != "" {
			return err
		}
		if len(xs) < minimo {
			return ErrDiv0
		}
		return f(regress(ys, xs), float64(len(xs)))
	}
}

// secuencia es 1, 2, …, n: las x por defecto de LINEST, TREND y compañía.
func secuencia(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i + 1)
	}
	return out
}

// pareja lee (conocido_y, [conocido_x]) de LINEST, LOGEST y GROWTH.
func pareja(args []*Arg) (ys, xs []float64, err Error) {
	if ys, err = numerosDe(args[0]); err != "" {
		return nil, nil, err
	}
	if len(args) < 2 {
		return ys, secuencia(len(ys)), ""
	}
	if xs, err = numerosDe(args[1]); err != "" {
		return nil, nil, err
	}
	switch {
	case len(xs) == 0:
		return ys, secuencia(len(ys)), ""
	case len(xs) != len(ys):
		return nil, nil, ErrRef
	}
	return ys, xs, ""
}

// optBool es un booleano opcional donde el blanco también es «por defecto».
func optBool(args []*Arg, i int, porDefecto bool) (bool, Error) {
	if i >= len(args) {
		return porDefecto, ""
	}
	v := args[i].Value()
	if v == "" {
		return porDefecto, ""
	}
	return CoerceBoolean(v)
}

// rectaMinimos ajusta y = m·x + b por mínimos cuadrados; sin constante, b = 0. Falso si
// las x no tienen dispersión.
func rectaMinimos(ys, xs []float64, conConstante bool) (m, b float64, ok bool) {
	if !conConstante {
		sxy, sxx := 0.0, 0.0
		for i := range xs {
			sxy += xs[i] * ys[i]
			sxx += xs[i] * xs[i]
		}
		return sxy / sxx, 0, sxx != 0
	}
	r := regress(ys, xs)
	m = r.sxy / r.sxx
	return m, r.my - m*r.mx, r.sxx != 0
}

// logaritmos de las y de LOGEST y GROWTH: una y no positiva es #NUM!.
func logaritmos(ys []float64) ([]float64, Error) {
	logs := make([]float64, len(ys))
	for i, y := range ys {
		if y <= 0 {
			return nil, ErrNum
		}
		logs[i] = math.Log(y)
	}
	return logs, ""
}

// ajuste lee los argumentos comunes de LINEST, LOGEST y GROWTH y devuelve la recta.
func ajuste(args []*Arg, iConstante int, logaritmico bool) (m, b float64, xs []float64, err Error) {
	if len(args) < 1 || len(args) > 4 {
		return 0, 0, nil, ErrValue
	}
	ys, xs, err := pareja(args)
	if err != "" {
		return 0, 0, nil, err
	}
	if len(ys) < 2 {
		return 0, 0, nil, ErrDiv0
	}
	conConstante, err := optBool(args, iConstante, true)
	if err != "" {
		return 0, 0, nil, err
	}
	if logaritmico {
		if ys, err = logaritmos(ys); err != "" {
			return 0, 0, nil, err
		}
	}
	m, b, ok := rectaMinimos(ys, xs, conConstante)
	if !ok {
		return 0, 0, nil, ErrDiv0
	}
	return m, b, xs, ""
}

func enColumna(vals []Value) Matrix {
	out := make(Matrix, len(vals))
	for i, v := range vals {
		out[i] = []Value{v}
	}
	return out
}

// trend es TREND(conocido_y, [conocido_x], [nuevas_x], [constante]).
func trend(_ *FnCtx, args []*Arg) any {
	if len(args) < 1 || len(args) > 4 {
		return ErrValue
	}
	ys, err := numerosDe(args[0])
	if err != "" {
		return err
	}
	if len(ys) == 0 {
		return ErrValue
	}
	xs := secuencia(len(ys))
	if len(args) >= 2 {
		if xs, err = numerosDe(args[1]); err != "" {
			return err
		}
	}
	if len(xs) != len(ys) {
		return ErrRef
	}
	conConstante, err := boolOpc(args, 3, true)
	if err != "" {
		return err
	}
	m, b, ok := rectaMinimos(ys, xs, conConstante)
	if !ok {
		return ErrDiv0
	}
	if len(args) < 3 {
		out := make([]Value, len(xs))
		for i, x := range xs {
			out[i] = finito(b + m*x)
		}
		return enColumna(out)
	}
	nuevas := args[2].Matrix()
	out := make(Matrix, len(nuevas))
	for r, fila := range nuevas {
		out[r] = make([]Value, len(fila))
		for c, v := range fila {
			if x, err := CoerceNumber(v); err != "" {
				out[r][c] = err
			} else {
				out[r][c] = finito(b + m*x)
			}
		}
	}
	return out
}

// tTest es T.TEST(x, y, colas, tipo): 1 pareado, 2 varianzas iguales, 3 de Welch.
func tTest(_ *FnCtx, args []*Arg) any {
	if len(args) != 4 {
		return ErrValue
	}
	xs, err := numerosDe(args[0])
	if err != "" {
		return err
	}
	ys, err := numerosDe(args[1])
	if err != "" {
		return err
	}
	colas, err := intOpc(args, 2, 0)
	if err != "" {
		return err
	}
	tipo, err := intOpc(args, 3, 0)
	if err != "" {
		return err
	}
	if (colas != 1 && colas != 2) || tipo < 1 || tipo > 3 {
		return ErrNum
	}
	var stat, df float64
	n1, n2 := float64(len(xs)), float64(len(ys))
	switch {
	case tipo == 1:
		if len(xs) != len(ys) {
			return ErrNA
		}
		d := make([]float64, len(xs))
		for i := range xs {
			d[i] = xs[i] - ys[i]
		}
		if n1 < 2 {
			return ErrDiv0
		}
		sd := math.Sqrt(sumSqDev(d) / (n1 - 1))
		if sd == 0 {
			return ErrDiv0
		}
		stat, df = (sumar(d)/n1)/(sd/math.Sqrt(n1)), n1-1
	case n1 < 2 || n2 < 2:
		return ErrDiv0
	default:
		m1, m2 := sumar(xs)/n1, sumar(ys)/n2
		v1, v2 := sumSqDev(xs)/(n1-1), sumSqDev(ys)/(n2-1)
		if tipo == 2 {
			vp := ((n1-1)*v1 + (n2-1)*v2) / (n1 + n2 - 2)
			if vp == 0 {
				return ErrDiv0
			}
			stat, df = (m1-m2)/math.Sqrt(vp*(1/n1+1/n2)), n1+n2-2
		} else {
			a, b := v1/n1, v2/n2
			if a+b == 0 {
				return ErrDiv0
			}
			stat, df = (m1-m2)/math.Sqrt(a+b), ((a+b)*(a+b))/((a*a)/(n1-1)+(b*b)/(n2-1))
		}
	}
	unaCola := 1 - tcdf(math.Abs(stat), df)
	return finito(colas * unaCola)
}

// chisqTest es CHISQ.TEST(observado, esperado).
func chisqTest(_ *FnCtx, args []*Arg) any {
	if len(args) != 2 {
		return ErrValue
	}
	obs, esp := args[0].Matrix(), args[1].Matrix()
	filas, cols := dims(obs)
	if er, ec := dims(esp); filas == 0 || cols == 0 || er != filas || ec != cols {
		return ErrNA
	}
	chi := 0.0
	for i := 0; i < filas; i++ {
		for j := 0; j < cols; j++ {
			if j >= len(obs[i]) || j >= len(esp[i]) {
				continue
			}
			if e, ok := obs[i][j].(Error); ok {
				return e
			}
			if e, ok := esp[i][j].(Error); ok {
				return e
			}
			o, okO := obs[i][j].(float64)
			e, okE := esp[i][j].(float64)
			if !okO || !okE {
				continue
			}
			if e == 0 {
				return ErrDiv0 // dividir por una frecuencia esperada nula
			}
			chi += ((o - e) * (o - e)) / e
		}
	}
	df := float64((filas - 1) * (cols - 1))
	if filas == 1 || cols == 1 {
		df = float64(filas*cols - 1)
	}
	if df < 1 {
		return ErrNA
	}
	return finito(1 - gammap(df/2, chi/2))
}

// funcionesRegresiones es el registro, como un MAPA y no como una función, por lo mismo
// que los de los demás ficheros: su complejidad medida sería la suma de los
// cuerpos que lleva dentro.
var funcionesRegresiones = map[string]fn{
	"CORREL": dePares(1, func(r regresion, _ float64) Value {
		if d := math.Sqrt(r.sxx * r.syy); d != 0 {
			return r.sxy / d
		}
		return ErrDiv0
	}),
	"COVARIANCE.P": dePares(1, func(r regresion, n float64) Value { return r.sxy / n }),
	"COVARIANCE.S": dePares(2, func(r regresion, n float64) Value { return r.sxy / (n - 1) }),
	"SLOPE": dePares(1, func(r regresion, _ float64) Value {
		if r.sxx == 0 {
			return ErrDiv0
		}
		return r.sxy / r.sxx
	}),
	"INTERCEPT": dePares(1, func(r regresion, _ float64) Value {
		if r.sxx == 0 {
			return ErrDiv0
		}
		return r.my - (r.sxy/r.sxx)*r.mx
	}),
	"RSQ": dePares(1, func(r regresion, _ float64) Value {
		d := r.sxx * r.syy
		if d == 0 {
			return ErrDiv0
		}
		corr := r.sxy / math.Sqrt(d)
		return corr * corr
	}),
	// Error típico de la predicción de y: hacen falta al menos tres pares.
	"STEYX": dePares(3, func(r regresion, n float64) Value {
		if r.sxx == 0 {
			return ErrDiv0
		}
		return finito(math.Sqrt(math.Max(0, (r.syy-(r.sxy*r.sxy)/r.sxx)/(n-2))))
	}),
	"FORECAST.LINEAR": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 3 {
			return ErrValue
		}
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		ys, xs, err := collectPairs(args[1], args[2])
		if err != "" {
			return err
		}
		if len(xs) < 1 {
			return ErrDiv0
		}
		r := regress(ys, xs)
		if r.sxx == 0 {
			return ErrDiv0
		}
		pendiente := r.sxy / r.sxx
		return finito(r.my - pendiente*r.mx + pendiente*x)
	},
	"TREND": trend,
	"PERCENTILE.EXC": listaYNumero(func(nums []float64, p float64) Value {
		if len(nums) == 0 {
			return ErrNum
		}
		return percentileExc(nums, p)
	}),
	"QUARTILE.EXC": listaYNumero(func(nums []float64, q float64) Value {
		qi := math.Trunc(q)
		if len(nums) == 0 || qi < 1 || qi > 3 || math.IsNaN(qi) {
			return ErrNum
		}
		return percentileExc(nums, qi/4)
	}),
	// Media tras descartar la fracción `pct` de los datos, repartida entre los dos extremos.
	"TRIMMEAN": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		nums, err := collectNumbers(args[:1])
		if err != "" {
			return err
		}
		pct, err := numArg(args, 1)
		if err != "" {
			return err
		}
		n := len(nums)
		if !(pct >= 0 && pct < 1) || n == 0 {
			return ErrNum
		}
		k := int(math.Floor((float64(n) * pct) / 2))
		resto := ordenados(nums, false)[k : n-k]
		if len(resto) == 0 {
			return ErrNum
		}
		return sumar(resto) / float64(len(resto))
	},
	// Todas las modas, en columna y en orden de aparición.
	"MODE.MULT": deNumeros(collectNumbers, func(nums []float64) Value {
		cuentas, mejor := map[float64]int{}, 0
		for _, v := range nums {
			cuentas[v]++
			mejor = max(mejor, cuentas[v])
		}
		if mejor < 2 {
			return ErrNA
		}
		var modas []Value
		for _, v := range nums {
			if cuentas[v] == mejor {
				modas = append(modas, v)
				cuentas[v] = 0 // ya apuntada
			}
		}
		return enColumna(modas)
	}),
	"STDEVA": deNumeros(collectA, func(nums []float64) Value {
		if len(nums) < 2 {
			return ErrDiv0
		}
		return finito(math.Sqrt(sumSqDev(nums) / float64(len(nums)-1)))
	}),
	"VARA": deNumeros(collectA, func(nums []float64) Value {
		if len(nums) < 2 {
			return ErrDiv0
		}
		return finito(sumSqDev(nums) / float64(len(nums)-1))
	}),
	// Z.TEST(datos, mu, [sigma]): una cola; sin sigma se usa la desviación muestral.
	"Z.TEST": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		nums, err := collectNumbers(args[:1])
		if err != "" {
			return err
		}
		n := float64(len(nums))
		if n < 1 {
			return ErrNA
		}
		mu, err := numArg(args, 1)
		if err != "" {
			return err
		}
		var sd float64
		switch {
		case len(args) == 3:
			if sd, err = numArg(args, 2); err != "" {
				return err
			}
		case n < 2:
			return ErrDiv0
		default:
			sd = math.Sqrt(sumSqDev(nums) / (n - 1))
		}
		if sd <= 0 {
			return ErrNum
		}
		return finito(1 - normCdf((sumar(nums)/n-mu)/(sd/math.Sqrt(n))))
	},
	"PROB":   prob,
	"T.TEST": tTest,
	"F.TEST": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		xs, err := numerosDe(args[0])
		if err != "" {
			return err
		}
		ys, err := numerosDe(args[1])
		if err != "" {
			return err
		}
		n1, n2 := float64(len(xs)), float64(len(ys))
		if n1 < 2 || n2 < 2 {
			return ErrDiv0
		}
		v1, v2 := sumSqDev(xs)/(n1-1), sumSqDev(ys)/(n2-1)
		if v1 == 0 || v2 == 0 {
			return ErrDiv0
		}
		if v1 < v2 {
			v1, v2, n1, n2 = v2, v1, n2, n1 // la varianza mayor va en el numerador
		}
		return finito(2 * (1 - fcdf(v1/v2, n1-1, n2-1)))
	},
	"CHISQ.TEST": chisqTest,
	// FREQUENCY(datos, límites): una cuenta por tramo y una más para lo que supera al último.
	"FREQUENCY": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		datos, err := numerosDe(args[0])
		if err != "" {
			return err
		}
		limites, err := numerosDe(args[1])
		if err != "" {
			return err
		}
		orden := ordenados(limites, false)
		cuentas := make([]Value, len(orden)+1)
		for i := range cuentas {
			cuentas[i] = 0.0
		}
		for _, v := range datos {
			i := 0
			for i < len(orden) && v > orden[i] {
				i++
			}
			cuentas[i] = cuentas[i].(float64) + 1 //nolint:errcheck,forcetypeassert // se acaba de llenar de float64
		}
		return enColumna(cuentas)
	},
	"LINEST": func(_ *FnCtx, args []*Arg) any {
		m, b, _, err := ajuste(args, 2, false)
		if err != "" {
			return err
		}
		return Matrix{{m, b}}
	},
	"LOGEST": func(_ *FnCtx, args []*Arg) any {
		m, b, _, err := ajuste(args, 2, true)
		if err != "" {
			return err
		}
		return Matrix{{math.Exp(m), math.Exp(b)}}
	},
	"GROWTH": func(_ *FnCtx, args []*Arg) any {
		m, b, xs, err := ajuste(args, 3, true)
		if err != "" {
			return err
		}
		if len(args) >= 3 {
			nuevas, err := numerosDe(args[2])
			if err != "" {
				return err
			}
			if len(nuevas) > 0 {
				xs = nuevas
			}
		}
		out := make([]Value, len(xs))
		for i, x := range xs {
			out[i] = finito(math.Exp(b + m*x))
		}
		return enColumna(out)
	},
}

// prob es PROB(x, probabilidades, inferior, [superior]).
func prob(_ *FnCtx, args []*Arg) any {
	if len(args) < 3 || len(args) > 4 {
		return ErrValue
	}
	xs, err := numerosDe(args[0])
	if err != "" {
		return err
	}
	ps, err := numerosDe(args[1])
	if err != "" {
		return err
	}
	if len(xs) != len(ps) || len(xs) == 0 {
		return ErrNA
	}
	total := 0.0
	for _, q := range ps {
		if q < 0 || q > 1 {
			return ErrNum
		}
		total += q
	}
	if math.Abs(total-1) > 1e-9 {
		return ErrNum
	}
	lo, err := numArg(args, 2)
	if err != "" {
		return err
	}
	hi, err := numOpc(args, 3, lo)
	if err != "" {
		return err
	}
	acc := 0.0
	for i, x := range xs {
		if x >= lo && x <= hi {
			acc += ps[i]
		}
	}
	return finito(acc)
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Cupones irregulares (ODDFPRICE, ODDFYIELD, ODDLPRICE, ODDLYIELD). Espejo de
// packages/formula/src/functions/financial.ts, donde está explicado: un periodo irregular se
// mide en «cuasi-cupones» (DCi/NLi de ECMA-376) prolongando el calendario regular hacia atrás
// desde el primer cupón (ODDF) o hacia delante desde el último interés (ODDL).

// largoCupon es la longitud de un periodo de cupón [a, b]: solo la base 1 la cuenta de verdad.
func largoCupon(a, b, freq, basis float64) float64 {
	if basis == 1 {
		return b - a
	}
	if basis == 3 {
		return 365 / freq
	}
	return 360 / freq
}

// diasBase son los días entre dos fechas por la base (ya validada: no puede fallar).
func diasBase(a, b, basis float64) float64 {
	d, _ := dayCount(a, b, basis)
	return d
}

type oddF struct{ fc, acc, tfc, m float64 }

type oddArgs struct{ rate, x, red, freq float64 }

// oddFlujo son los argumentos de ODDF*: (liquidación, vencimiento, emisión, primer cupón, tasa,
// rend/precio, rescate, frecuencia, [base]); #NUM! si las fechas no cumplen vencimiento >
// primer cupón > liquidación > emisión o el primer cupón no está en el calendario del vencimiento.
func oddFlujo(a []float64) (oddF, oddArgs, Error) {
	settle, maturity, issue, first := math.Floor(a[0]), math.Floor(a[1]), math.Floor(a[2]), math.Floor(a[3])
	rate, x, red := a[4], a[5], a[6]
	freq, basis := math.Trunc(a[7]), math.Trunc(a[8])
	if !(maturity > first && first > settle && settle > issue) || issue < 0 {
		return oddF{}, oddArgs{}, ErrNum
	}
	if rate < 0 || x < 0 || red <= 0 || !validFreq(freq) || basis < 0 || basis > 4 {
		return oddF{}, oddArgs{}, ErrNum
	}
	m := -1.0
	for k := 0.0; k <= 12_000; k++ {
		d := coupDate(maturity, k, freq)
		if d == first {
			m = k
			break
		}
		if d < first {
			break
		}
	}
	if m < 0 {
		return oddF{}, oddArgs{}, ErrNum
	}
	q := []float64{first}
	for k := 1.0; q[0] > issue; k++ {
		if k > 12_000 {
			return oddF{}, oddArgs{}, ErrNum
		}
		q = append([]float64{coupDate(first, k, freq)}, q...)
	}
	var fc, acc, tfc float64
	for i := 1; i < len(q); i++ {
		lo, hi := q[i-1], q[i]
		nl := largoCupon(lo, hi, freq, basis)
		desde := math.Max(lo, issue)
		fc += diasBase(desde, hi, basis) / nl
		if hasta := math.Min(hi, settle); hasta > desde {
			acc += diasBase(desde, hasta, basis) / nl
		}
		if s0 := math.Max(lo, settle); hi > s0 {
			tfc += diasBase(s0, hi, basis) / nl
		}
	}
	return oddF{fc, acc, tfc, m}, oddArgs{rate, x, red, freq}, ""
}

func (f oddF) precio(rate, yld, red, freq float64) float64 {
	c := (100 * rate) / freq
	v := 1 + yld/freq
	pv := (c * f.fc) / math.Pow(v, f.tfc)
	for j := 1.0; j <= f.m; j++ {
		pv += c / math.Pow(v, f.tfc+j)
	}
	return pv + red/math.Pow(v, f.tfc+f.m) - c*f.acc
}

type oddL struct{ fc, fa, tm float64 }

// oddLFlujo son los argumentos de ODDL*: (liquidación, vencimiento, último interés, tasa,
// rend/precio, rescate, frecuencia, [base]); #NUM! si no se cumple vencimiento > liquidación >
// último interés.
func oddLFlujo(a []float64) (oddL, oddArgs, Error) {
	settle, maturity, last := math.Floor(a[0]), math.Floor(a[1]), math.Floor(a[2])
	rate, x, red := a[3], a[4], a[5]
	freq, basis := math.Trunc(a[6]), math.Trunc(a[7])
	if !(maturity > settle && settle > last) || last < 0 {
		return oddL{}, oddArgs{}, ErrNum
	}
	if rate < 0 || x < 0 || red <= 0 || !validFreq(freq) || basis < 0 || basis > 4 {
		return oddL{}, oddArgs{}, ErrNum
	}
	q := []float64{last}
	for k := 1.0; q[len(q)-1] < maturity; k++ {
		if k > 12_000 {
			return oddL{}, oddArgs{}, ErrNum
		}
		q = append(q, coupDate(last, -k, freq))
	}
	var fc, fa, tm float64
	for i := 1; i < len(q); i++ {
		lo, hi0 := q[i-1], q[i]
		nl := largoCupon(lo, hi0, freq, basis)
		hi := math.Min(hi0, maturity)
		fc += diasBase(lo, hi, basis) / nl
		if hasta := math.Min(hi, settle); hasta > lo {
			fa += diasBase(lo, hasta, basis) / nl
		}
		if s0 := math.Max(lo, settle); hi > s0 {
			tm += diasBase(s0, hi, basis) / nl
		}
	}
	return oddL{fc, fa, tm}, oddArgs{rate, x, red, freq}, ""
}

func (f oddL) precio(rate, yld, red, freq float64) float64 {
	c := (100 * rate) / freq
	return (red+c*f.fc)/(1+(f.tm*yld)/freq) - c*f.fa
}

func init() {
	registrar(map[string]fn{
		"ODDFPRICE": conNums(8, []float64{0}, func(a []float64) Value {
			f, o, err := oddFlujo(a)
			if err != "" {
				return err
			}
			return finito(f.precio(o.rate, o.x, o.red, o.freq))
		}),
		"ODDFYIELD": conNums(8, []float64{0}, func(a []float64) Value {
			f, o, err := oddFlujo(a)
			if err != "" {
				return err
			}
			if o.x <= 0 {
				return ErrNum
			}
			guess := o.rate
			if guess == 0 {
				guess = 0.05
			}
			return findRate(func(y float64) float64 { return f.precio(o.rate, y, o.red, o.freq) - o.x }, guess)
		}),
		"ODDLPRICE": conNums(7, []float64{0}, func(a []float64) Value {
			f, o, err := oddLFlujo(a)
			if err != "" {
				return err
			}
			return finito(f.precio(o.rate, o.x, o.red, o.freq))
		}),
		"ODDLYIELD": conNums(7, []float64{0}, func(a []float64) Value {
			f, o, err := oddLFlujo(a)
			if err != "" {
				return err
			}
			if o.x <= 0 {
				return ErrNum
			}
			c := (100 * o.rate) / o.freq
			return finito((((o.red+c*f.fc)/(o.x+c*f.fa) - 1) * o.freq) / f.tm)
		}),
	})
}

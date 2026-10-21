// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// erf y erfc a doble precisión (error ≲ 1e-15), las de functions/erf.ts. Son DOS funciones
// propias y no math.Erf a propósito: el motor de TypeScript no tiene el de Go, y las dos
// han de dar el mismo decimal (el contraste con LibreOffice lo exige a 1e-9).
//
//   - |x| < 3: la serie erf(x) = (2/√π)·e^(−x²)·Σ 2ⁿx²ⁿ⁺¹ / (2n+1)!!, de términos positivos.
//   - |x| ≥ 3: erfc por la fracción continua e^(−x²)/√π · 1/(x + ½/(x + 1/(x + 3/2/(x + …)))).
const (
	dosRaizPi = 1.1283791670955126 // 2/√π
	raizPi    = 1.7724538509055159
)

func erf(x float64) float64 {
	z := math.Abs(x)
	if z >= 3 {
		r := 1 - erfcPositivo(z)
		if x < 0 {
			return -r
		}
		return r
	}
	term, sum := z, z
	for n := 0; n < 200; n++ {
		term *= (2 * z * z) / float64(2*n+3)
		sum += term
		if term < sum*1e-17 {
			break
		}
	}
	r := dosRaizPi * math.Exp(-z*z) * sum
	if x < 0 {
		return -r
	}
	return r
}

// erfc es 1 − erf(x), sin perder cifras cuando es pequeña (x grande).
func erfc(x float64) float64 {
	if x < 0 {
		return 2 - erfc(-x)
	}
	if x >= 3 {
		return erfcPositivo(x)
	}
	return 1 - erf(x)
}

// erfcPositivo es erfc(x) para x ≥ 3 por la fracción continua, de atrás hacia delante.
func erfcPositivo(x float64) float64 {
	if x > 27 {
		return 0 // e^(−x²) ya es 0 en doble precisión
	}
	f := x
	for k := 80; k >= 1; k-- {
		f = x + float64(k)/2/f
	}
	return math.Exp(-x*x) / (raizPi * f)
}

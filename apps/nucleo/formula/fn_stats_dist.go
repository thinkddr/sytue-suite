// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"maps"
	"math"
)

// Distribuciones y contrastes (functions/stats.ts, segunda mitad). Las aproximaciones
// numéricas son LAS DEL ORIGINAL, coeficiente a coeficiente: cambiar una por la función
// «buena» de math (Gamma, Lgamma) daría otro decimal que la web. La erf, la normal y su
// inversa son las de erf.go, que sí se mejoraron en las dos a la vez.

// normCdf es Φ(z) por erfc: las dos colas con todas sus cifras.
func normCdf(z float64) float64 { return 0.5 * erfc(-z/math.Sqrt2) }

func normPdf(z float64) float64 { return math.Exp((-z*z)/2) / math.Sqrt(2*math.Pi) }

// normInv es la inversa de la normal estándar: la racional de Acklam (error relativo ≲ 1e-9)
// y un paso de Halley contra la Φ exacta, que la lleva a doble precisión.
func normInv(p float64) float64 {
	if p <= 0 || p >= 1 {
		return math.NaN()
	}
	x := normInvAprox(p)
	e := normCdf(x) - p
	u := e * math.Sqrt(2*math.Pi) * math.Exp(x*x/2)
	return x - u/(1+x*u/2)
}

func normInvAprox(p float64) float64 {
	a := [6]float64{-3.969683028665376e1, 2.209460984245205e2, -2.759285104469687e2, 1.38357751867269e2, -3.066479806614716e1, 2.506628277459239}
	b := [5]float64{-5.447609879822406e1, 1.615858368580409e2, -1.556989798598866e2, 6.680131188771972e1, -1.328068155288572e1}
	c := [6]float64{-7.784894002430293e-3, -3.223964580411365e-1, -2.400758277161838, -2.549732539343734, 4.374664141464968, 2.938163982698783}
	d := [4]float64{7.784695709041462e-3, 3.224671290700398e-1, 2.445134137142996, 3.754408661907416}
	cola := func(q float64) float64 {
		return (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
	const plow = 0.02425
	switch {
	case p < plow:
		return cola(math.Sqrt(-2 * math.Log(p)))
	case p <= 1-plow:
		q := p - 0.5
		r := q * q
		return ((((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q) / (((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	}
	return -cola(math.Sqrt(-2 * math.Log(1-p)))
}

// gammaln es el logaritmo de gamma por la aproximación de Lanczos (g = 7, n = 9).
func gammaln(x float64) float64 {
	c := [9]float64{
		0.99999999999980993, 676.5203681218851, -1259.1392167224028, 771.32342877765313, -176.61502916214059,
		12.507343278686905, -0.13857109526572012, 9.9843695780195716e-6, 1.5056327351493116e-7,
	}
	if x < 0.5 {
		return math.Log(math.Pi/math.Sin(math.Pi*x)) - gammaln(1-x)
	}
	x--
	a, t := c[0], x+7.5
	for i := 1; i < 9; i++ {
		a += c[i] / (x + float64(i))
	}
	return 0.5*math.Log(2*math.Pi) + (x+0.5)*math.Log(t) - t + math.Log(a)
}

func gammafn(x float64) float64 {
	if x < 0.5 {
		return math.Pi / (math.Sin(math.Pi*x) * gammafn(1-x))
	}
	return math.Exp(gammaln(x))
}

func lbeta(a, b float64) float64 { return gammaln(a) + gammaln(b) - gammaln(a+b) }

const diminuto = 1e-300

func noCero(x float64) float64 {
	if math.Abs(x) < diminuto {
		return diminuto
	}
	return x
}

// gammap es la gamma incompleta regularizada P(a, x): serie para x < a+1 y fracción
// continua (Lentz) para el resto.
func gammap(a, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if a <= 0 {
		return 1
	}
	if x < a+1 {
		ap, del := a, 1/a
		suma := del
		for n := 0; n < 300; n++ {
			ap++
			del *= x / ap
			suma += del
			if math.Abs(del) < math.Abs(suma)*1e-15 {
				break
			}
		}
		return suma * math.Exp(-x+a*math.Log(x)-gammaln(a))
	}
	b := x + 1 - a
	c, d := 1/diminuto, 1/b
	h := d
	for i := 1.0; i < 300; i++ {
		an := -i * (i - a)
		b += 2
		d = noCero(an*d + b)
		c = noCero(b + an/c)
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return 1 - math.Exp(-x+a*math.Log(x)-gammaln(a))*h
}

// betacf es la fracción continua de la beta incompleta (Lentz).
func betacf(a, b, x float64) float64 {
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1/noCero(1-(qab*x)/qap)
	h := d
	for m := 1.0; m < 300; m++ {
		m2 := 2 * m
		aa := (m * (b - m) * x) / ((qam + m2) * (a + m2))
		d = 1 / noCero(1+aa*d)
		c = noCero(1 + aa/c)
		h *= d * c
		aa = (-(a + m) * (qab + m) * x) / ((a + m2) * (qap + m2))
		d = 1 / noCero(1+aa*d)
		c = noCero(1 + aa/c)
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return h
}

// betai es la beta incompleta regularizada I_x(a, b).
func betai(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	bt := math.Exp(a*math.Log(x) + b*math.Log(1-x) - lbeta(a, b))
	if x < (a+1)/(a+b+2) {
		return (bt * betacf(a, b, x)) / a
	}
	return 1 - (bt*betacf(b, a, 1-x))/b
}

func binomPmf(j, n, p float64) float64 {
	if p <= 0 {
		return b2f(j == 0)
	}
	if p >= 1 {
		return b2f(j == n)
	}
	return math.Exp(gammaln(n+1) - gammaln(j+1) - gammaln(n-j+1) + j*math.Log(p) + (n-j)*math.Log(1-p))
}

// solveInc invierte por bisección una función creciente en [lo, hi].
func solveInc(f func(x float64) float64, p, lo, hi float64) float64 {
	for i := 0; i < 200; i++ {
		if m := (lo + hi) / 2; f(m) < p {
			lo = m
		} else {
			hi = m
		}
	}
	return (lo + hi) / 2
}

// solvePos invierte sobre [0, ∞) ampliando el extremo superior; solveReal, sobre toda la recta.
func solvePos(f func(x float64) float64, p float64) float64 {
	hi := 1.0
	for f(hi) < p && hi < 1e12 {
		hi *= 2
	}
	return solveInc(f, p, 0, hi)
}

func solveReal(f func(x float64) float64, p float64) float64 {
	hi, lo := 1.0, -1.0
	for f(hi) < p && hi < 1e12 {
		hi *= 2
	}
	for f(lo) > p && lo > -1e12 {
		lo *= 2
	}
	return solveInc(f, p, lo, hi)
}

func tcdf(x, df float64) float64 {
	cola := 0.5 * betai(df/2, 0.5, df/(df+x*x))
	if x >= 0 {
		return 1 - cola
	}
	return cola
}

func tpdf(x, df float64) float64 {
	return (math.Exp(gammaln((df+1)/2)-gammaln(df/2)) / math.Sqrt(df*math.Pi)) * jsPow(1+(x*x)/df, -(df+1)/2)
}

func fcdf(x, d1, d2 float64) float64 {
	if x <= 0 {
		return 0
	}
	return betai(d1/2, d2/2, (d1*x)/(d1*x+d2))
}

func fpdf(x, d1, d2 float64) float64 {
	if x <= 0 {
		return 0
	}
	return math.Exp((d1/2)*math.Log(d1/d2) + (d1/2-1)*math.Log(x) - ((d1+d2)/2)*math.Log(1+(d1*x)/d2) - lbeta(d1/2, d2/2))
}

func chisqpdf(x, df float64) float64 {
	switch {
	case x < 0:
		return 0
	case x == 0 && df < 2:
		return math.Inf(1)
	case x == 0 && df == 2:
		return 0.5
	case x == 0:
		return 0
	}
	return math.Exp((df/2-1)*math.Log(x) - x/2 - (df/2)*math.Ln2 - gammaln(df/2))
}

// dist envuelve una distribución: aridad exacta (−1 = sin comprobar), k números
// obligatorios y, si conCum, el booleano «acumulado» en el argumento k.
func dist(aridad, k int, conCum bool, f func(a []float64, cum bool) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if aridad >= 0 && len(args) != aridad {
			return ErrValue
		}
		if len(args) < k {
			return ErrValue
		}
		a, err := numArgs(args, k)
		if err != "" {
			return err
		}
		cum := false
		if conCum {
			if cum, err = CoerceBoolean(args[k].Value()); err != "" {
				return err
			}
		}
		return f(a, cum)
	}
}

// cumOpc es dist con el «acumulado» OPCIONAL (NEGBINOM.DIST, HYPGEOM.DIST).
func cumOpc(k int, f func(a []float64, cum bool) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < k || len(args) > k+1 {
			return ErrValue
		}
		a, err := numArgs(args, k)
		if err != "" {
			return err
		}
		cum, err := boolOpc(args, k, false)
		if err != "" {
			return err
		}
		return f(a, cum)
	}
}

// conLiteral reenvía a otra función insertando un argumento literal en la posición i:
// es como las heredadas (NORMSDIST, BETADIST…) llaman a las modernas.
func conLiteral(aridadMin, aridadMax, i int, v Value, destino fn) fn {
	return func(c *FnCtx, args []*Arg) any {
		if len(args) < aridadMin || len(args) > aridadMax {
			return ErrValue
		}
		nuevos := append(append(append([]*Arg(nil), args[:i]...), scalarArg(v)), args[i:]...)
		return destino(c, nuevos)
	}
}

// acumular01 suma la masa de `desde` a `hasta`. ponytail: el original no acota el bucle y
// BINOM.DIST(1E+15; …; TRUE) cuelga la pestaña; por encima de diez millones de términos
// es #NUM! (NaN). Relevar por la beta incompleta si alguien llega ahí de verdad.
func acumular01(desde, hasta float64, masa func(k float64) float64) float64 {
	if hasta > 1e7 {
		return math.NaN()
	}
	acc := 0.0
	for k := desde; k <= hasta; k++ {
		acc += masa(k)
	}
	return acc
}

// funcionesDistribuciones es el registro, como un MAPA y no como una función, por lo mismo
// que los de los demás ficheros: su complejidad medida sería la suma de los
// cuerpos que lleva dentro.
var funcionesDistribuciones = map[string]fn{
	"NORM.DIST": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 4 {
			return ErrValue
		}
		a, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		if a[2] <= 0 {
			return ErrNum
		}
		cum, err := CoerceBoolean(args[3].Value())
		if err != "" {
			return err
		}
		if z := (a[0] - a[1]) / a[2]; !cum {
			return normPdf(z) / a[2]
		}
		return normCdf((a[0] - a[1]) / a[2])
	},
	"NORM.S.DIST": dist(2, 1, true, func(a []float64, cum bool) Value {
		if cum {
			return normCdf(a[0])
		}
		return normPdf(a[0])
	}),
	"NORM.INV": dist(3, 3, false, func(a []float64, _ bool) Value {
		if a[2] <= 0 || a[0] <= 0 || a[0] >= 1 {
			return ErrNum
		}
		return a[1] + a[2]*normInv(a[0])
	}),
	"NORM.S.INV": dist(1, 1, false, func(a []float64, _ bool) Value {
		if a[0] <= 0 || a[0] >= 1 {
			return ErrNum
		}
		return normInv(a[0])
	}),
	"GAMMALN": dist(1, 1, false, func(a []float64, _ bool) Value {
		if a[0] <= 0 {
			return ErrNum
		}
		return finito(gammaln(a[0]))
	}),
	"GAMMA": dist(1, 1, false, func(a []float64, _ bool) Value {
		if a[0] <= 0 && esEntero(a[0]) {
			return ErrNum
		}
		return finito(gammafn(a[0]))
	}),
	"GAMMA.DIST": dist(4, 3, true, func(a []float64, cum bool) Value {
		x, al, be := a[0], a[1], a[2]
		if x < 0 || al <= 0 || be <= 0 {
			return ErrNum
		}
		if cum {
			return finito(gammap(al, x/be))
		}
		return finito(math.Exp((al-1)*math.Log(x) - x/be - al*math.Log(be) - gammaln(al)))
	}),
	"GAMMA.INV": dist(3, 3, false, func(a []float64, _ bool) Value {
		p, al, be := a[0], a[1], a[2]
		if p < 0 || p >= 1 || al <= 0 || be <= 0 {
			return ErrNum
		}
		return finito(be * solvePos(func(u float64) float64 { return gammap(al, u) }, p))
	}),
	"POISSON.DIST": dist(3, 2, true, func(a []float64, cum bool) Value {
		k, media := math.Trunc(a[0]), a[1]
		switch {
		case k < 0 || media < 0:
			return ErrNum
		case cum:
			return finito(1 - gammap(k+1, media))
		case media == 0:
			return b2f(k == 0)
		}
		return finito(math.Exp(-media + k*math.Log(media) - gammaln(k+1)))
	}),
	"EXPON.DIST": dist(3, 2, true, func(a []float64, cum bool) Value {
		x, lam := a[0], a[1]
		if x < 0 || lam <= 0 {
			return ErrNum
		}
		if cum {
			return finito(1 - math.Exp(-lam*x))
		}
		return finito(lam * math.Exp(-lam*x))
	}),
	"BINOM.DIST": dist(4, 3, true, func(a []float64, cum bool) Value {
		k, n, p := math.Trunc(a[0]), math.Trunc(a[1]), a[2]
		if k < 0 || k > n || p < 0 || p > 1 {
			return ErrNum
		}
		if !cum {
			return finito(binomPmf(k, n, p))
		}
		return finito(acumular01(0, k, func(j float64) float64 { return binomPmf(j, n, p) }))
	}),
	"WEIBULL.DIST": dist(4, 3, true, func(a []float64, cum bool) Value {
		x, al, be := a[0], a[1], a[2]
		if x < 0 || al <= 0 || be <= 0 {
			return ErrNum
		}
		z := jsPow(x/be, al)
		if cum {
			return finito(1 - math.Exp(-z))
		}
		return finito((al / jsPow(be, al)) * jsPow(x, al-1) * math.Exp(-z))
	}),
	"T.DIST": dist(3, 2, true, func(a []float64, cum bool) Value {
		df := math.Trunc(a[1])
		if df < 1 {
			return ErrNum
		}
		if cum {
			return finito(tcdf(a[0], df))
		}
		return finito(tpdf(a[0], df))
	}),
	"T.DIST.2T": dist(2, 2, false, func(a []float64, _ bool) Value {
		x, df := a[0], math.Trunc(a[1])
		if df < 1 || x < 0 {
			return ErrNum
		}
		return finito(betai(df/2, 0.5, df/(df+x*x)))
	}),
	"T.DIST.RT": dist(2, 2, false, func(a []float64, _ bool) Value {
		df := math.Trunc(a[1])
		if df < 1 {
			return ErrNum
		}
		return finito(1 - tcdf(a[0], df))
	}),
	"T.INV": dist(2, 2, false, func(a []float64, _ bool) Value {
		p, df := a[0], math.Trunc(a[1])
		if df < 1 || p <= 0 || p >= 1 {
			return ErrNum
		}
		return finito(solveReal(func(x float64) float64 { return tcdf(x, df) }, p))
	}),
	"T.INV.2T": dist(2, 2, false, func(a []float64, _ bool) Value {
		p, df := a[0], math.Trunc(a[1])
		if df < 1 || p <= 0 || p > 1 {
			return ErrNum
		}
		return finito(solveReal(func(x float64) float64 { return tcdf(x, df) }, 1-p/2))
	}),
	"CHISQ.DIST": dist(3, 2, true, func(a []float64, cum bool) Value {
		x, df := a[0], math.Trunc(a[1])
		if x < 0 || df < 1 {
			return ErrNum
		}
		if cum {
			return finito(gammap(df/2, x/2))
		}
		return finito(chisqpdf(x, df))
	}),
	"CHISQ.DIST.RT": dist(2, 2, false, func(a []float64, _ bool) Value {
		x, df := a[0], math.Trunc(a[1])
		if x < 0 || df < 1 {
			return ErrNum
		}
		return finito(1 - gammap(df/2, x/2))
	}),
	"CHISQ.INV":    chiInv(func(p float64) bool { return p < 0 || p >= 1 }, func(p float64) float64 { return p }),
	"CHISQ.INV.RT": chiInv(func(p float64) bool { return p <= 0 || p > 1 }, func(p float64) float64 { return 1 - p }),
	"F.DIST": dist(4, 3, true, func(a []float64, cum bool) Value {
		x, d1, d2 := a[0], math.Trunc(a[1]), math.Trunc(a[2])
		if x < 0 || d1 < 1 || d2 < 1 {
			return ErrNum
		}
		if cum {
			return finito(fcdf(x, d1, d2))
		}
		return finito(fpdf(x, d1, d2))
	}),
	"F.DIST.RT": dist(3, 3, false, func(a []float64, _ bool) Value {
		x, d1, d2 := a[0], math.Trunc(a[1]), math.Trunc(a[2])
		if x < 0 || d1 < 1 || d2 < 1 {
			return ErrNum
		}
		return finito(1 - fcdf(x, d1, d2))
	}),
	"F.INV":    fInv(func(p float64) bool { return p < 0 || p >= 1 }, func(p float64) float64 { return p }),
	"F.INV.RT": fInv(func(p float64) bool { return p <= 0 || p > 1 }, func(p float64) float64 { return 1 - p }),
	"BETA.DIST": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 4 || len(args) > 6 {
			return ErrValue
		}
		v, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		cum, err := CoerceBoolean(args[3].Value())
		if err != "" {
			return err
		}
		lo, hi, err := limitesBeta(args, 4)
		if err != "" {
			return err
		}
		x, al, be := v[0], v[1], v[2]
		if al <= 0 || be <= 0 || hi <= lo {
			return ErrNum
		}
		z := (x - lo) / (hi - lo)
		if z < 0 || z > 1 {
			return ErrNum
		}
		if cum {
			return finito(betai(al, be, z))
		}
		return finito(math.Exp((al-1)*math.Log(z)+(be-1)*math.Log(1-z)-lbeta(al, be)) / (hi - lo))
	},
	"BETA.INV": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 5 {
			return ErrValue
		}
		v, err := numArgs(args, 3)
		if err != "" {
			return err
		}
		lo, hi, err := limitesBeta(args, 3)
		if err != "" {
			return err
		}
		p, al, be := v[0], v[1], v[2]
		if p <= 0 || p >= 1 || al <= 0 || be <= 0 || hi <= lo {
			return ErrNum
		}
		z := solveInc(func(t float64) float64 { return betai(al, be, t) }, p, 0, 1)
		return finito(lo + z*(hi-lo))
	},
	"CONFIDENCE.NORM": dist(3, 3, false, func(a []float64, _ bool) Value {
		alfa, sd, n := a[0], a[1], math.Trunc(a[2])
		if alfa <= 0 || alfa >= 1 || sd <= 0 || n < 1 {
			return ErrNum
		}
		return finito(normInv(1-alfa/2) * (sd / math.Sqrt(n)))
	}),
	"CONFIDENCE.T": dist(3, 3, false, func(a []float64, _ bool) Value {
		alfa, sd, n := a[0], a[1], math.Trunc(a[2])
		if alfa <= 0 || alfa >= 1 || sd <= 0 || n < 2 {
			return ErrNum
		}
		t := solveReal(func(x float64) float64 { return tcdf(x, n-1) }, 1-alfa/2)
		return finito(t * (sd / math.Sqrt(n)))
	}),
	"STANDARDIZE": dist(3, 3, false, func(a []float64, _ bool) Value {
		if a[2] <= 0 {
			return ErrNum
		}
		return (a[0] - a[1]) / a[2]
	}),
	"FISHER": dist(1, 1, false, func(a []float64, _ bool) Value {
		if a[0] <= -1 || a[0] >= 1 {
			return ErrNum
		}
		return 0.5 * math.Log((1+a[0])/(1-a[0]))
	}),
	"FISHERINV": dist(1, 1, false, func(a []float64, _ bool) Value {
		e2 := math.Exp(2 * a[0])
		return finito((e2 - 1) / (e2 + 1))
	}),
	"GAUSS": dist(1, 1, false, func(a []float64, _ bool) Value { return normCdf(a[0]) - 0.5 }),
	"PHI":   dist(1, 1, false, func(a []float64, _ bool) Value { return normPdf(a[0]) }),
	// Variaciones CON repetición: n^k.
	"PERMUTATIONA": dist(-1, 2, false, func(a []float64, _ bool) Value {
		n, k := math.Trunc(a[0]), math.Trunc(a[1])
		if n < 0 || k < 0 {
			return ErrNum
		}
		return finito(jsPow(n, k))
	}),
	"LOGNORM.DIST": dist(4, 3, true, func(a []float64, cum bool) Value {
		x, mu, sd := a[0], a[1], a[2]
		if x <= 0 || sd <= 0 {
			return ErrNum
		}
		z := (math.Log(x) - mu) / sd
		if cum {
			return finito(normCdf(z))
		}
		return finito(normPdf(z) / (x * sd))
	}),
	"LOGNORM.INV": dist(-1, 3, false, func(a []float64, _ bool) Value {
		if a[0] <= 0 || a[0] >= 1 || a[2] <= 0 {
			return ErrNum
		}
		return finito(math.Exp(a[1] + a[2]*normInv(a[0])))
	}),
	// Fracasos antes del s-ésimo éxito.
	"NEGBINOM.DIST": cumOpc(3, func(a []float64, cum bool) Value {
		f, s, p := math.Trunc(a[0]), math.Trunc(a[1]), a[2]
		if f < 0 || s < 1 || p <= 0 || p > 1 {
			return ErrNum
		}
		masa := func(k float64) float64 {
			return math.Exp(gammaln(k+s) - gammaln(k+1) - gammaln(s) + s*math.Log(p) + k*math.Log(1-p))
		}
		if !cum {
			return finito(masa(f))
		}
		return finito(acumular01(0, f, masa))
	}),
	"HYPGEOM.DIST": cumOpc(4, func(a []float64, cum bool) Value {
		// k éxitos en la muestra, n tamaño de la muestra, exitos y total de la población.
		k, n, exitos, total := math.Trunc(a[0]), math.Trunc(a[1]), math.Trunc(a[2]), math.Trunc(a[3])
		if n <= 0 || n > total || exitos < 0 || exitos > total || k < 0 || k > n || k > exitos || n-k > total-exitos {
			return ErrNum
		}
		lc := func(a, b float64) float64 { return gammaln(a+1) - gammaln(b+1) - gammaln(a-b+1) }
		masa := func(j float64) float64 { return math.Exp(lc(exitos, j) + lc(total-exitos, n-j) - lc(total, n)) }
		if !cum {
			return finito(masa(k))
		}
		return finito(acumular01(math.Max(0, n-(total-exitos)), k, masa))
	}),
	// El menor k cuya acumulada alcanza alfa.
	"BINOM.INV": dist(-1, 3, false, func(a []float64, _ bool) Value {
		n, p, alfa := math.Trunc(a[0]), a[1], a[2]
		if n < 0 || p < 0 || p > 1 || alfa <= 0 || alfa >= 1 {
			return ErrNum
		}
		acc := 0.0
		for k := 0.0; k <= n && k <= 1e7; k++ { // ponytail: mismo tope que acumular01
			if acc += binomPmf(k, n, p); acc >= alfa {
				return k
			}
		}
		return n
	}),
}

// limitesBeta lee los límites opcionales [A, B] de BETA.DIST y BETA.INV (0 y 1).
func limitesBeta(args []*Arg, i int) (lo, hi float64, err Error) {
	if lo, err = numOpc(args, i, 0); err != "" {
		return lo, 0, err
	}
	hi, err = numOpc(args, i+1, 1)
	return lo, hi, err
}

func chiInv(fuera func(p float64) bool, objetivo func(p float64) float64) fn {
	return dist(2, 2, false, func(a []float64, _ bool) Value {
		p, df := a[0], math.Trunc(a[1])
		if fuera(p) || df < 1 {
			return ErrNum
		}
		return finito(solvePos(func(x float64) float64 { return gammap(df/2, x/2) }, objetivo(p)))
	})
}

func fInv(fuera func(p float64) bool, objetivo func(p float64) float64) fn {
	return dist(3, 3, false, func(a []float64, _ bool) Value {
		p, d1, d2 := a[0], math.Trunc(a[1]), math.Trunc(a[2])
		if fuera(p) || d1 < 1 || d2 < 1 {
			return ErrNum
		}
		return finito(solvePos(func(x float64) float64 { return fcdf(x, d1, d2) }, objetivo(p)))
	})
}

func init() {
	// Una COPIA: el registro es ahora un var de paquete y lo de abajo le añade
	// entradas; sin clonar, el segundo init lo dejaría tocado para siempre.
	t := maps.Clone(funcionesDistribuciones)
	for nombre, f := range funcionesRegresiones {
		t[nombre] = f
	}
	// Las heredadas son las modernas con otro nombre; algunas insertan el «acumulado» que
	// su firma antigua no tenía.
	for viejo, nuevo := range map[string]string{
		"GAMMALN.PRECISE": "GAMMALN", "BINOMDIST": "BINOM.DIST", "WEIBULL": "WEIBULL.DIST", "CONFIDENCE": "CONFIDENCE.NORM",
		"PEARSON": "CORREL", "COVAR": "COVARIANCE.P", "FORECAST": "FORECAST.LINEAR", "NORMDIST": "NORM.DIST",
		"NORMINV": "NORM.INV", "NORMSINV": "NORM.S.INV", "TINV": "T.INV.2T", "FDIST": "F.DIST.RT", "FINV": "F.INV.RT",
		"CHIDIST": "CHISQ.DIST.RT", "CHIINV": "CHISQ.INV.RT", "CHITEST": "CHISQ.TEST", "FTEST": "F.TEST", "TTEST": "T.TEST",
		"ZTEST": "Z.TEST", "EXPONDIST": "EXPON.DIST", "POISSON": "POISSON.DIST", "LOGINV": "LOGNORM.INV",
		"BETAINV": "BETA.INV", "GAMMADIST": "GAMMA.DIST", "GAMMAINV": "GAMMA.INV", "CRITBINOM": "BINOM.INV",
	} {
		t[viejo] = t[nuevo]
	}
	t["NORMSDIST"] = conLiteral(1, 1, 1, true, t["NORM.S.DIST"])
	t["HYPGEOMDIST"] = conLiteral(4, 4, 4, false, t["HYPGEOM.DIST"])
	t["LOGNORMDIST"] = conLiteral(3, 3, 3, true, t["LOGNORM.DIST"])
	t["NEGBINOMDIST"] = conLiteral(3, 3, 3, false, t["NEGBINOM.DIST"])
	t["BETADIST"] = conLiteral(3, 5, 3, true, t["BETA.DIST"])
	// TDIST(x, gl, colas): una cola es T.DIST.RT y dos, T.DIST.2T.
	t["TDIST"] = func(c *FnCtx, args []*Arg) any {
		if len(args) != 3 {
			return ErrValue
		}
		x, err := numArg(args, 0)
		if err != "" {
			return err
		}
		colas, err := numArg(args, 2)
		if err != "" {
			return err
		}
		switch n := math.Trunc(colas); {
		case x < 0 || (n != 1 && n != 2):
			return ErrNum
		case n == 1:
			return t["T.DIST.RT"](c, args[:2])
		}
		return t["T.DIST.2T"](c, args[:2])
	}
	registrar(t)
}

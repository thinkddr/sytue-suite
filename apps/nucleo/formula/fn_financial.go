// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// Financieras (functions/financial.ts).
//
// Núcleo del valor temporal del dinero (TVM). Para rate ≠ 0 se cumple
//
//	pv·(1+rate)^nper + pmt·(1 + rate·type)·((1+rate)^nper − 1)/rate + fv = 0
//
// con type 0 (pago a fin de periodo) o 1 (a principio); para rate = 0,
// pv + pmt·nper + fv = 0. De ahí se despejan PMT, PV, FV y NPER. IRR, RATE y XIRR
// resuelven por Newton con derivada numérica y bisección de reserva; #NUM! si no convergen.

// finNums lee `required` números obligatorios seguidos de opcionales con sus valores por
// defecto. Un obligatorio ausente → #VALUE!; el primer #error se propaga.
func finNums(args []*Arg, required int, defaults ...float64) ([]float64, Error) {
	out := make([]float64, 0, required+len(defaults))
	for i := 0; i < required+len(defaults); i++ {
		if i >= len(args) {
			if i < required {
				return nil, ErrValue
			}
			out = append(out, defaults[i-required])
			continue
		}
		n, err := CoerceNumber(args[i].Value())
		if err != "" {
			return nil, err
		}
		out = append(out, n)
	}
	return out, ""
}

// conNums envuelve una financiera de argumentos numéricos.
func conNums(required int, defaults []float64, f func(a []float64) Value) fn {
	return func(_ *FnCtx, args []*Arg) any {
		a, err := finNums(args, required, defaults...)
		if err != "" {
			return err
		}
		return f(a)
	}
}

// orderedNums son los números de UN argumento en orden: texto, blancos y booleanos se
// ignoran (regla de NPV e IRR). El orden importa: posición → periodo.
func orderedNums(a *Arg) ([]float64, Error) {
	var out []float64
	for _, fila := range a.Matrix() {
		for _, v := range fila {
			switch x := v.(type) {
			case Error:
				return nil, x
			case float64:
				out = append(out, x)
			}
		}
	}
	return out, ""
}

// flatNums son TODAS las celdas de un argumento coaccionadas (blanco = 0), para emparejar
// valores y fechas en XNPV y XIRR.
func flatNums(a *Arg) ([]float64, Error) {
	var out []float64
	for _, fila := range a.Matrix() {
		for _, v := range fila {
			n, err := CoerceNumber(v)
			if err != "" {
				return nil, err
			}
			out = append(out, n)
		}
	}
	return out, ""
}

/* ---------- Núcleo TVM ---------- */

func pmtCore(rate, nper, pv, fv, tipo float64) float64 {
	if rate == 0 {
		return -(pv + fv) / nper
	}
	p := jsPow(1+rate, nper)
	return (-(pv*p + fv) * rate) / ((1 + rate*tipo) * (p - 1))
}

func fvCore(rate, nper, pmt, pv, tipo float64) float64 {
	if rate == 0 {
		return -(pv + pmt*nper)
	}
	p := jsPow(1+rate, nper)
	return -(pv*p + (pmt*(1+rate*tipo)*(p-1))/rate)
}

func pvCore(rate, nper, pmt, fv, tipo float64) float64 {
	if rate == 0 {
		return -(pmt*nper + fv)
	}
	p := jsPow(1+rate, nper)
	return -((pmt*(1+rate*tipo)*(p-1))/rate + fv) / p
}

// ipmtAt es el interés del periodo `per` dado el pago. type=0: interés sobre el saldo al
// inicio de `per`. type=1: el primer pago es todo principal.
func ipmtAt(rate, per, pv, tipo, pmt float64) float64 {
	if tipo == 1 {
		if per <= 1 {
			return 0
		}
		return (fvCore(rate, per-2, pmt, pv, 1) - pmt) * rate
	}
	return fvCore(rate, per-1, pmt, pv, 0) * rate
}

// payType normaliza el tipo de pago a 0/1. ponytail (del original): cualquier valor ≠ 0
// cuenta como «principio de periodo», sin #NUM!.
func payType(tipo float64) float64 { return b2f(tipo != 0) }

/* ---------- Solucionador de tasa (Newton + bisección) ---------- */

// newton usa derivada numérica y se mantiene en el dominio rate > −1.
func newton(f func(x float64) float64, guess float64) (float64, bool) {
	x := guess
	if !esFinito(x) {
		x = 0.1
	}
	for i := 0; i < 80; i++ {
		fx := f(x)
		if !esFinito(fx) {
			return 0, false
		}
		if math.Abs(fx) < 1e-10 {
			return x, true
		}
		h := 1e-6 * math.Max(1, math.Abs(x))
		d := (f(x+h) - fx) / h
		if !esFinito(d) || d == 0 {
			return 0, false
		}
		nx := x - fx/d
		if !esFinito(nx) {
			return 0, false
		}
		if nx <= -1 {
			nx = (x - 1) / 2 // no salir del dominio
		}
		if math.Abs(nx-x) < 1e-12 {
			// Paso ínfimo: se acepta solo si es raíz real. Evita el falso «convergió» cuando
			// el recorte se estanca asintóticamente en rate = −1.
			fn := f(nx)
			return nx, esFinito(fn) && math.Abs(fn) < 1e-7
		}
		x = nx
	}
	return 0, false
}

// bisect busca un cambio de signo y biseca el primer intervalo que lo tenga.
// ponytail (del original): rejilla ~[-99 %, +10000 %]; una raíz fuera de ahí, o entre
// nodos de una función no monótona, no se halla → #NUM!.
func bisect(f func(x float64) float64) (float64, bool) {
	var pts []float64
	for r := -0.99; r <= 1.0001; r += 0.01 {
		pts = append(pts, r)
	}
	for r := 2.0; r <= 100; r++ {
		pts = append(pts, r)
	}
	prev := -0.9999
	fprev := f(prev)
	for _, x := range pts {
		fx := f(x)
		if esFinito(fprev) && esFinito(fx) && fprev*fx < 0 {
			lo, hi, flo := prev, x, fprev
			for i := 0; i < 100; i++ {
				mid := (lo + hi) / 2
				fm := f(mid)
				if !esFinito(fm) {
					return 0, false
				}
				if math.Abs(fm) < 1e-12 || hi-lo < 1e-13 {
					return mid, true
				}
				if flo*fm < 0 {
					hi = mid
				} else {
					lo, flo = mid, fm
				}
			}
			return (lo + hi) / 2, true
		}
		prev, fprev = x, fx
	}
	return 0, false
}

func findRate(f func(x float64) float64, guess float64) Value {
	if r, ok := newton(f, guess); ok {
		return r
	}
	if r, ok := bisect(f); ok {
		return r
	}
	return ErrNum
}

// escala es `Σ|v| || 1`: la normalización de los flujos de IRR y XIRR.
func escala(vals []float64) float64 {
	s := 0.0
	for _, v := range vals {
		s += math.Abs(v)
	}
	if s == 0 || math.IsNaN(s) {
		return 1
	}
	return s
}

/* ---------- Bono con cupones (PRICE, YIELD, DURATION, MDURATION) ---------- */

// topePeriodos corta los bucles periodo a periodo. ponytail: el original no los acota y
// un vencimiento o una vida útil de 1e15 cuelgan la pestaña; aquí eso es #NUM!. Ningún
// bono ni activo real tiene diez millones de periodos.
const topePeriodos = 1e7

// validFreq: 1 anual, 2 semestral o 4 trimestral.
func validFreq(freq float64) bool {
	f := math.Trunc(freq)
	return f == 1 || f == 2 || f == 4
}

/* ---------- Calendario de cupones (COUP*) ----------
 * Un bono devenga dentro del periodo de cupón que contiene la liquidación, y ese periodo
 * NO se puede aproximar con meses redondos sin equivocarse de un día en cada febrero.
 */

// coupDate es la fecha del k-ésimo cupón contando hacia ATRÁS desde el vencimiento. El
// calendario se ancla siempre en el vencimiento: encadenando cupón a cupón, el recorte de
// febrero se arrastra y un bono que vence el 31-mar acabaría pagando el 30.
func coupDate(maturity, k, freq float64) float64 {
	return shiftMonth(maturity, (-k*12)/freq, isMonthEnd(maturity))
}

// coupIndex es el índice del último cupón anterior o igual a la liquidación: a la vez,
// los cupones que quedan por cobrar (COUPNUM). −1 si la entrada no describe un bono.
func coupIndex(settle, maturity, freq float64) float64 {
	for k := 0.0; k <= 12_000; k++ {
		if coupDate(maturity, k, freq) <= settle {
			return k
		}
	}
	return -1
}

type coupArgs struct{ settle, maturity, freq, basis, k float64 }

// cupon envuelve la familia COUP*: (liquidación, vencimiento, frecuencia, [base]).
func cupon(f func(c coupArgs) Value) fn {
	return conNums(3, []float64{0}, func(a []float64) Value {
		c := coupArgs{settle: math.Floor(a[0]), maturity: math.Floor(a[1]), freq: math.Trunc(a[2]), basis: math.Trunc(a[3])}
		if c.settle < 0 || c.maturity <= c.settle || !validFreq(c.freq) || c.basis < 0 || c.basis > 4 {
			return ErrNum
		}
		if c.k = coupIndex(c.settle, c.maturity, c.freq); c.k < 0 {
			return ErrNum
		}
		return f(c)
	})
}

// nominal es la longitud NOMINAL del año de una base.
func nominal(basis float64) float64 {
	if basis == 3 {
		return 365
	}
	return 360
}

// bono es el bono en la fecha de liquidación según su calendario de cupones: cupones por
// cobrar (n), días devengados desde el anterior (a), del periodo (e) y hasta el siguiente
// (dsc). Es lo que piden PRICE, YIELD, DURATION y MDURATION (ECMA-376): no se pueden calcular
// con «n periodos enteros», porque la liquidación casi nunca cae en fecha de cupón.
type bono struct{ n, a, e, dsc float64 }

func nuevoBono(settle, maturity, freq, basis float64) (bono, Error) {
	if basis < 0 || basis > 4 {
		return bono{}, ErrNum
	}
	k := coupIndex(settle, maturity, freq)
	if k < 0 {
		return bono{}, ErrNum
	}
	a, err := dayCount(coupDate(maturity, k, freq), settle, basis)
	if err != "" {
		return bono{}, err
	}
	dsc, err := dayCount(settle, coupDate(maturity, k-1, freq), basis)
	if err != "" {
		return bono{}, err
	}
	return bono{n: k, a: a, e: periodoDeCupon(coupArgs{settle, maturity, freq, basis, k}), dsc: dsc}, ""
}

// periodoDeCupon son los días del periodo de cupón completo (COUPDAYS).
func periodoDeCupon(c coupArgs) float64 {
	if c.basis == 1 {
		return coupDate(c.maturity, c.k-1, c.freq) - coupDate(c.maturity, c.k, c.freq)
	}
	return nominal(c.basis) / c.freq
}

// precio es el precio por 100 de nominal: cupones y rescate descontados a `yld`, menos el
// cupón devengado.
func (b bono) precio(rate, yld, rescate, freq float64) float64 {
	cpn, w := (100*rate)/freq, b.dsc/b.e
	if b.n == 1 {
		return (rescate+cpn)/(1+(w*yld)/freq) - cpn*(b.a/b.e)
	}
	pv := rescate / jsPow(1+yld/freq, b.n-1+w)
	for k := 1.0; k <= b.n; k++ {
		pv += cpn / jsPow(1+yld/freq, k-1+w)
	}
	return pv - cpn*(b.a/b.e)
}

// duracion es la duración de Macaulay (años), con el precio sucio.
func (b bono) duracion(rate, yld, freq float64) float64 {
	cpn, w := (100*rate)/freq, b.dsc/b.e
	ponderado, pv := 0.0, 0.0
	for k := 1.0; k <= b.n; k++ {
		flujo := cpn
		if k == b.n {
			flujo += 100
		}
		cf := flujo / jsPow(1+yld/freq, k-1+w)
		ponderado += ((k - 1 + w) / freq) * cf
		pv += cf
	}
	return ponderado / pv
}

// accruedCoupons es el devengo entre dos fechas medido en CUPONES (la Σ Aᵢ/NLᵢ de
// ECMA-376). Con base 1 se recorren los cuasi-cupones uno a uno: cada uno mide distinto.
func accruedCoupons(start, settle, ancla, freq, basis float64) (float64, Error) {
	if basis != 1 {
		d, err := dayCount(start, settle, basis)
		return (d * freq) / nominal(basis), err
	}
	eom, meses := isMonthEnd(ancla), -12/freq
	primero := 0.0
	for shiftMonth(ancla, primero*meses, eom) > start {
		primero++
		if primero > 12_000 {
			return 0, ErrNum
		}
	}
	total := 0.0
	for i := primero; i >= -12_000; i-- {
		de, a := shiftMonth(ancla, i*meses, eom), shiftMonth(ancla, (i-1)*meses, eom)
		if lo, hi := math.Max(de, start), math.Min(a, settle); hi > lo {
			total += (hi - lo) / (a - de)
		}
		if a >= settle {
			return total, ""
		}
	}
	return 0, ErrNum
}

/* ---------- Depreciación ---------- */

// ddbPeriodo es el DDB de UN periodo, en forma cerrada.
func ddbPeriodo(cost, salvage, life, period, factor float64) float64 {
	f := factor / life
	if f >= 1 {
		// ponytail (del original): si factor ≥ life el primer periodo agota la depreciación.
		if period <= 1 {
			return math.Max(cost-salvage, 0)
		}
		return 0
	}
	ini, fin := cost*jsPow(1-f, period-1), cost*jsPow(1-f, period)
	if fin < salvage {
		return math.Max(ini-salvage, 0)
	}
	return ini - fin
}

// vdbInter es la depreciación 0→`hasta` con cambio a lineal, para VDB. Va periodo a
// periodo porque el cambio depende de lo que quede por amortizar, y es DEFINITIVO.
// `vidaLineal` no siempre es `life`: cuando VDB arranca más tarde, el tramo se calcula
// sobre la vida que queda.
func vdbInter(cost, salvage, life, vidaLineal, hasta, factor float64) float64 {
	total, porAmortizar, lineal, enLineal := 0.0, cost-salvage, 0.0, false
	fin := math.Ceil(hasta)
	if fin > topePeriodos {
		return math.NaN()
	}
	for i := 1.0; i <= fin; i++ {
		term := lineal
		if !enLineal {
			dec := ddbPeriodo(cost, salvage, life, i, factor)
			if lineal = porAmortizar / (vidaLineal - (i - 1)); lineal > dec {
				term, enLineal = lineal, true
			} else {
				term = dec
				porAmortizar -= dec
			}
		}
		if i == fin {
			term *= hasta + 1 - fin // el último periodo puede entrar solo en parte
		}
		total += term
	}
	return total
}

// acumulado es CUMIPMT y CUMPRINC: la suma entre start y end, con fv = 0.
func acumulado(parte func(pmt, interes float64) float64) fn {
	return conNums(6, nil, func(a []float64) Value {
		rate, nper, pv := a[0], a[1], a[2]
		start, end, tipo := math.Trunc(a[3]), math.Trunc(a[4]), payType(a[5])
		if rate <= 0 || nper <= 0 || pv <= 0 || start < 1 || end < start || end > nper {
			return ErrNum
		}
		if end > topePeriodos {
			return ErrNum
		}
		pmt, acc := pmtCore(rate, nper, pv, 0, tipo), 0.0
		for per := start; per <= end; per++ {
			acc += parte(pmt, ipmtAt(rate, per, pv, tipo, pmt))
		}
		return finito(acc)
	})
}

// cuota es IPMT y PPMT.
func cuota(parte func(pmt, interes float64) float64) fn {
	return conNums(4, []float64{0, 0}, func(a []float64) Value {
		rate, per, nper, tipo := a[0], a[1], a[2], payType(a[5])
		if per < 1 || per > nper {
			return ErrNum
		}
		pmt := pmtCore(rate, nper, a[3], a[4], tipo)
		return finito(parte(pmt, ipmtAt(rate, per, a[3], tipo, pmt)))
	})
}

// dolar es DOLLARDE y DOLLARFR.
func dolar(aDecimal bool) fn {
	return conNums(2, nil, func(a []float64) Value {
		frac := math.Trunc(a[1])
		if frac < 0 {
			return ErrNum
		}
		if frac == 0 {
			return ErrDiv0
		}
		signo, abs := 1.0, math.Abs(a[0])
		if a[0] < 0 {
			signo = -1
		}
		entero := math.Floor(abs)
		potencia := jsPow(10, math.Ceil(math.Log(frac)/math.Ln10))
		if aDecimal {
			return signo * (entero + ((abs-entero)*potencia)/frac)
		}
		return signo * (entero + ((abs-entero)*frac)/potencia)
	})
}

// descuento es DISC, INTRATE y YIELDDISC: (rescate − base) / divisor / fracción de año.
func descuento(f func(a []float64, yf float64) Value, conCero bool) fn {
	return conNums(4, []float64{0}, func(a []float64) Value {
		if a[1] <= a[0] || a[2] <= 0 || a[3] <= 0 {
			return ErrNum
		}
		yf, err := yearFrac(math.Floor(a[0]), math.Floor(a[1]), math.Trunc(a[4]))
		if err != "" {
			return err
		}
		if conCero && yf == 0 {
			return ErrNum
		}
		return f(a, yf)
	})
}

// funcionesFinancieras es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesFinancieras = map[string]fn{
	"PMT": conNums(3, []float64{0, 0}, func(a []float64) Value {
		if a[1] == 0 {
			return ErrDiv0
		}
		return finito(pmtCore(a[0], a[1], a[2], a[3], payType(a[4])))
	}),
	"PV": conNums(3, []float64{0, 0}, func(a []float64) Value { return finito(pvCore(a[0], a[1], a[2], a[3], payType(a[4]))) }),
	"FV": conNums(3, []float64{0, 0}, func(a []float64) Value { return finito(fvCore(a[0], a[1], a[2], a[3], payType(a[4]))) }),
	"NPER": conNums(3, []float64{0, 0}, func(a []float64) Value {
		rate, pmt, pv, fv, tipo := a[0], a[1], a[2], a[3], payType(a[4])
		if rate == 0 {
			if pmt == 0 {
				return ErrDiv0
			}
			return -(pv + fv) / pmt
		}
		k := (pmt * (1 + rate*tipo)) / rate
		ratio := (k - fv) / (pv + k)
		if !(ratio > 0) {
			return ErrNum // logaritmo de un valor no positivo: sin solución
		}
		return finito(math.Log(ratio) / math.Log(1+rate))
	}),
	"IPMT": cuota(func(_, interes float64) float64 { return interes }),
	"PPMT": cuota(func(pmt, interes float64) float64 { return pmt - interes }),
	"RATE": conNums(3, []float64{0, 0, 0.1}, func(a []float64) Value {
		nper, pmt, pv, fv, tipo := a[0], a[1], a[2], a[3], payType(a[4])
		norma := math.Abs(pv) + math.Abs(fv) + math.Abs(pmt)*math.Abs(nper) + 1
		return findRate(func(rate float64) float64 {
			if math.Abs(rate) < 1e-12 {
				return (pv + pmt*nper + fv) / norma
			}
			p := jsPow(1+rate, nper)
			return (pv*p + (pmt*(1+rate*tipo)*(p-1))/rate + fv) / norma
		}, a[5])
	}),
	// ponytail (del original): texto, blancos y lógicos se ignoran también como
	// argumentos escalares (=NPV(0,TRUE,1) da 1, no 2).
	"NPV": func(_ *FnCtx, args []*Arg) any {
		rate, err := numArg(args, 0)
		if err != "" {
			return err
		}
		if 1+rate == 0 {
			return ErrDiv0
		}
		s, i := 0.0, 0.0
		for _, a := range args[1:] {
			vals, err := orderedNums(a)
			if err != "" {
				return err
			}
			for _, v := range vals {
				i++
				s += v / jsPow(1+rate, i)
			}
		}
		return s
	},
	// El primer flujo es en t = 0.
	"IRR": func(_ *FnCtx, args []*Arg) any {
		if len(args) == 0 {
			return ErrValue
		}
		vals, err := orderedNums(args[0])
		if err != "" {
			return err
		}
		if len(vals) < 2 {
			return ErrNum
		}
		guess, err := numOpc(args, 1, 0.1)
		if err != "" {
			return err
		}
		norma := escala(vals)
		return findRate(func(r float64) float64 {
			s := 0.0
			for i, v := range vals {
				s += v / jsPow(1+r, float64(i))
			}
			return s / norma
		}, guess)
	},
	"SLN": conNums(3, nil, func(a []float64) Value {
		if a[2] == 0 {
			return ErrDiv0
		}
		return (a[0] - a[1]) / a[2]
	}),
	"SYD": conNums(4, nil, func(a []float64) Value {
		life, per := a[2], a[3]
		if life <= 0 || per < 1 || per > life {
			return ErrNum
		}
		return ((a[0] - a[1]) * (life - per + 1) * 2) / (life * (life + 1))
	}),
	// La tasa se redondea a 3 decimales; el primer periodo y (si month < 12) el último
	// se prorratean por meses.
	"DB": conNums(4, []float64{12}, func(a []float64) Value {
		cost, salvage, life, period, month := a[0], a[1], a[2], a[3], a[4]
		if cost <= 0 || salvage < 0 || life < 1 || period < 1 || month < 1 || month > 12 || period > life+1 || period > topePeriodos {
			return ErrNum
		}
		rate := jsRound((1-jsPow(salvage/cost, 1/life))*1000) / 1000
		acc, dep := 0.0, 0.0
		for p := 1.0; p <= period; p++ {
			switch p {
			case 1:
				dep = (cost * rate * month) / 12
			case life + 1:
				dep = ((cost - acc) * rate * (12 - month)) / 12
			default:
				dep = (cost - acc) * rate
			}
			acc += dep
		}
		return finito(dep)
	}),
	"DDB": conNums(4, []float64{2}, func(a []float64) Value {
		if a[0] < 0 || a[1] < 0 || a[2] <= 0 || a[3] < 1 || a[4] <= 0 {
			return ErrNum
		}
		return finito(ddbPeriodo(a[0], a[1], a[2], a[3], a[4]))
	}),
	// VDB es DDB por tramos, con periodos FRACCIONARIOS y, salvo `no_switch`, cambio a
	// lineal en cuanto la lineal del resto de vida deprecia más que la decreciente.
	"VDB": conNums(5, []float64{2, 0}, func(a []float64) Value {
		cost, salvage, life, start, end, factor := a[0], a[1], a[2], a[3], a[4], a[5]
		if cost < 0 || salvage < 0 || life <= 0 || factor <= 0 || start < 0 || end < start || end > life {
			return ErrNum
		}
		si := math.Floor(start)
		if a[6] == 0 {
			// Primero se deprecia hasta `start`, y el tramo va sobre lo que queda y con
			// la VIDA RESTANTE.
			restante := cost - vdbInter(cost, salvage, life, life, si, factor)
			return finito(vdbInter(restante, salvage, life, life-si, end-si, factor))
		}
		dep := 0.0
		if end > topePeriodos {
			return ErrNum
		}
		for p := si + 1; p <= math.Ceil(end); p++ {
			d := ddbPeriodo(cost, salvage, life, p, factor)
			if p-1 < start {
				d *= math.Min(1, p-start)
			} else if p > end {
				d *= end + 1 - p
			}
			dep += d
		}
		return finito(dep)
	}),
	"XNPV":     xnpv,
	"XIRR":     xirr,
	"CUMIPMT":  acumulado(func(_, interes float64) float64 { return interes }),
	"CUMPRINC": acumulado(func(pmt, interes float64) float64 { return pmt - interes }),
	"PDURATION": conNums(3, nil, func(a []float64) Value {
		if a[0] <= 0 || a[1] <= 0 || a[2] <= 0 {
			return ErrNum
		}
		return (math.Log(a[2]) - math.Log(a[1])) / math.Log(1+a[0])
	}),
	"RRI": conNums(3, nil, func(a []float64) Value {
		if a[0] <= 0 || a[1] == 0 {
			return ErrNum
		}
		return finito(jsPow(a[2]/a[1], 1/a[0]) - 1)
	}),
	"EFFECT": conNums(2, nil, func(a []float64) Value {
		p := math.Trunc(a[1])
		if a[0] <= 0 || p < 1 {
			return ErrNum
		}
		return jsPow(1+a[0]/p, p) - 1
	}),
	"NOMINAL": conNums(2, nil, func(a []float64) Value {
		p := math.Trunc(a[1])
		if a[0] <= 0 || p < 1 {
			return ErrNum
		}
		return p * (jsPow(1+a[0], 1/p) - 1)
	}),
	"DOLLARDE": dolar(true),
	"DOLLARFR": dolar(false),
	"PRICE": conNums(6, []float64{0}, func(a []float64) Value {
		settle, maturity, freq := math.Floor(a[0]), math.Floor(a[1]), math.Trunc(a[5])
		if maturity <= settle || a[2] < 0 || a[3] < 0 || a[4] <= 0 || !validFreq(freq) {
			return ErrNum
		}
		b, err := nuevoBono(settle, maturity, freq, math.Trunc(a[6]))
		if err != "" {
			return err
		}
		return finito(b.precio(a[2], a[3], a[4], freq))
	}),
	"YIELD": conNums(6, []float64{0}, func(a []float64) Value {
		settle, maturity, freq := math.Floor(a[0]), math.Floor(a[1]), math.Trunc(a[5])
		if maturity <= settle || a[2] < 0 || a[3] <= 0 || a[4] <= 0 || !validFreq(freq) {
			return ErrNum
		}
		b, err := nuevoBono(settle, maturity, freq, math.Trunc(a[6]))
		if err != "" {
			return err
		}
		return findRate(func(y float64) float64 { return b.precio(a[2], y, a[4], freq) - a[3] }, 0.05)
	}),
	"DURATION":  duracion(false),
	"MDURATION": duracion(true),
	"MIRR":      mirr,
	"DISC":      descuento(func(a []float64, yf float64) Value { return (a[3] - a[2]) / a[3] / yf }, true),
	"INTRATE":   descuento(func(a []float64, yf float64) Value { return (a[3] - a[2]) / a[2] / yf }, true),
	"RECEIVED": descuento(func(a []float64, yf float64) Value {
		if denom := 1 - a[3]*yf; denom > 0 {
			return a[2] / denom
		}
		return ErrNum
	}, false),
	"PRICEDISC": conNums(4, []float64{0}, func(a []float64) Value {
		settle, maturity := math.Floor(a[0]), math.Floor(a[1])
		if maturity <= settle || a[2] <= 0 || a[3] <= 0 {
			return ErrNum
		}
		yf, err := yearFrac(settle, maturity, math.Trunc(a[4]))
		if err != "" {
			return err
		}
		return a[3] - a[2]*a[3]*yf
	}),
	"YIELDDISC": conNums(4, []float64{0}, func(a []float64) Value {
		settle, maturity := math.Floor(a[0]), math.Floor(a[1])
		if maturity <= settle || a[2] <= 0 || a[3] <= 0 {
			return ErrNum
		}
		yf, err := yearFrac(settle, maturity, math.Trunc(a[4]))
		if err != "" {
			return err
		}
		if yf == 0 {
			return ErrNum
		}
		return (a[3] - a[2]) / a[2] / yf
	}),
	"COUPPCD": cupon(func(c coupArgs) Value { return coupDate(c.maturity, c.k, c.freq) }),
	"COUPNCD": cupon(func(c coupArgs) Value { return coupDate(c.maturity, c.k-1, c.freq) }),
	"COUPNUM": cupon(func(c coupArgs) Value { return c.k }),
	// Solo la base 1 cuenta los días de verdad; las demás usan la longitud NOMINAL del
	// año entre la frecuencia (de ahí el 182,5 de la base 3).
	"COUPDAYS": cupon(func(c coupArgs) Value { return periodoDeCupon(c) }),
	"COUPDAYBS": cupon(func(c coupArgs) Value {
		return valorOError(dayCount(coupDate(c.maturity, c.k, c.freq), c.settle, c.basis))
	}),
	"COUPDAYSNC": cupon(func(c coupArgs) Value {
		return valorOError(dayCount(c.settle, coupDate(c.maturity, c.k-1, c.freq), c.basis))
	}),
	// `método_cálculo` FALSO devenga desde el primer_interés, y SOLO si la liquidación
	// es posterior a él; si no, Excel devenga igualmente desde la emisión.
	"ACCRINT": conNums(6, []float64{0, 1}, func(a []float64) Value {
		emision, primero, settle := math.Floor(a[0]), math.Floor(a[1]), math.Floor(a[2])
		rate, par, freq, basis := a[3], a[4], math.Trunc(a[5]), math.Trunc(a[6])
		if settle <= emision || rate <= 0 || par <= 0 || !validFreq(freq) || basis < 0 || basis > 4 {
			return ErrNum
		}
		desde := emision
		if a[7] == 0 && settle > primero {
			desde = primero
		}
		cupones, err := accruedCoupons(desde, settle, primero, freq, basis)
		if err != "" {
			return err
		}
		return (par * rate * cupones) / freq
	}),
	// Paga TODO al vencimiento: sin calendario, es la fracción de año pura.
	"ACCRINTM": conNums(4, []float64{0}, func(a []float64) Value {
		emision, settle := math.Floor(a[0]), math.Floor(a[1])
		if settle <= emision || a[2] <= 0 || a[3] <= 0 {
			return ErrNum
		}
		yf, err := yearFrac(emision, settle, math.Trunc(a[4]))
		if err != "" {
			return err
		}
		return a[3] * a[2] * yf
	}),
	// Interés del periodo en un préstamo de AMORTIZACIÓN CONSTANTE. Negativo: es un pago.
	"ISPMT": conNums(4, nil, func(a []float64) Value {
		if a[2] == 0 {
			return ErrDiv0
		}
		return a[3] * a[0] * (a[1]/a[2] - 1)
	}),
}

func init() { registrar(funcionesFinancieras) }

// duracion es DURATION y MDURATION (= DURATION / (1 + rendimiento/frecuencia)).
func duracion(modificada bool) fn {
	return conNums(5, []float64{0}, func(a []float64) Value {
		settle, maturity, freq := math.Floor(a[0]), math.Floor(a[1]), math.Trunc(a[4])
		if maturity <= settle || a[2] < 0 || a[3] < 0 || !validFreq(freq) {
			return ErrNum
		}
		b, err := nuevoBono(settle, maturity, freq, math.Trunc(a[5]))
		if err != "" {
			return err
		}
		d := b.duracion(a[2], a[3], freq)
		if modificada {
			d /= 1 + a[3]/freq
		}
		return finito(d)
	})
}

// fechasYValores lee los dos rangos emparejados de XNPV y XIRR y devuelve los flujos y
// los días desde la primera fecha.
func fechasYValores(va, da *Arg, minimo int) (vals, dias []float64, err Error) {
	if vals, err = flatNums(va); err != "" {
		return nil, nil, err
	}
	fechas, err := flatNums(da)
	if err != "" {
		return nil, nil, err
	}
	if len(vals) < minimo || len(vals) != len(fechas) {
		return nil, nil, ErrNum
	}
	return vals, fechas, ""
}

// desdeLaPrimera convierte fechas en días desde la primera; una anterior es #NUM!.
func desdeLaPrimera(fechas []float64) ([]float64, Error) {
	base := math.Trunc(fechas[0])
	dias := make([]float64, len(fechas))
	for i, f := range fechas {
		if dias[i] = math.Trunc(f) - base; dias[i] < 0 {
			return nil, ErrNum
		}
	}
	return dias, ""
}

// xnpv es XNPV(rate, valores, fechas): descuenta cada flujo por (fecha − fecha0)/365 años.
func xnpv(_ *FnCtx, args []*Arg) any {
	if len(args) < 3 {
		return ErrValue
	}
	rate, err := numArg(args, 0)
	if err != "" {
		return err
	}
	vals, fechas, err := fechasYValores(args[1], args[2], 1)
	if err != "" {
		return err
	}
	if 1+rate <= 0 {
		return ErrNum
	}
	base, s := math.Trunc(fechas[0]), 0.0
	for i, v := range vals {
		di := math.Trunc(fechas[i])
		if di < base {
			return ErrNum
		}
		s += v / jsPow(1+rate, (di-base)/365)
	}
	return s
}

// xirr es XIRR(valores, fechas, [estimación]).
func xirr(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 {
		return ErrValue
	}
	vals, fechas, err := fechasYValores(args[0], args[1], 2)
	if err != "" {
		return err
	}
	guess, err := numOpc(args, 2, 0.1)
	if err != "" {
		return err
	}
	dias, err := desdeLaPrimera(fechas)
	if err != "" {
		return err
	}
	norma := escala(vals)
	return findRate(func(r float64) float64 {
		s := 0.0
		for i, v := range vals {
			s += v / jsPow(1+r, dias[i]/365)
		}
		return s / norma
	}, guess)
}

// mirr es MIRR(valores, tasa_financ, tasa_reinv): los negativos se descuentan a la de
// financiación y los positivos se capitalizan a la de reinversión.
func mirr(_ *FnCtx, args []*Arg) any {
	if len(args) != 3 {
		return ErrValue
	}
	vals, err := orderedNums(args[0])
	if err != "" {
		return err
	}
	fr, err := numArg(args, 1)
	if err != "" {
		return err
	}
	rr, err := numArg(args, 2)
	if err != "" {
		return err
	}
	n := float64(len(vals))
	if n < 2 {
		return ErrDiv0
	}
	pvNeg, fvPos, hayNeg, hayPos := 0.0, 0.0, false, false
	for i, v := range vals {
		if v < 0 {
			pvNeg += v / jsPow(1+fr, float64(i))
			hayNeg = true
		} else if v > 0 {
			fvPos += v * jsPow(1+rr, n-1-float64(i))
			hayPos = true
		}
	}
	if !hayPos || !hayNeg {
		return ErrDiv0 // hace falta al menos un flujo positivo y otro negativo
	}
	return finito(jsPow(-fvPos/pvNeg, 1/(n-1)) - 1)
}

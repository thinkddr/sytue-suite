// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"sort"
)

// Suavizado exponencial triple (Holt-Winters): FORECAST.ETS y su familia. Espejo de
// packages/formula/src/functions/ets.ts (donde está el modelo, las reglas de datos y por
// qué cada decisión): el mismo orden de operaciones, para que los dos motores den lo mismo.
// Solo suma, resta, producto, cociente y raíz cuadrada en el ajuste.

const (
	etsMaxPeriodo = 8760
	etsMaxPuntos  = 1_000_000
)

// etsSerie es la serie ya regular: un valor por paso de la línea de tiempo.
type etsSerie struct {
	y       []float64
	paso    float64 // en meses si mensual
	origen  float64 // primer instante (o su índice de mes si mensual)
	mensual bool
	dia     float64 // día del mes de todos los puntos (0 si todos son fin de mes)
	ultimo  float64 // instante del último punto
}

func etsMediana(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	m := len(s) >> 1
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

// etsAgregar agrega los valores de un mismo instante: 0 y 1 media, 2 y 3 cuenta, 4 máximo,
// 5 mediana, 6 mínimo, 7 suma.
func etsAgregar(v []float64, modo int) float64 {
	switch modo {
	case 2, 3:
		return float64(len(v))
	case 4:
		m := v[0]
		for _, x := range v[1:] {
			m = math.Max(m, x)
		}
		return m
	case 5:
		return etsMediana(v)
	case 6:
		m := v[0]
		for _, x := range v[1:] {
			m = math.Min(m, x)
		}
		return m
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	if modo == 7 {
		return s
	}
	return s / float64(len(v))
}

type etsMes struct {
	mes, dia float64
	fin      bool
}

func etsMesDe(serial float64) etsMes {
	f := ymdFromSerial(serial)
	sig := ymdFromSerial(serial + 1)
	return etsMes{mes: f.y*12 + f.m - 1, dia: f.d, fin: sig.d == 1}
}

func etsMultiplos(p float64, v []float64) bool {
	for _, d := range v {
		q := d / p
		if math.Abs(q-math.Round(q)) > 1e-6*math.Max(1, q) {
			return false
		}
	}
	return true
}

// etsPares lee (línea de tiempo, valores), ordena por instante y agrega los repetidos.
func etsPares(vals, tl []Value, agregacion int) (xs, ys []float64, err Error) {
	if len(vals) != len(tl) {
		return nil, nil, ErrNA
	}
	type par struct{ x, v float64 }
	pares := []par{}
	for i := range tl {
		if e, ok := tl[i].(Error); ok {
			return nil, nil, e
		}
		if e, ok := vals[i].(Error); ok {
			return nil, nil, e
		}
		x, ok := tl[i].(float64)
		if !ok {
			return nil, nil, ErrValue
		}
		if s, vacio := vals[i].(string); vacio && s == "" {
			continue // celda vacía: punto que falta
		}
		v, ok := vals[i].(float64)
		if !ok {
			return nil, nil, ErrValue
		}
		pares = append(pares, par{x, v})
	}
	sort.SliceStable(pares, func(a, b int) bool { return pares[a].x < pares[b].x })
	for i := 0; i < len(pares); {
		j := i
		var grupo []float64
		for j < len(pares) && pares[j].x == pares[i].x {
			grupo = append(grupo, pares[j].v)
			j++
		}
		xs = append(xs, pares[i].x)
		ys = append(ys, etsAgregar(grupo, agregacion))
		i = j
	}
	return xs, ys, ""
}

// etsDiferencias son las diferencias entre vecinos y la menor de ellas.
func etsDiferencias(xs []float64) (difs []float64, paso float64) {
	paso = math.Inf(1)
	difs = make([]float64, 0, len(xs)-1)
	for i := 1; i < len(xs); i++ {
		d := xs[i] - xs[i-1]
		difs = append(difs, d)
		paso = math.Min(paso, d)
	}
	return difs, paso
}

// etsPosMensual sitúa los puntos de una línea de tiempo que avanza por meses: el mismo día
// del mes (o fin de mes) en todos los puntos.
func etsPosMensual(xs []float64, s *etsSerie) ([]int, Error) {
	ms := make([]float64, len(xs))
	fin, mismoDia, dia0 := true, true, 0.0
	for i, x := range xs {
		if x != math.Trunc(x) {
			return nil, ErrNum
		}
		m := etsMesDe(x)
		if i == 0 {
			dia0 = m.dia
		}
		ms[i] = m.mes
		fin = fin && m.fin
		mismoDia = mismoDia && m.dia == dia0
	}
	if !fin && !mismoDia {
		return nil, ErrNum
	}
	dm, paso := etsDiferencias(ms)
	if paso < 1 || !etsMultiplos(paso, dm) {
		return nil, ErrNum
	}
	s.mensual, s.paso, s.origen = true, paso, ms[0]
	if !fin {
		s.dia = dia0
	}
	pos := make([]int, len(ms))
	for i, m := range ms {
		pos[i] = int(math.Round((m - s.origen) / paso))
	}
	return pos, ""
}

// etsCompletar rellena los huecos por interpolación lineal entre los vecinos con dato.
func etsCompletar(y []float64, hay []bool) {
	ant := 0
	for p := 1; p < len(y); p++ {
		if !hay[p] {
			continue
		}
		for k := ant + 1; k < p; k++ {
			y[k] = y[ant] + ((y[p]-y[ant])*float64(k-ant))/float64(p-ant)
		}
		ant = p
	}
}

// etsPreparar convierte (línea de tiempo, valores) en una serie regular, o da el error de
// Excel: tamaños distintos (#N/A), texto (#VALUE!), menos de tres instantes o paso no
// constante (#NUM!).
func etsPreparar(vals, tl []Value, completar bool, agregacion int) (*etsSerie, Error) {
	xs, ys, err := etsPares(vals, tl, agregacion)
	if err != "" {
		return nil, err
	}
	if len(xs) < 3 {
		return nil, ErrNum
	}
	s := &etsSerie{origen: xs[0], ultimo: xs[len(xs)-1]}
	difs, paso := etsDiferencias(xs)
	var pos []int
	if etsMultiplos(paso, difs) {
		s.paso = paso
		for _, x := range xs {
			pos = append(pos, int(math.Round((x-s.origen)/paso)))
		}
	} else if pos, err = etsPosMensual(xs, s); err != "" {
		return nil, err
	}
	n := pos[len(pos)-1] + 1
	if n > etsMaxPuntos {
		return nil, ErrNum
	}
	s.y = make([]float64, n)
	hay := make([]bool, n)
	for i, p := range pos {
		s.y[p] = ys[i]
		hay[p] = true
	}
	if completar {
		etsCompletar(s.y, hay)
	}
	return s, ""
}

// etsPasosHasta son los pasos que separan el objetivo del último punto (pueden ser fracción).
func (s *etsSerie) pasosHasta(objetivo float64) float64 {
	if !s.mensual {
		return (objetivo - s.ultimo) / s.paso
	}
	ult := s.origen + float64(len(s.y)-1)*s.paso
	t := etsMesDe(math.Floor(objetivo))
	var fr float64
	switch {
	case s.dia == 0 && t.fin:
		fr = 0
	case s.dia == 0:
		fr = (t.dia - 31) / 30
	default:
		fr = (t.dia - s.dia) / 30
	}
	return (t.mes - ult + fr) / s.paso
}

// etsAjuste son los parámetros hallados y lo que queda del modelo tras recorrer la serie.
type etsAjuste struct {
	alfa, beta, gamma float64
	sse               float64
	cuenta            float64
	mae, smape        float64
	nivel, tend       float64
	est               []float64
}

// etsInicio son el nivel, la tendencia y la estación de partida; ok=false si el modelo
// multiplicativo se sale del dominio.
func etsInicio(y []float64, m int, mult bool) (nivel, tend float64, s []float64, ok bool) {
	s = make([]float64, max(m, 1))
	if m <= 1 {
		k := min(len(y)-1, 4)
		return y[0], (y[k] - y[0]) / float64(k), s, true
	}
	m1, m2 := 0.0, 0.0
	for i := 0; i < m; i++ {
		m1 += y[i]
		m2 += y[m+i]
	}
	m1 /= float64(m)
	m2 /= float64(m)
	tend = (m2 - m1) / float64(m)
	nivel = m1 - (tend*float64(m+1))/2
	// La estación inicial: la media de los dos primeros periodos, sin tendencia.
	for i := 0; i < m; i++ {
		b1 := nivel + tend*float64(i+1)
		b2 := nivel + tend*float64(i+1+m)
		if !mult {
			s[i] = (y[i] - b1 + (y[m+i] - b2)) / 2
			continue
		}
		if !(b1 > 0) || !(b2 > 0) {
			return 0, 0, nil, false
		}
		s[i] = (y[i]/b1 + y[m+i]/b2) / 2
	}
	return nivel, tend, s, true
}

// etsPaso es un paso del modelo estacional: el nivel nuevo y la estación nueva.
func etsPaso(yt, base, sj, alfa, gamma float64, mult bool) (nivel, est float64, ok bool) {
	if !mult {
		return alfa*(yt-sj) + (1-alfa)*base, gamma*(yt-base) + (1-gamma)*sj, true
	}
	if !(sj > 0) || !(base > 0) {
		return 0, 0, false
	}
	return alfa*(yt/sj) + (1-alfa)*base, gamma*(yt/base) + (1-gamma)*sj, true
}

// etsRecorrer pasa el modelo por la serie con (α, β, γ) y devuelve el SSE (+Inf si el
// multiplicativo se sale del dominio); con `completo`, también lo que queda del modelo.
func etsRecorrer(y []float64, m int, mult bool, alfa, beta, gamma float64, completo bool) (float64, *etsAjuste) {
	n := len(y)
	nivel, tend, s, ok := etsInicio(y, m, mult)
	if !ok {
		return math.Inf(1), nil
	}
	estacional := m > 1
	t0, desde := 1, 1
	if estacional {
		t0, desde = 0, m
	}
	sse, cuenta, sabs, ssmape := 0.0, 0, 0.0, 0.0
	for t := t0; t < n; t++ {
		base := nivel + tend
		j, pred := 0, base
		if estacional {
			j = t % m
			pred = base + s[j]
			if mult {
				pred = base * s[j]
			}
		}
		e := y[t] - pred
		if t >= desde {
			sse += e * e
			cuenta++
		}
		if completo && t >= desde {
			sabs += math.Abs(e)
			if d := (math.Abs(y[t]) + math.Abs(pred)) / 2; d > 0 {
				ssmape += math.Abs(e) / d
			}
		}
		nuevo := alfa*y[t] + (1-alfa)*base
		if estacional {
			if nuevo, s[j], ok = etsPaso(y[t], base, s[j], alfa, gamma, mult); !ok {
				return math.Inf(1), nil
			}
		}
		tend = beta*(nuevo-nivel) + (1-beta)*tend
		nivel = nuevo
		if math.IsInf(sse, 0) || math.IsNaN(sse) {
			return math.Inf(1), nil
		}
	}
	if !completo {
		return sse, nil
	}
	// Los coeficientes en el orden de uso: el que sigue a la última observación va primero.
	var est []float64
	for i := 0; estacional && i < m; i++ {
		est = append(est, s[(n+i)%m])
	}
	c := float64(cuenta)
	return sse, &etsAjuste{
		alfa: alfa, beta: beta, gamma: gamma, sse: sse, cuenta: c,
		mae: sabs / c, smape: ssmape / c, nivel: nivel, tend: tend, est: est,
	}
}

// etsRejilla busca en una rejilla de 0,1 los parámetros de menor SSE.
func etsRejilla(coste func(q [3]float64) float64, dim int) (p [3]float64, mejor float64) {
	mejor = math.Inf(1)
	const pasos = 11
	total := pasos * pasos
	if dim == 3 {
		total *= pasos
	}
	for c := 0; c < total; c++ {
		q := [3]float64{float64(c%pasos) / 10, float64((c/pasos)%pasos) / 10, 0}
		if dim == 3 {
			q[2] = float64(c/(pasos*pasos)) / 10
		}
		if e := coste(q); e < mejor {
			mejor, p = e, q
		}
	}
	return p, mejor
}

// etsAfinar es la búsqueda por patrones desde p: prueba ±paso en cada parámetro y, cuando
// ninguno mejora, reduce el paso a la mitad, hasta 1e-4.
func etsAfinar(coste func(q [3]float64) float64, dim int, p [3]float64, mejor float64) [3]float64 {
	for paso := 0.05; paso >= 1e-4; paso /= 2 {
		for vuelta, mejora := 0, true; mejora && vuelta < 100; vuelta++ {
			mejora = false
			for k := 0; k < dim; k++ {
				for _, signo := range [2]float64{1, -1} {
					c := p
					c[k] = math.Min(1, math.Max(0, c[k]+signo*paso))
					if c[k] == p[k] {
						continue
					}
					if e := coste(c); e < mejor {
						mejor, p[k], mejora = e, c[k], true
					}
				}
			}
		}
	}
	return p
}

// etsAjustar es la rejilla de 0,1 y la búsqueda por patrones: los parámetros de menor SSE.
func etsAjustar(y []float64, m int, mult bool) (*etsAjuste, Error) {
	dim := 2
	if m > 1 {
		dim = 3
	}
	coste := func(q [3]float64) float64 {
		e, _ := etsRecorrer(y, m, mult, q[0], q[1], q[2], false)
		return e
	}
	p, mejor := etsRejilla(coste, dim)
	if math.IsInf(mejor, 0) || math.IsNaN(mejor) {
		return nil, ErrNum
	}
	p = etsAfinar(coste, dim, p, mejor)
	_, aj := etsRecorrer(y, m, mult, p[0], p[1], p[2], true)
	if aj == nil {
		return nil, ErrNum
	}
	return aj, ""
}

// etsDetectarPeriodo es el periodo estacional por autocorrelación de lo que queda tras quitar
// la recta de regresión; 1 si no hay estacionalidad.
func etsDetectarPeriodo(y []float64) int {
	n := len(y)
	if n < 6 {
		return 1
	}
	sx, sy := 0.0, 0.0
	for i := 0; i < n; i++ {
		sx += float64(i)
		sy += y[i]
	}
	mx, my := sx/float64(n), sy/float64(n)
	sxx, sxy := 0.0, 0.0
	for i := 0; i < n; i++ {
		dx := float64(i) - mx
		sxx += dx * dx
		sxy += dx * (y[i] - my)
	}
	b := sxy / sxx
	r := make([]float64, n)
	e0 := 0.0
	for i, v := range y {
		r[i] = v - my - b*(float64(i)-mx)
		e0 += r[i] * r[i]
	}
	esc := math.Abs(my) + 1
	if !(e0 > 1e-12*float64(n)*esc*esc) {
		return 1
	}
	maxLag := n >> 1
	rho := make([]float64, maxLag+2)
	for k := 1; k <= maxLag+1; k++ {
		c := 0.0
		for t := 0; t+k < n; t++ {
			c += r[t] * r[t+k]
		}
		rho[k] = c / float64(n-k) / (e0 / float64(n))
	}
	umbral := 2 / math.Sqrt(float64(n))
	maximo := math.Inf(-1)
	var picos []int
	for k := 2; k <= maxLag; k++ {
		if rho[k] > rho[k-1] && rho[k] >= rho[k+1] && rho[k] > umbral {
			picos = append(picos, k)
			maximo = math.Max(maximo, rho[k])
		}
	}
	for _, k := range picos {
		if rho[k] >= 0.9*maximo {
			return k
		}
	}
	return 1
}

func etsPlanos(a *Arg) []Value { return flatten(a.Matrix()) }

// etsOpc es un argumento numérico opcional: def si falta o llega vacío.
func etsOpc(args []*Arg, i int, def float64) (float64, Error) {
	if i >= len(args) {
		return def, ""
	}
	v := args[i].Value()
	if s, ok := v.(string); ok && s == "" {
		return def, ""
	}
	return CoerceNumber(v)
}

func etsOpciones(args []*Arg, iComp int) (bool, int, Error) {
	c, err := etsOpc(args, iComp, 1)
	if err != "" {
		return false, 0, err
	}
	g, err := etsOpc(args, iComp+1, 0)
	if err != "" {
		return false, 0, err
	}
	ci, gi := int(math.Trunc(c)), int(math.Trunc(g))
	if (ci != 0 && ci != 1) || gi < 0 || gi > 7 {
		return false, 0, ErrNum
	}
	return ci == 1, gi, ""
}

func etsPeriodo(y []float64, pedido float64, mult bool) (int, Error) {
	pf := math.Trunc(pedido)
	if pf < 0 || pf > etsMaxPeriodo {
		return 0, ErrNum
	}
	p := int(pf)
	if p == 0 {
		return 1, ""
	}
	m := p
	if p == 1 {
		m = etsDetectarPeriodo(y)
	}
	if m > 1 && 2*m > len(y) {
		return 0, ErrNum
	}
	if m > 1 && mult {
		for _, v := range y {
			if !(v > 0) {
				return 0, ErrNum
			}
		}
	}
	return m, ""
}

type etsModelo struct {
	serie *etsSerie
	m     int
	aj    *etsAjuste
}

func etsArmar(args []*Arg, iVals, iSea int, mult bool) (*etsModelo, Error) {
	sea, err := etsOpc(args, iSea, 1)
	if err != "" {
		return nil, err
	}
	completar, agg, err := etsOpciones(args, iSea+1)
	if err != "" {
		return nil, err
	}
	serie, err := etsPreparar(etsPlanos(args[iVals]), etsPlanos(args[iVals+1]), completar, agg)
	if err != "" {
		return nil, err
	}
	m, err := etsPeriodo(serie.y, sea, mult)
	if err != "" {
		return nil, err
	}
	aj, err := etsAjustar(serie.y, m, mult)
	if err != "" {
		return nil, err
	}
	return &etsModelo{serie, m, aj}, ""
}

func (c *etsModelo) entero(mult bool, j float64) float64 {
	aj := c.aj
	base := aj.nivel + j*aj.tend
	if c.m <= 1 {
		return base
	}
	m := float64(c.m)
	e := aj.est[int(math.Mod(math.Mod(j-1, m)+m, m))]
	if mult {
		return base * e
	}
	return base + e
}

func (c *etsModelo) pronostico(mult bool, h float64) float64 {
	j0 := math.Floor(h)
	f0 := c.entero(mult, j0)
	fr := h - j0
	if fr == 0 {
		return f0
	}
	return f0 + fr*(c.entero(mult, j0+1)-f0)
}

func etsObjetivo(args []*Arg, i int, c *etsModelo) (float64, Error) {
	t, err := CoerceNumber(args[i].Value())
	if err != "" {
		return 0, err
	}
	h := c.serie.pasosHasta(t)
	if !(h > 0) {
		return 0, ErrNum
	}
	return h, ""
}

func etsPrever(args []*Arg, mult bool) any {
	if len(args) < 3 || len(args) > 6 {
		return ErrValue
	}
	c, err := etsArmar(args, 1, 3, mult)
	if err != "" {
		return err
	}
	h, err := etsObjetivo(args, 0, c)
	if err != "" {
		return err
	}
	return finito(c.pronostico(mult, h))
}

func etsIntervalo(args []*Arg, mult bool) any {
	if len(args) < 3 || len(args) > 7 {
		return ErrValue
	}
	nc, err := etsOpc(args, 3, 0.95)
	if err != "" {
		return err
	}
	if !(nc > 0 && nc < 1) {
		return ErrNum
	}
	c, err := etsArmar(args, 1, 4, mult)
	if err != "" {
		return err
	}
	h, err := etsObjetivo(args, 0, c)
	if err != "" {
		return err
	}
	aj := c.aj
	hh := int(math.Max(1, math.Round(h)))
	suma := 1.0
	for j := 1; j < hh; j++ {
		cj := aj.alfa * (1 + float64(j)*aj.beta)
		if c.m > 1 && j%c.m == 0 {
			cj += aj.gamma * (1 - aj.alfa)
		}
		suma += cj * cj
	}
	sigma2 := aj.sse / aj.cuenta
	z := normInv((1 + nc) / 2)
	return finito(z * math.Sqrt(sigma2*suma))
}

func etsEstacionalidad(args []*Arg) any {
	if len(args) < 2 || len(args) > 4 {
		return ErrValue
	}
	completar, agg, err := etsOpciones(args, 2)
	if err != "" {
		return err
	}
	serie, err := etsPreparar(etsPlanos(args[0]), etsPlanos(args[1]), completar, agg)
	if err != "" {
		return err
	}
	return float64(etsDetectarPeriodo(serie.y))
}

func etsEstadistica(args []*Arg, mult bool) any {
	if len(args) < 3 || len(args) > 6 {
		return ErrValue
	}
	tipo, err := CoerceNumber(args[2].Value())
	if err != "" {
		return err
	}
	t := int(math.Trunc(tipo))
	if t < 1 || t > 8 {
		return ErrNum
	}
	c, err := etsArmar(args, 0, 3, mult)
	if err != "" {
		return err
	}
	aj := c.aj
	switch t {
	case 1:
		return aj.alfa
	case 2:
		return aj.beta
	case 3:
		if c.m > 1 {
			return aj.gamma
		}
		return 0.0
	case 8:
		return c.serie.paso
	case 4:
		y := c.serie.y
		d := 0.0
		for i := 1; i < len(y); i++ {
			d += math.Abs(y[i] - y[i-1])
		}
		d /= float64(len(y) - 1)
		if d > 0 {
			return aj.mae / d
		}
		return ErrDiv0
	case 5:
		return aj.smape
	case 6:
		return aj.mae
	}
	return math.Sqrt(aj.sse / aj.cuenta)
}

func init() {
	prever := func(mult bool) fn { return func(_ *FnCtx, a []*Arg) any { return etsPrever(a, mult) } }
	intervalo := func(mult bool) fn { return func(_ *FnCtx, a []*Arg) any { return etsIntervalo(a, mult) } }
	estadistica := func(mult bool) fn { return func(_ *FnCtx, a []*Arg) any { return etsEstadistica(a, mult) } }
	registrar(map[string]fn{
		"FORECAST.ETS.ADD":         prever(false),
		"FORECAST.ETS":             prever(false),
		"FORECAST.ETS.MULT":        prever(true),
		"FORECAST.ETS.PI.ADD":      intervalo(false),
		"FORECAST.ETS.CONFINT":     intervalo(false),
		"FORECAST.ETS.PI.MULT":     intervalo(true),
		"FORECAST.ETS.SEASONALITY": func(_ *FnCtx, a []*Arg) any { return etsEstacionalidad(a) },
		"FORECAST.ETS.STAT.ADD":    estadistica(false),
		"FORECAST.ETS.STAT":        estadistica(false),
		"FORECAST.ETS.STAT.MULT":   estadistica(true),
	})
}

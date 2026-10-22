// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analisis

import (
	"math"
	"slices"
)

// El ANÁLISIS DE HIPÓTESIS: Buscar objetivo, el Solver, la Tabla de datos y el informe
// de Escenarios. Son los puertos de `sheets/goalseek.ts`, `solver.ts`, `datatable.ts` y
// `scenario.ts` de la web, operación a operación —el orden de las sumas incluido—,
// para que el mismo libro dé el mismo número en las dos; los vectores de
// testdata/hipotesis.json lo comprueban. Ninguno conoce el motor: quien llama pasa la
// función que fija las celdas y lee el resultado.

// ---- Buscar objetivo ----

// ResultadoObjetivo es `GoalSeekResult`.
type ResultadoObjetivo struct {
	Resuelto    bool
	Valor       float64
	Iteraciones int
	Error       float64
}

// OpcionesObjetivo es `GoalSeekOptions`; los ceros son los valores por defecto.
type OpcionesObjetivo struct {
	X0          float64
	Tolerancia  float64 // 0: 1e-7
	Iteraciones int     // 0: 100
}

const factorExpansion = 1.6

// BuscarObjetivo es `goalSeek`: la x con f(x) ≈ objetivo. f da false en una «pared»
// (la celda objetivo no es un número).
//
//nolint:gocognit,gocyclo // es goalSeek paso a paso, para compararlas línea a línea
func BuscarObjetivo(f func(x float64) (float64, bool), objetivo float64, o OpcionesObjetivo) ResultadoObjetivo {
	tol := 1e-7
	if o.Tolerancia > 0 {
		tol = o.Tolerancia
	}
	tope := 100
	if o.Iteraciones > 0 {
		tope = o.Iteraciones
	}
	it, mejorX, mejorErr := 0, o.X0, math.Inf(1)
	g := func(x float64) (float64, bool) {
		if math.IsNaN(x) || math.IsInf(x, 0) || it >= tope {
			return 0, false
		}
		it++
		y, ok := f(x)
		if !ok || math.IsNaN(y) || math.IsInf(y, 0) {
			return 0, false
		}
		e := y - objetivo
		if a := math.Abs(e); a < mejorErr {
			mejorErr, mejorX = a, x
		}
		return e, true
	}
	res := func() ResultadoObjetivo {
		return ResultadoObjetivo{Resuelto: mejorErr <= tol, Valor: mejorX, Iteraciones: it, Error: mejorErr}
	}
	a := o.X0
	ga, ok := g(a)
	if !ok {
		for _, x := range sondasAlrededor(o.X0) {
			if gx, okx := g(x); okx {
				a, ga, ok = x, gx, true
				break
			}
		}
		if !ok {
			return res()
		}
	}
	if math.Abs(ga) <= tol {
		return res()
	}
	paso := pasoInicial(a)
	b := a + paso
	gb, okb := g(b)
	if !okb {
		b = a - paso
		gb, okb = g(b)
	}
	if !okb || math.Abs(gb) <= tol {
		return res()
	}
	oka := true
	for oka && okb && ga*gb > 0 {
		if math.Abs(ga) < math.Abs(gb) {
			a += factorExpansion * (a - b)
			ga, oka = g(a)
		} else {
			b += factorExpansion * (b - a)
			gb, okb = g(b)
		}
	}
	if oka && okb {
		if ga*gb <= 0 {
			refinarHorquilla(a, ga, b, gb, g, tol)
		} else {
			refinarSecante(a, ga, b, gb, g, tol)
		}
	}
	return res()
}

// refinarHorquilla es `refineBracket`: falsa posición con la modificación de Illinois.
func refinarHorquilla(a, ga, b, gb float64, g func(float64) (float64, bool), tol float64) {
	lado := 0
	for math.Abs(b-a) > 1e-14*(1+math.Abs(a)+math.Abs(b)) {
		if math.Abs(ga) <= tol || math.Abs(gb) <= tol {
			return
		}
		den := gb - ga
		x := (a + b) / 2
		if den != 0 {
			x = (a*gb - b*ga) / den
		}
		if !(x > math.Min(a, b) && x < math.Max(a, b)) {
			x = (a + b) / 2
		}
		gx, ok := g(x)
		if !ok {
			medio := (a + b) / 2
			gm, okm := g(medio)
			if !okm {
				return
			}
			if ga*gm > 0 {
				a, ga = medio, gm
			} else {
				b, gb = medio, gm
			}
			lado = 0
			continue
		}
		if ga*gx > 0 {
			a, ga = x, gx
			if lado == 1 {
				gb /= 2
			}
			lado = 1
		} else {
			b, gb = x, gx
			if lado == -1 {
				ga /= 2
			}
			lado = -1
		}
	}
}

// refinarSecante es `refineSecant`: secante abierta hasta dar con una horquilla.
func refinarSecante(a, ga, b, gb float64, g func(float64) (float64, bool), tol float64) {
	for {
		if math.Abs(gb) <= tol || gb == ga {
			return
		}
		x := b - (gb*(b-a))/(gb-ga)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return
		}
		gx, ok := g(x)
		if !ok {
			return
		}
		if ga*gx < 0 {
			refinarHorquilla(a, ga, x, gx, g, tol)
			return
		}
		if gb*gx < 0 {
			refinarHorquilla(b, gb, x, gx, g, tol)
			return
		}
		a, ga, b, gb = b, gb, x, gx
	}
}

func pasoInicial(a float64) float64 {
	if m := math.Abs(a); m > 1 {
		return m * 1e-3
	}
	return 1e-3
}

func sondasAlrededor(x0 float64) []float64 {
	base := x0
	if math.IsNaN(base) || math.IsInf(base, 0) {
		base = 0
	}
	escala := 1.0
	if math.Abs(base) > 1 {
		escala = math.Abs(base)
	}
	out := make([]float64, 0, 12)
	for _, f := range []float64{1e-3, 1e-2, 0.1, 1, 10, 100} {
		out = append(out, base+f*escala, base-f*escala)
	}
	return out
}

// ---- Solver ----

// Relaciones de una restricción: `lte`, `gte` o `eq`.
const (
	Menor = "lte"
	Mayor = "gte"
	Igual = "eq"
)

// EvaluacionSolver es `SolverEvalResult`: el objetivo y, por restricción, sus dos lados.
// NaN es «no numérico» (una pared).
type EvaluacionSolver struct {
	Objetivo      float64
	Restricciones [][2]float64
}

// ResultadoSolver es `SolverResult`. Objetivo NaN si no fue numérico.
type ResultadoSolver struct {
	Factible    bool
	Valores     []float64
	Objetivo    float64
	Iteraciones int
	Violacion   float64
}

// MetaSolver es `SolverGoal`: "max", "min" o "value" con su valor.
type MetaSolver struct {
	Tipo  string
	Valor float64
}

func violacion(rel string, lhs, rhs float64) float64 {
	if math.IsNaN(lhs) || math.IsNaN(rhs) || math.IsInf(lhs, 0) || math.IsInf(rhs, 0) {
		return math.Inf(1)
	}
	switch rel {
	case Menor:
		return math.Max(0, lhs-rhs)
	case Mayor:
		return math.Max(0, rhs-lhs)
	}
	return math.Abs(lhs - rhs)
}

type presupuesto struct{ usado, tope int }

type puntoSolver struct {
	x         []float64
	base, obj float64
	viol      float64
}

// Resolver es `solve` con sus opciones por defecto (tolerancia 1e-6, 2000
// evaluaciones, 6 reinicios, paso 1).
//
//nolint:gocognit,gocyclo // es solve paso a paso, para compararlas línea a línea
func Resolver(evaluar func(x []float64) EvaluacionSolver, x0 []float64, meta MetaSolver, relaciones []string) ResultadoSolver {
	n := len(x0)
	const tol, paso, reinicios = 1e-6, 1.0, 6
	pres := &presupuesto{tope: max(n+2, 2000)}
	if n == 0 {
		return ResultadoSolver{Objetivo: math.NaN(), Violacion: math.Inf(1)}
	}
	base := func(o float64) (float64, bool) {
		if math.IsNaN(o) || math.IsInf(o, 0) {
			return 0, false
		}
		switch meta.Tipo {
		case "max":
			return -o, true
		case "value":
			return math.Abs(o - meta.Valor), true
		}
		return o, true
	}
	sumaViol := func(cs [][2]float64) float64 {
		s := 0.0
		for i, rel := range relaciones {
			l, r := math.NaN(), math.NaN()
			if i < len(cs) {
				l, r = cs[i][0], cs[i][1]
			}
			s += violacion(rel, l, r)
		}
		return s
	}
	maxViol := func(cs [][2]float64) float64 {
		m := 0.0
		for i, rel := range relaciones {
			l, r := math.NaN(), math.NaN()
			if i < len(cs) {
				l, r = cs[i][0], cs[i][1]
			}
			m = math.Max(m, violacion(rel, l, r))
		}
		return m
	}
	var factible *puntoSolver
	cualquiera := puntoSolver{x: slices.Clone(x0), viol: math.Inf(1), base: math.Inf(1), obj: math.NaN()}
	considerar := func(x []float64, ev EvaluacionSolver) {
		v := maxViol(ev.Restricciones)
		b, ok := base(ev.Objetivo)
		obj := ev.Objetivo
		if math.IsInf(obj, 0) {
			obj = math.NaN()
		}
		if v <= tol && ok && (factible == nil || b < factible.base) {
			factible = &puntoSolver{x: slices.Clone(x), base: b, obj: obj, viol: v}
		}
		bv := math.Inf(1)
		if ok {
			bv = b
		}
		if v < cualquiera.viol || (v == cualquiera.viol && bv < cualquiera.base) {
			cualquiera = puntoSolver{x: slices.Clone(x), viol: v, base: bv, obj: obj}
		}
	}
	penalizado := func(p float64) func([]float64) float64 {
		return func(x []float64) float64 {
			if pres.usado >= pres.tope {
				return math.Inf(1)
			}
			pres.usado++
			ev := evaluar(x)
			considerar(x, ev)
			b, ok := base(ev.Objetivo)
			if !ok {
				return math.Inf(1)
			}
			return b + p*sumaViol(ev.Restricciones)
		}
	}
	inicio := slices.Clone(x0)
	local := max((n+1)*8, pres.tope/(reinicios+1))
	for k := 0; k <= reinicios && pres.usado < pres.tope; k++ {
		inicio = nelderMead(penalizado(1e3*math.Pow(10, float64(k))), inicio, paso, tol, pres, local)
	}
	if factible != nil {
		return ResultadoSolver{Factible: true, Valores: factible.x, Objetivo: factible.obj, Iteraciones: pres.usado, Violacion: factible.viol}
	}
	return ResultadoSolver{
		Factible: cualquiera.viol <= tol, Valores: cualquiera.x, Objetivo: cualquiera.obj,
		Iteraciones: pres.usado, Violacion: cualquiera.viol,
	}
}

// nelderMead es el de la web: reflexión 1, expansión 2, contracción 0,5 y encogimiento
// 0,5, con el presupuesto compartido y un tope por reinicio.
//
//nolint:gocognit,gocyclo,cyclop // es nelderMead paso a paso, para compararlas línea a línea
func nelderMead(f func([]float64) float64, inicio []float64, paso, tol float64, pres *presupuesto, topeLocal int) []float64 {
	n := len(inicio)
	simplex := make([][]float64, 0, 1+n)
	simplex = append(simplex, slices.Clone(inicio))
	for i := range n {
		v := slices.Clone(inicio)
		if mag := math.Abs(v[i]); mag > 1 {
			v[i] += 0.05 * mag
		} else {
			v[i] += paso
		}
		simplex = append(simplex, v)
	}
	fv := make([]float64, len(simplex))
	for i, v := range simplex {
		fv[i] = f(v)
	}
	desde := pres.usado
	peor := func() int {
		w := 0
		for i := range fv {
			if fv[i] > fv[w] {
				w = i
			}
		}
		return w
	}
	mejor := func() int {
		b := 0
		for i := range fv {
			if fv[i] < fv[b] {
				b = i
			}
		}
		return b
	}
	for pres.usado < pres.tope && pres.usado-desde < topeLocal {
		w, b := peor(), mejor()
		segundo := b
		for i := range fv {
			if i != w && fv[i] > fv[segundo] {
				segundo = i
			}
		}
		tam := 0.0
		for i := range simplex {
			d := 0.0
			for k := range n {
				d += math.Abs(simplex[i][k] - simplex[b][k])
			}
			tam = math.Max(tam, d)
		}
		if math.Abs(fv[w]-fv[b]) <= tol && tam <= tol {
			break
		}
		cent := make([]float64, n)
		for i := range simplex {
			if i == w {
				continue
			}
			for k := range n {
				cent[k] += simplex[i][k] / float64(n)
			}
		}
		reflejar := func(coef float64) []float64 {
			out := make([]float64, n)
			for k, c := range cent {
				out[k] = c + coef*(c-simplex[w][k])
			}
			return out
		}
		xr := reflejar(1)
		fr := f(xr)
		switch {
		case fr < fv[b]:
			xe := reflejar(2)
			if fe := f(xe); fe < fr {
				simplex[w], fv[w] = xe, fe
			} else {
				simplex[w], fv[w] = xr, fr
			}
		case fr < fv[segundo]:
			simplex[w], fv[w] = xr, fr
		default:
			xc := make([]float64, n)
			for k, c := range cent {
				xc[k] = c + 0.5*(simplex[w][k]-c)
			}
			if fc := f(xc); fc < fv[w] {
				simplex[w], fv[w] = xc, fc
				continue
			}
			for i := range simplex {
				if i == b {
					continue
				}
				nuevo := make([]float64, n)
				for k, vi := range simplex[i] {
					nuevo[k] = simplex[b][k] + 0.5*(vi-simplex[b][k])
				}
				simplex[i] = nuevo
				fv[i] = f(nuevo)
				if pres.usado >= pres.tope {
					break
				}
			}
		}
	}
	return slices.Clone(simplex[mejor()])
}

// ---- Tabla de datos ----

// CeldaPlan es una casilla de resultado de la tabla de datos y de qué cabeceras sale.
type CeldaPlan struct {
	C, F          int
	FilaDe, ColDe *[2]int // (c, f) de la cabecera de fila y de columna, o nil
}

// PlanTabla es `DataTablePlan`: la orientación, la celda de la fórmula y las casillas.
type PlanTabla struct {
	Orientacion string
	Formula     [2]int
	Celdas      []CeldaPlan
}

// TopeTablaDatos es `DATA_TABLE_MAX_CELLS`.
const TopeTablaDatos = 5000

// PlanearTabla es `planDataTable`. El error es el de la web: "no-input", "too-small" o
// "too-many".
func PlanearTabla(c1, f1, c2, f2 int, conFila, conCol bool) (PlanTabla, string) {
	if !conFila && !conCol {
		return PlanTabla{}, "no-input"
	}
	if c2-c1 < 1 || f2-f1 < 1 {
		return PlanTabla{}, "too-small"
	}
	var p PlanTabla
	switch {
	case conFila && conCol:
		p.Orientacion, p.Formula = "both", [2]int{c1, f1}
		for f := f1 + 1; f <= f2; f++ {
			for c := c1 + 1; c <= c2; c++ {
				p.Celdas = append(p.Celdas, CeldaPlan{C: c, F: f, FilaDe: &[2]int{c, f1}, ColDe: &[2]int{c1, f}})
			}
		}
	case conCol:
		p.Orientacion, p.Formula = "col", [2]int{c1 + 1, f1}
		for f := f1 + 1; f <= f2; f++ {
			p.Celdas = append(p.Celdas, CeldaPlan{C: c1 + 1, F: f, ColDe: &[2]int{c1, f}})
		}
	default:
		p.Orientacion, p.Formula = "row", [2]int{c1, f1 + 1}
		for c := c1 + 1; c <= c2; c++ {
			p.Celdas = append(p.Celdas, CeldaPlan{C: c, F: f1 + 1, FilaDe: &[2]int{c, f1}})
		}
	}
	if len(p.Celdas) > TopeTablaDatos {
		return PlanTabla{}, "too-many"
	}
	return p, ""
}

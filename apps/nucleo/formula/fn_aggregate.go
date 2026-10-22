// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import "math"

// SUBTOTAL y AGGREGATE (functions/aggregate.ts): las agregaciones que saben de ESQUEMA.
// Se saltan las celdas que ya son un SUBTOTAL o un AGGREGATE —así el total general de
// un listado con subtotales es SUBTOTAL(9; todo el rango) sin contar nada dos veces—,
// las filas que esconde el filtro y, según el número u opción, las ocultas a mano.
//
// No reimplementan ninguna agregación: construyen el rango con lo saltado VACÍO, que es
// lo que toda agregación ya ignora, y llaman a la función de siempre del registro. Los
// vectores son los de la web: testdata/subtotal.json.

var (
	deSubtotal = []string{
		"AVERAGE", "COUNT", "COUNTA", "MAX", "MIN", "PRODUCT", "STDEV", "STDEVP", "SUM", "VAR", "VARP",
	}
	deAggregate = []string{
		"AVERAGE", "COUNT", "COUNTA", "MAX", "MIN", "PRODUCT", "STDEV.S", "STDEV.P", "SUM", "VAR.S",
		"VAR.P", "MEDIAN", "MODE.SNGL", "LARGE", "SMALL", "PERCENTILE.INC", "QUARTILE.INC",
		"PERCENTILE.EXC", "QUARTILE.EXC",
	}
)

// salto es qué se salta al leer un rango. `ocultas`: 0 nada, 1 las filtradas, 2 también
// las de a mano.
type salto struct {
	anidadas bool
	ocultas  uint8
	errores  bool
}

// sinSaltadas es el argumento con lo saltado convertido en celdas vacías. Si no es una
// referencia, solo se pueden saltar los errores.
func sinSaltadas(c *FnCtx, a *Arg, s salto) *Arg {
	rr := a.Ref()
	m := a.Matrix()
	out := make(Matrix, len(m))
	for i, fila := range m {
		out[i] = make([]Value, len(fila))
		for j, v := range fila {
			if !fuera(c, rr, s, v, j, i) {
				out[i][j] = v
			} else {
				out[i][j] = ""
			}
		}
	}
	var sv Value = ""
	if len(out) > 0 && len(out[0]) > 0 {
		sv = out[0][0]
	}
	return &Arg{sv: sv, svOK: true, mv: out, mvOK: true, ref: rr, refOK: true}
}

// fuera dice si la celda (j, i) del argumento se salta.
func fuera(c *FnCtx, rr *RefRange, s salto, v Value, j, i int) bool {
	if s.errores && IsError(v) {
		return true
	}
	if rr == nil {
		return false
	}
	if s.anidadas && c.SubtotalCell(rr.Sheet, rr.C1+j, rr.R1+i) {
		return true
	}
	// RowHidden: 2 filtrada, 1 a mano.
	h := c.RowHidden(rr.Sheet, rr.R1+i)
	return (h == 2 && s.ocultas >= 1) || (h == 1 && s.ocultas == 2)
}

// numeroDeFuncion es el primer argumento: entero truncado, o el error que toque.
func numeroDeFuncion(a *Arg) (int, Error) {
	n, err := CoerceNumber(a.Value())
	if err != "" {
		return 0, err
	}
	return int(math.Trunc(n)), ""
}

// soloReferencias dice si todos los argumentos son referencias literales.
func soloReferencias(args []*Arg) bool {
	for _, a := range args {
		if a.Ref() == nil {
			return false
		}
	}
	return len(args) > 0
}

func init() {
	registrar(map[string]fn{"SUBTOTAL": subtotal, "AGGREGATE": aggregate})
}

// subtotal es SUBTOTAL(función; ref1; …): 1-11 saltan lo filtrado y los resúmenes
// anidados; 101-111, además, lo oculto a mano. Solo admite referencias.
func subtotal(c *FnCtx, args []*Arg) any {
	if len(args) == 0 {
		return ErrValue
	}
	n, err := numeroDeFuncion(args[0])
	if err != "" {
		return err
	}
	base := n
	if n > 100 {
		base = n - 100
	}
	refs := args[1:]
	if base < 1 || base > len(deSubtotal) || !soloReferencias(refs) {
		return ErrValue
	}
	s := salto{anidadas: true, ocultas: 1}
	if n > 100 {
		s.ocultas = 2
	}
	return registro[deSubtotal[base-1]](c, conSaltos(c, refs, s))
}

// aggregate es AGGREGATE(función; opciones; ref1; …) y AGGREGATE(función; opciones;
// matriz; k).
func aggregate(c *FnCtx, args []*Arg) any {
	if len(args) == 0 {
		return ErrValue
	}
	n, err := numeroDeFuncion(args[0])
	if err != "" {
		return err
	}
	opcion := 0
	if len(args) > 1 {
		if opcion, err = numeroDeFuncion(args[1]); err != "" {
			return err
		}
	}
	if n < 1 || n > len(deAggregate) || opcion < 0 || opcion > 7 {
		return ErrValue
	}
	s := salto{anidadas: opcion < 4, errores: opcion == 2 || opcion == 3 || opcion == 6 || opcion == 7}
	if opcion%2 == 1 {
		s.ocultas = 2
	}
	f := registro[deAggregate[n-1]]
	if n >= 14 {
		// Forma de matriz: una matriz (o rango) y la k.
		if len(args) != 4 {
			return ErrValue
		}
		return f(c, []*Arg{sinSaltadas(c, args[2], s), args[3]})
	}
	if len(args) < 3 || !soloReferencias(args[2:]) {
		return ErrValue
	}
	return f(c, conSaltos(c, args[2:], s))
}

// conSaltos aplica sinSaltadas a cada referencia.
func conSaltos(c *FnCtx, refs []*Arg, s salto) []*Arg {
	out := make([]*Arg, len(refs))
	for i, a := range refs {
		out[i] = sinSaltadas(c, a, s)
	}
	return out
}

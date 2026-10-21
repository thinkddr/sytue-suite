// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"regexp"
	"strings"
)

// Búsqueda y referencia (functions/lookup.ts). Las que solo necesitan la referencia
// (ROW, COLUMN, ROWS, COLUMNS, OFFSET) usan Arg.Ref para no evaluar celdas. CHOOSE y el
// «si_no_encontrado» de XLOOKUP son perezosos.

// flatten aplana una matriz en orden de lectura.
func flatten(m Matrix) []Value {
	var out []Value
	for _, fila := range m {
		out = append(out, fila...)
	}
	return out
}

// primeraCol es `tabla.map(fila => fila[0] ?? ”)`.
func primeraCol(m Matrix) []Value {
	out := make([]Value, len(m))
	for r := range m {
		out[r] = en(m, r, 0, "")
	}
	return out
}

// wildRegex: si `lookup` es texto con comodines de hoja, su patrón; si no, nil y se usa
// la igualdad normal.
func wildRegex(lookup Value) *regexp.Regexp {
	if s, ok := lookup.(string); ok && strings.ContainsAny(s, "*?") {
		return wildcardToRegex(s)
	}
	return nil
}

// casa: ¿la celda es la buscada? Con patrón, solo contra celdas de texto; sin él, por
// igualdad de hoja (texto sin distinguir mayúsculas). Una celda de error nunca casa.
func casa(v, lookup Value, rx *regexp.Regexp) bool {
	if rx != nil {
		s, ok := v.(string)
		return ok && rx.MatchString(s)
	}
	c, err := CompareValues(v, lookup)
	return err == "" && c == 0
}

// exactIndex es la coincidencia EXACTA (VLOOKUP FALSE, MATCH 0), con comodines.
func exactIndex(vec []Value, lookup Value) int {
	rx := wildRegex(lookup)
	for i, v := range vec {
		if casa(v, lookup, rx) {
			return i
		}
	}
	return -1
}

// approx es la coincidencia APROXIMADA sobre un vector ordenado: ascendente (signo 1), el
// mayor valor ≤ lookup; descendente (signo -1), el menor ≥ lookup. Para en el primero que
// se pasa; las celdas de error se saltan.
func approx(vec []Value, lookup Value, signo int) int {
	cand := -1
	for i, v := range vec {
		c, err := CompareValues(v, lookup)
		if err != "" {
			continue
		}
		if c*signo > 0 {
			break
		}
		cand = i
	}
	return cand
}

// xfind es la búsqueda de XLOOKUP y XMATCH: exacta (modo 0), comodines (modo 2) o, si
// falla, el «siguiente menor» (-1) o «siguiente mayor» (1). El sentido rompe empates.
// ponytail (del original): los modos binarios (±2) se tratan como lineales en su sentido.
func xfind(vec []Value, lookup Value, modo, sentido float64) int {
	orden := make([]int, len(vec))
	for i := range orden {
		if orden[i] = i; sentido < 0 {
			orden[i] = len(vec) - 1 - i
		}
	}
	var rx *regexp.Regexp
	if modo == 2 {
		rx = wildRegex(lookup)
	}
	for _, i := range orden {
		if casa(vec[i], lookup, rx) {
			return i
		}
	}
	if modo != -1 && modo != 1 {
		return -1
	}
	mejor := -1
	for _, i := range orden {
		c, err := CompareValues(vec[i], lookup)
		if err != "" || c == 0 || float64(c) != modo {
			continue
		}
		if mejor >= 0 {
			// El mayor de los menores, o el menor de los mayores.
			if d, err := CompareValues(vec[i], vec[mejor]); err != "" || float64(d) != -modo {
				continue
			}
		}
		mejor = i
	}
	return mejor
}

var rePalabraHoja = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

// quoteSheet cita el nombre de hoja para ADDRESS si no es una palabra.
func quoteSheet(nombre string) string {
	if rePalabraHoja.MatchString(nombre) {
		return nombre
	}
	return "'" + strings.ReplaceAll(nombre, "'", "''") + "'"
}

// aproxOpc lee el cuarto argumento de VLOOKUP y HLOOKUP (TRUE si falta).
func aproxOpc(args []*Arg) (bool, Error) {
	if len(args) < 4 {
		return true, ""
	}
	return CoerceBoolean(args[3].Value())
}

// buscarEn es VLOOKUP (vertical) y HLOOKUP: busca en la primera columna (o fila) y
// devuelve la celda de la columna (o fila) pedida, 1-based.
func buscarEn(vertical bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 4 {
			return ErrValue
		}
		lookup := args[0].Value()
		if IsError(lookup) {
			return lookup
		}
		tabla := args[1].Matrix()
		filas, cols := dims(tabla)
		idx, err := intOpc(args, 2, 1)
		if err != "" {
			return err
		}
		tope := filas
		if vertical {
			tope = cols
		}
		if idx < 1 {
			return ErrValue
		}
		if idx > float64(tope) {
			return ErrRef
		}
		aprox, err := aproxOpc(args)
		if err != "" {
			return err
		}
		vec := fila0(tabla)
		if vertical {
			vec = primeraCol(tabla)
		}
		pos := exactIndex(vec, lookup)
		if aprox {
			pos = approx(vec, lookup, 1)
		}
		if pos < 0 {
			return ErrNA
		}
		if vertical {
			return en(tabla, pos, int(idx)-1, "")
		}
		return en(tabla, int(idx)-1, pos, "")
	}
}

// fila0 es `m[0] ?? []`.
func fila0(m Matrix) []Value {
	if len(m) == 0 {
		return nil
	}
	return m[0]
}

// unaDimension lee el vector de MATCH y XMATCH: una matriz 2D de verdad es #N/A.
func unaDimension(m Matrix) ([]Value, bool) {
	if r, c := dims(m); r > 1 && c > 1 {
		return nil, false
	}
	return flatten(m), true
}

func posicion(idx int) Value {
	if idx < 0 {
		return ErrNA
	}
	return float64(idx + 1)
}

// errorSuelto: una matriz 1×1 que es un error (ROWS y COLUMNS lo propagan).
func errorSuelto(m Matrix) (Error, bool) {
	if r, c := dims(m); r == 1 && c == 1 {
		e, ok := m[0][0].(Error)
		return e, ok
	}
	return "", false
}

// medida es ROWS y COLUMNS: usa la referencia si la hay (no evalúa).
func medida(deRef func(r *RefRange) int, deMatriz func(filas, cols int) int) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		if ref := args[0].Ref(); ref != nil {
			return float64(deRef(ref))
		}
		m := args[0].Matrix()
		if e, ok := errorSuelto(m); ok {
			return e
		}
		return float64(deMatriz(dims(m)))
	}
}

// filaOColumna es ROW y COLUMN: sin argumento, la de la celda actual; con una referencia,
// su número o, si abarca varias, el vector (vertical para ROW, horizontal para COLUMN).
func filaOColumna(fila bool) fn {
	return func(c *FnCtx, args []*Arg) any {
		if len(args) == 0 {
			if fila {
				return float64(c.Here.Row + 1)
			}
			return float64(c.Here.Col + 1)
		}
		if len(args) != 1 || args[0].Ref() == nil {
			return ErrValue
		}
		ref := args[0].Ref()
		ini, fin := ref.C1, ref.C2
		if fila {
			ini, fin = ref.R1, ref.R2
		}
		if ini == fin {
			return float64(ini + 1)
		}
		out := make(Matrix, 0, fin-ini+1)
		vector := make([]Value, 0, fin-ini+1)
		for i := ini; i <= fin; i++ {
			out, vector = append(out, []Value{float64(i + 1)}), append(vector, float64(i+1))
		}
		if fila {
			return out
		}
		return Matrix{vector}
	}
}

func columna(m Matrix, c int) Matrix {
	out := make(Matrix, len(m))
	for r := range m {
		out[r] = []Value{en(m, r, c, "")}
	}
	return out
}

// funcionesBusqueda es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesBusqueda = map[string]fn{
	"VLOOKUP": buscarEn(true),
	"HLOOKUP": buscarEn(false),
	// XLOOKUP(valor, buscar_en, devolver, [si_no], [modo=0], [sentido=1]).
	"XLOOKUP": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 6 {
			return ErrValue
		}
		lookup := args[0].Value()
		if IsError(lookup) {
			return lookup
		}
		la, ra := args[1].Matrix(), args[2].Matrix()
		lr, lc := dims(la)
		vec := flatten(la)
		modo, err := intOpc(args, 4, 0)
		if err != "" {
			return err
		}
		sentido, err := intOpc(args, 5, 1)
		if err != "" {
			return err
		}
		idx := xfind(vec, lookup, modo, sentido)
		rr, rc := dims(ra)
		switch {
		case idx < 0 && len(args) >= 4:
			return args[3].Value() // perezoso
		case idx < 0:
			return ErrNA
		case lr == 1 && lc > 1: // horizontal: la columna idx, en vertical
			if rc != len(vec) {
				return ErrValue
			}
			return columna(ra, idx)
		case rr != len(vec):
			return ErrValue
		}
		return Matrix{append([]Value(nil), ra[idx]...)}
	},
	"INDEX": index,
	// MATCH(valor, vector, [tipo=1]): 0 exacta, 1 mayor ≤ valor (asc), -1 menor ≥ valor.
	"MATCH": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		lookup := args[0].Value()
		if IsError(lookup) {
			return lookup
		}
		vec, ok := unaDimension(args[1].Matrix())
		if !ok {
			return ErrNA
		}
		tipo, err := intOpc(args, 2, 1)
		switch {
		case err != "":
			return err
		case tipo == 0:
			return posicion(exactIndex(vec, lookup))
		case tipo > 0:
			return posicion(approx(vec, lookup, 1))
		}
		return posicion(approx(vec, lookup, -1))
	},
	"XMATCH": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 4 {
			return ErrValue
		}
		lookup := args[0].Value()
		if IsError(lookup) {
			return lookup
		}
		vec, ok := unaDimension(args[1].Matrix())
		if !ok {
			return ErrNA
		}
		modo, err := intOpc(args, 2, 0)
		if err != "" {
			return err
		}
		sentido, err := intOpc(args, 3, 1)
		if err != "" {
			return err
		}
		return posicion(xfind(vec, lookup, modo, sentido))
	},
	// LOOKUP: forma vectorial, o de matriz (busca en la primera fila o columna, según
	// sea más ancha o alta, y devuelve la última). Siempre aproximada ascendente.
	"LOOKUP": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		lookup := args[0].Value()
		if IsError(lookup) {
			return lookup
		}
		m := args[1].Matrix()
		filas, cols := dims(m)
		if len(args) == 3 {
			idx := approx(flatten(m), lookup, 1)
			if devolver := flatten(args[2].Matrix()); idx >= 0 && idx < len(devolver) {
				return devolver[idx]
			}
			return ErrNA
		}
		if cols > filas {
			if idx := approx(fila0(m), lookup, 1); idx >= 0 {
				return en(m, filas-1, idx, "")
			}
			return ErrNA
		}
		if idx := approx(primeraCol(m), lookup, 1); idx >= 0 {
			return en(m, idx, cols-1, "")
		}
		return ErrNA
	},
	// CHOOSE devuelve la MATRIZ del elegido, para poder pasar un rango entero.
	"CHOOSE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 {
			return ErrValue
		}
		i, err := intOpc(args, 0, 0)
		if err != "" {
			return err
		}
		if i < 1 || i > float64(len(args)-1) {
			return ErrValue
		}
		return args[int(i)].Matrix()
	},
	// OFFSET(ref, filas, cols, [alto], [ancho]): el primer argumento debe ser una referencia.
	"OFFSET": func(c *FnCtx, args []*Arg) any {
		if len(args) < 3 || len(args) > 5 || args[0].Ref() == nil {
			return ErrValue
		}
		base := args[0].Ref()
		v := [4]float64{0, 0, float64(base.R2 - base.R1 + 1), float64(base.C2 - base.C1 + 1)}
		for i := range v {
			n, err := intOpc(args, i+1, v[i])
			if err != "" {
				return err
			}
			v[i] = n
		}
		r1, c1 := float64(base.R1)+v[0], float64(base.C1)+v[1]
		if !(v[2] >= 1) || !(v[3] >= 1) || !(r1 >= 0) || !(c1 >= 0) || r1+v[2] > 1<<21 || c1+v[3] > 1<<15 {
			return ErrRef
		}
		return c.Resolve(RefRange{base.Sheet, int(c1), int(r1), int(c1 + v[3] - 1), int(r1 + v[2] - 1)})
	},
	// INDIRECT(texto, [a1=TRUE]). ponytail (del original): solo estilo A1; la
	// dependencia es dinámica y el grafo estático no la rastrea.
	"INDIRECT": func(c *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		t, err := CoerceText(args[0].Value())
		if err != "" {
			return err
		}
		if len(args) == 2 {
			a1, err := CoerceBoolean(args[1].Value())
			if err != "" {
				return err
			}
			if !a1 {
				return ErrRef
			}
		}
		if rr := c.ParseRef(jsTrim(t)); rr != nil {
			return c.Resolve(*rr)
		}
		return ErrRef
	},
	"ROW":     filaOColumna(true),
	"COLUMN":  filaOColumna(false),
	"ROWS":    medida(func(r *RefRange) int { return r.R2 - r.R1 + 1 }, func(filas, _ int) int { return filas }),
	"COLUMNS": medida(func(r *RefRange) int { return r.C2 - r.C1 + 1 }, func(_, cols int) int { return cols }),
	"TRANSPOSE": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		m := args[0].Matrix()
		filas, cols := dims(m)
		if filas == 0 || cols == 0 {
			return ErrValue
		}
		out := make(Matrix, cols)
		for c := range out {
			out[c] = make([]Value, filas)
			for r := range out[c] {
				out[c][r] = en(m, r, c, "")
			}
		}
		return out
	},
	"ADDRESS": address,
	// El VALOR de la celda es el nombre visible; el enlace lo pinta el editor.
	"HYPERLINK": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 2 {
			return ErrValue
		}
		t, err := textos(args, len(args))
		if err != "" {
			return err
		}
		if len(t) == 2 && t[1] != "" {
			return t[1]
		}
		return t[0]
	},
}

func init() { registrar(funcionesBusqueda) }

// index es INDEX(matriz, fila, [col]), 1-based. Fila o columna 0 devuelve la columna o la
// fila entera; en una matriz de una sola fila o columna el único índice la recorre.
func index(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 || len(args) > 3 {
		return ErrValue
	}
	m := args[0].Matrix()
	filas, cols := dims(m)
	if filas == 0 || cols == 0 {
		return ErrRef
	}
	ri, err := intOpc(args, 1, 0)
	if err != "" {
		return err
	}
	ci := 0.0 // 2D con la columna omitida: la fila entera
	switch {
	case len(args) == 3:
		if ci, err = intOpc(args, 2, 0); err != "" {
			return err
		}
	case filas == 1:
		ci, ri = ri, 1 // fila única: el índice es la columna
	case cols == 1:
		ci = 1
	}
	switch {
	case ri < 0 || ci < 0 || math.IsNaN(ri+ci):
		return ErrValue
	case ri > float64(filas) || ci > float64(cols):
		return ErrRef
	case ri == 0 && ci == 0:
		return m
	case ri == 0:
		return columna(m, int(ci)-1)
	case ci == 0:
		return Matrix{append([]Value(nil), m[int(ri)-1]...)}
	}
	return en(m, int(ri)-1, int(ci)-1, "")
}

// address es ADDRESS(fila, col, [abs=1], [a1=TRUE], [hoja]). abs: 1=$A$1, 2=A$1, 3=$A1,
// 4=A1; a1=FALSE da estilo R1C1.
func address(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 || len(args) > 5 {
		return ErrValue
	}
	v, err := numArgs(args, 2)
	if err != "" {
		return err
	}
	r, c := math.Trunc(v[0]), math.Trunc(v[1])
	if r < 1 || c < 1 {
		return ErrValue
	}
	abs, err := intOpc(args, 2, 1)
	if err != "" {
		return err
	}
	if abs < 1 || abs > 4 {
		return ErrValue
	}
	a1 := true
	if len(args) >= 4 {
		if a1, err = CoerceBoolean(args[3].Value()); err != "" {
			return err
		}
	}
	filaAbs, colAbs := abs == 1 || abs == 2, abs == 1 || abs == 3
	marca := func(abs bool, a, b string) string {
		if abs {
			return a
		}
		return b
	}
	fila, col := jsNumero(r), jsNumero(c)
	dir := marca(colAbs, "$", "") + NumToCol(int(math.Min(c, 1<<30))-1) + marca(filaAbs, "$", "") + fila
	if !a1 {
		dir = marca(filaAbs, "R"+fila, "R["+fila+"]") + marca(colAbs, "C"+col, "C["+col+"]")
	}
	if len(args) == 5 {
		hoja, err := CoerceText(args[4].Value())
		if err != "" {
			return err
		}
		if hoja != "" {
			dir = quoteSheet(hoja) + "!" + dir
		}
	}
	return dir
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"slices"
	"sort"
	"strings"
)

// Arrays dinámicos, álgebra matricial y remodelado (functions/array.ts). Devuelven una
// MATRIZ que el motor DERRAMA; en contexto escalar la colapsa. LET y las de orden
// superior viven en el motor (necesitan el AST).

// topeCeldas es el tope defensivo de tamaño de SEQUENCE, RANDARRAY y MUNIT.
const topeCeldas = 1_000_000

// topeInt es a lo que satura aInt: 2^40 donde quepa y el mayor int donde no —en un
// Android de 32 bits, que es donde se vio, `1 << 40` ni siquiera compila—. Solo tiene
// que quedar MUY por encima de topeCeldas, que es quien rechaza de verdad.
const topeInt = min(1<<40, math.MaxInt)

// aInt convierte un float ya validado a int sin el comportamiento indefinido de Go con
// NaN e infinitos.
func aInt(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f > topeInt:
		return topeInt
	case f < -topeInt:
		return -topeInt
	}
	return int(f)
}

// boolOpc es un argumento booleano opcional.
func boolOpc(args []*Arg, i int, def bool) (bool, Error) {
	if i >= len(args) {
		return def, ""
	}
	return CoerceBoolean(args[i].Value())
}

// valKey es la clave estable de un valor para UNIQUE: tipo y contenido (el texto, sin
// distinguir mayúsculas, como la hoja).
func valKey(v Value) string {
	switch x := v.(type) {
	case Error:
		return "e:" + string(x)
	case float64:
		return "n:" + jsNumero(x)
	case bool:
		if x {
			return "b:true"
		}
		return "b:false"
	case string:
		return "s:" + jsUpper(x)
	}
	return ""
}

func rowKey(fila []Value) string {
	claves := make([]string, len(fila))
	for i, v := range fila {
		claves[i] = valKey(v)
	}
	return strings.Join(claves, "\x01")
}

// cmpOrden es el orden de hoja con los errores AL FINAL en los dos sentidos.
func cmpOrden(a, b Value, dir int) int {
	ea, eb := IsError(a), IsError(b)
	switch {
	case ea && eb:
		return 0
	case ea:
		return 1
	case eb:
		return -1
	}
	c, _ := CompareValues(a, b)
	return c * dir
}

func transpose(m Matrix) Matrix {
	filas, cols := dims(m)
	out := make(Matrix, cols)
	for c := range out {
		out[c] = make([]Value, filas)
		for r := range out[c] {
			out[c][r] = en(m, r, c, "")
		}
	}
	return out
}

func copiaFila(fila []Value) []Value { return append([]Value(nil), fila...) }

/* ---------- Álgebra matricial ----------
 * Las eliminaciones van con float64(...) explícito en el producto: impide que el
 * compilador lo funda en una FMA (en arm64 lo hace) y que un pivote salga «casi cero» en
 * una máquina y cero en otra, que es la diferencia entre #NUM! y una inversa basura.
 */

// numMatrix es la matriz de un argumento coaccionada (blanco → 0; texto → #VALUE!).
func numMatrix(a *Arg) ([][]float64, Error) {
	var out [][]float64
	for _, fila := range a.Matrix() {
		r := make([]float64, 0, len(fila))
		for _, v := range fila {
			if e, ok := v.(Error); ok {
				return nil, e
			}
			n, err := CoerceNumber(v)
			if err != "" {
				return nil, ErrValue
			}
			r = append(r, n)
		}
		out = append(out, r)
	}
	return out, ""
}

func isSquare(m [][]float64) bool {
	for _, r := range m {
		if len(r) != len(m) {
			return false
		}
	}
	return len(m) > 0
}

// pivote es la fila con el mayor |valor| en la columna i, de la fila i hacia abajo.
func pivote(m [][]float64, i int) int {
	p := i
	for r := i + 1; r < len(m); r++ {
		if math.Abs(m[r][i]) > math.Abs(m[p][i]) {
			p = r
		}
	}
	return p
}

// pairedNums son los pares (x, y) alineados por posición; longitudes distintas → #N/A; se
// descarta el par si alguna celda no es número.
func pairedNums(ax, ay *Arg) (xs, ys []float64, err Error) {
	fx, fy := flatten(ax.Matrix()), flatten(ay.Matrix())
	if len(fx) != len(fy) {
		return nil, nil, ErrNA
	}
	for i := range fx {
		if e, ok := fx[i].(Error); ok {
			return nil, nil, e
		}
		if e, ok := fy[i].(Error); ok {
			return nil, nil, e
		}
		x, okX := fx[i].(float64)
		y, okY := fy[i].(float64)
		if okX && okY {
			xs, ys = append(xs, x), append(ys, y)
		}
	}
	return xs, ys, ""
}

// sumaPares es SUMX2MY2, SUMX2PY2 y SUMXMY2.
func sumaPares(termino func(x, y float64) float64) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		xs, ys, err := pairedNums(args[0], args[1])
		if err != "" {
			return err
		}
		s := 0.0
		for i := range xs {
			s += termino(xs[i], ys[i])
		}
		return s
	}
}

/* ---------- Remodelado ---------- */

// resolveIndex: índice 1-based (negativo = desde el final) → 0-based; -1 si está fuera.
func resolveIndex(k float64, total int) int {
	switch t := float64(total); {
	case k > 0 && k <= t:
		return int(k) - 1
	case k < 0 && -k <= t:
		return total + int(k)
	}
	return -1
}

// takeRange es el tramo [inicio, fin) de TAKE(n); dropRange, el de DROP(n).
func takeRange(n float64, total int) (int, int) {
	if n >= 0 {
		return 0, aInt(math.Min(n, float64(total)))
	}
	return aInt(math.Max(0, float64(total)+n)), total
}

func dropRange(n float64, total int) (int, int) {
	if n >= 0 {
		return aInt(math.Min(n, float64(total))), total
	}
	return 0, aInt(math.Max(0, float64(total)+n))
}

// recortar es TAKE y DROP: la submatriz, o #CALC! si queda vacía.
func recortar(tramo func(n float64, total int) (int, int)) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		m := args[0].Matrix()
		filas, cols := dims(m)
		rn, err := numArg(args, 1)
		if err != "" {
			return err
		}
		c0, c1 := 0, cols
		if len(args) == 3 {
			cn, err := numArg(args, 2)
			if err != "" {
				return err
			}
			c0, c1 = tramo(math.Trunc(cn), cols)
		}
		r0, r1 := tramo(math.Trunc(rn), filas)
		if r1 <= r0 || c1 <= c0 {
			return ErrCalc
		}
		out := make(Matrix, 0, r1-r0)
		for i := r0; i < r1; i++ {
			fila := make([]Value, 0, c1-c0)
			for j := c0; j < c1; j++ {
				fila = append(fila, en(m, i, j, ""))
			}
			out = append(out, fila)
		}
		return out
	}
}

// relleno lee el argumento de relleno opcional (#N/A si falta); un error explícito se propaga.
func relleno(args []*Arg, i int) (Value, Error) {
	if i >= len(args) {
		return ErrNA, ""
	}
	v := args[i].Value()
	if e, ok := v.(Error); ok {
		return nil, e
	}
	return v, ""
}

// toLine es TOROW y TOCOL: aplana con `ignorar` (1 blancos, 2 errores, 3 ambos) y el orden.
func toLine(columna bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 3 {
			return ErrValue
		}
		ignorar, err := intOpc(args, 1, 0)
		if err != "" {
			return err
		}
		if ignorar < 0 || ignorar > 3 {
			return ErrValue
		}
		porColumnas, err := boolOpc(args, 2, false)
		if err != "" {
			return err
		}
		m := args[0].Matrix()
		if porColumnas {
			m = transpose(m)
		}
		var vals []Value
		for _, fila := range m {
			for _, v := range fila {
				if ((ignorar == 1 || ignorar == 3) && v == "") || ((ignorar == 2 || ignorar == 3) && IsError(v)) {
					continue
				}
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			return ErrCalc
		}
		if !columna {
			return Matrix{vals}
		}
		return transpose(Matrix{vals})
	}
}

// wrap es WRAPROWS y WRAPCOLS: envuelve un vector en líneas de `cuenta`.
func wrap(porFilas bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 || len(args) > 3 {
			return ErrValue
		}
		m := args[0].Matrix()
		if filas, cols := dims(m); filas != 1 && cols != 1 {
			return ErrValue // debe ser un vector
		}
		cuenta, err := intOpc(args, 1, 1)
		if err != "" {
			return err
		}
		if cuenta < 1 {
			return ErrNum
		}
		pad, err := relleno(args, 2)
		if err != "" {
			return err
		}
		vals := flatten(m)
		if cuenta > topeCeldas {
			return ErrNum // ponytail: el original no tiene tope aquí; una línea de 1e9 celdas cuelga la pestaña
		}
		ancho := aInt(cuenta)
		var filas Matrix
		for i := 0; i < len(vals); i += ancho {
			trozo := make([]Value, ancho)
			for j := range trozo {
				if trozo[j] = pad; i+j < len(vals) {
					trozo[j] = vals[i+j]
				}
			}
			filas = append(filas, trozo)
		}
		if porFilas {
			return filas
		}
		return transpose(filas)
	}
}

// elegir es CHOOSEROWS y CHOOSECOLS.
func elegir(filas bool) fn {
	return func(_ *FnCtx, args []*Arg) any {
		if len(args) < 2 {
			return ErrValue
		}
		m := args[0].Matrix()
		if !filas {
			m = transpose(m)
		}
		var out Matrix
		for _, a := range args[1:] {
			for _, fila := range a.Matrix() {
				for _, v := range fila {
					n, err := CoerceNumber(v)
					if err != "" {
						return err
					}
					idx := resolveIndex(math.Trunc(n), len(m))
					if idx < 0 {
						return ErrValue
					}
					out = append(out, copiaFila(m[idx]))
				}
			}
		}
		switch {
		case len(out) == 0:
			return ErrCalc
		case filas:
			return out
		}
		return transpose(out)
	}
}

// forma lee filas y columnas (enteros, 1 por defecto) de SEQUENCE y RANDARRAY.
func forma(args []*Arg) (filas, cols float64, err Error) {
	if filas, err = intOpc(args, 0, 1); err != "" {
		return filas, 0, err
	}
	cols, err = intOpc(args, 1, 1)
	return filas, cols, err
}

// funcionesArrayReg es el registro de funciones de este fichero. Es un MAPA y no un init():
// el cuerpo de cada función vive dentro del literal, y medido como UNA función
// lo que sale es la suma de todos ellos, que no es una medida de nada.
var funcionesArrayReg = map[string]fn{
	"SEQUENCE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 4 {
			return ErrValue
		}
		filas, cols, err := forma(args)
		if err != "" {
			return err
		}
		inicio, err := numOpc(args, 2, 1)
		if err != "" {
			return err
		}
		paso, err := numOpc(args, 3, 1)
		switch {
		case err != "":
			return err
		case filas < 1 || cols < 1:
			return ErrValue
		case filas*cols > topeCeldas:
			return ErrNum
		}
		out, v := make(Matrix, aInt(filas)), inicio
		for r := range out {
			out[r] = make([]Value, aInt(cols))
			for c := range out[r] {
				out[r][c] = v
				v += paso
			}
		}
		return out
	},
	"RANDARRAY": func(ctx *FnCtx, args []*Arg) any {
		if len(args) > 5 {
			return ErrValue
		}
		filas, cols, err := forma(args)
		if err != "" {
			return err
		}
		lo, err := numOpc(args, 2, 0)
		if err != "" {
			return err
		}
		hi, err := numOpc(args, 3, 1)
		if err != "" {
			return err
		}
		entero, err := boolOpc(args, 4, false)
		switch {
		case err != "":
			return err
		case filas < 1 || cols < 1 || lo > hi:
			return ErrValue
		case filas*cols > topeCeldas:
			return ErrNum
		}
		out := make(Matrix, aInt(filas))
		for r := range out {
			out[r] = make([]Value, aInt(cols))
			for c := range out[r] {
				if u := ctx.Random(); entero {
					out[r][c] = math.Floor(u*(hi-lo+1)) + lo
				} else {
					out[r][c] = lo + u*(hi-lo)
				}
			}
		}
		return out
	},
	// UNIQUE(array, [por_columnas], [exactamente_una_vez]).
	"UNIQUE": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 3 {
			return ErrValue
		}
		m := args[0].Matrix()
		porColumnas, err := boolOpc(args, 1, false)
		if err != "" {
			return err
		}
		unaVez, err := boolOpc(args, 2, false)
		if err != "" {
			return err
		}
		if porColumnas {
			m = transpose(m)
		}
		veces := map[string]int{}
		for _, fila := range m {
			veces[rowKey(fila)]++
		}
		var out Matrix
		for _, fila := range m {
			k := rowKey(fila)
			if n := veces[k]; n > 0 && (!unaVez || n == 1) {
				out = append(out, copiaFila(fila))
			}
			veces[k] = 0 // ya vista
		}
		switch {
		case len(out) == 0:
			return ErrCalc
		case porColumnas:
			return transpose(out)
		}
		return out
	},
	// SORT(array, [índice=1], [orden=1], [por_columnas]). Estable, como el de JavaScript.
	"SORT": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 || len(args) > 4 {
			return ErrValue
		}
		m := args[0].Matrix()
		idx, err := intOpc(args, 1, 1)
		if err != "" {
			return err
		}
		sentido, err := intOpc(args, 2, 1)
		if err != "" {
			return err
		}
		porColumnas, err := boolOpc(args, 3, false)
		if err != "" {
			return err
		}
		dir := 1
		if sentido < 0 {
			dir = -1
		}
		if porColumnas {
			m = transpose(m)
		}
		if _, ancho := dims(m); idx < 1 || idx > float64(ancho) {
			return ErrValue
		}
		filas := make(Matrix, len(m))
		for i, f := range m {
			filas[i] = copiaFila(f)
		}
		k := int(idx) - 1
		sort.SliceStable(filas, func(a, b int) bool { return cmpOrden(en(filas, a, k, ""), en(filas, b, k, ""), dir) < 0 })
		if porColumnas {
			return transpose(filas)
		}
		return filas
	},
	"SORTBY": sortby,
	"FILTER": filter,
	"MMULT": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 2 {
			return ErrValue
		}
		a, err := numMatrix(args[0])
		if err != "" {
			return err
		}
		b, err := numMatrix(args[1])
		if err != "" {
			return err
		}
		ca, cb := 0, 0
		if len(a) > 0 {
			ca = len(a[0])
		}
		if len(b) > 0 {
			cb = len(b[0])
		}
		if ca == 0 || ca != len(b) {
			return ErrValue
		}
		out := make(Matrix, len(a))
		for i := range out {
			out[i] = make([]Value, cb)
			for j := range out[i] {
				s := 0.0
				for k := 0; k < ca; k++ {
					s += float64(a[i][k] * b[k][j])
				}
				out[i][j] = finito(s)
			}
		}
		return out
	},
	"MDETERM": func(_ *FnCtx, args []*Arg) any {
		if len(args) != 1 {
			return ErrValue
		}
		m, err := numMatrix(args[0])
		if err != "" {
			return err
		}
		if !isSquare(m) {
			return ErrValue
		}
		det := 1.0
		for i := range m {
			p := pivote(m, i)
			if m[p][i] == 0 {
				return 0.0
			}
			if p != i {
				m[i], m[p] = m[p], m[i]
				det = -det
			}
			det *= m[i][i]
			for r := i + 1; r < len(m); r++ {
				f := m[r][i] / m[i][i]
				for c := i; c < len(m); c++ {
					m[r][c] -= float64(f * m[i][c])
				}
			}
		}
		return finito(det)
	},
	"MINVERSE": minverse,
	"MUNIT": func(_ *FnCtx, args []*Arg) any {
		nv, err := arg1(args)
		if err != "" {
			return err
		}
		n := math.Trunc(nv)
		if n < 1 {
			return ErrValue
		}
		if n*n > topeCeldas {
			return ErrNum
		}
		out := make(Matrix, aInt(n))
		for i := range out {
			out[i] = make([]Value, aInt(n))
			for j := range out[i] {
				out[i][j] = b2f(i == j)
			}
		}
		return out
	},
	"SUMX2MY2": sumaPares(func(x, y float64) float64 { return float64(x*x) - float64(y*y) }),
	"SUMX2PY2": sumaPares(func(x, y float64) float64 { return float64(x*x) + float64(y*y) }),
	"SUMXMY2":  sumaPares(func(x, y float64) float64 { return (x - y) * (x - y) }),
	// Las matrices más cortas se rellenan con #N/A.
	"HSTACK": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 {
			return ErrValue
		}
		mats, filas := make([]Matrix, len(args)), 0
		for i, a := range args {
			mats[i] = a.Matrix()
			filas = max(filas, len(mats[i]))
		}
		out := make(Matrix, filas)
		for r := range out {
			for _, m := range mats {
				_, cols := dims(m)
				for c := 0; c < cols; c++ {
					if r < len(m) {
						out[r] = append(out[r], en(m, r, c, ""))
					} else {
						out[r] = append(out[r], ErrNA)
					}
				}
			}
		}
		return out
	},
	"VSTACK": func(_ *FnCtx, args []*Arg) any {
		if len(args) < 1 {
			return ErrValue
		}
		mats, cols := make([]Matrix, len(args)), 0
		for i, a := range args {
			mats[i] = a.Matrix()
			_, c := dims(mats[i])
			cols = max(cols, c)
		}
		var out Matrix
		for _, m := range mats {
			for r := range m {
				fila := make([]Value, cols)
				for c := range fila {
					fila[c] = en(m, r, c, ErrNA)
				}
				out = append(out, fila)
			}
		}
		return out
	},
	"TOROW":      toLine(false),
	"TOCOL":      toLine(true),
	"WRAPROWS":   wrap(true),
	"WRAPCOLS":   wrap(false),
	"TRIMRANGE":  trimRange,
	"TAKE":       recortar(takeRange),
	"DROP":       recortar(dropRange),
	"EXPAND":     expand,
	"CHOOSEROWS": elegir(true),
	"CHOOSECOLS": elegir(false),
}

func init() { registrar(funcionesArrayReg) }

// trimRange es TRIMRANGE(rango, [recortar_filas=3], [recortar_columnas=3]): quita las filas
// y columnas en blanco de los BORDES (0 nada, 1 las iniciales, 2 las finales, 3 ambas). Solo
// los bordes: una fila en blanco en medio se queda. Todo en blanco → #CALC!.
func trimRange(_ *FnCtx, args []*Arg) any {
	if len(args) < 1 || len(args) > 3 {
		return ErrValue
	}
	m := args[0].Matrix()
	var modos [2]int
	for i := range modos {
		modos[i] = 3
		if i+1 < len(args) {
			n, err := numArg(args, i+1)
			if err != "" {
				return err
			}
			if modos[i] = aInt(math.Trunc(n)); modos[i] < 0 || modos[i] > 3 {
				return ErrValue
			}
		}
	}
	filas, cols := dims(m)
	vacia := func(v Value) bool { return v == nil || v == "" }
	r1, r2 := recortarBordes(filas, modos[0], func(r int) bool { return slices.IndexFunc(m[r], func(v Value) bool { return !vacia(v) }) < 0 })
	c1, c2 := recortarBordes(cols, modos[1], func(c int) bool {
		return !slices.ContainsFunc(m, func(f []Value) bool { return c < len(f) && !vacia(f[c]) })
	})
	if r1 >= r2 || c1 >= c2 {
		return ErrCalc
	}
	out := make(Matrix, 0, r2-r1)
	for i := r1; i < r2; i++ {
		fila := make([]Value, 0, c2-c1)
		for j := c1; j < c2; j++ {
			fila = append(fila, en(m, i, j, ""))
		}
		out = append(out, fila)
	}
	return out
}

// recortarBordes es el tramo [ini, fin) de `total` posiciones que queda al quitar las
// vacías del principio (modo&1) y del final (modo&2).
func recortarBordes(total, modo int, vacia func(int) bool) (ini, fin int) {
	fin = total
	if modo&1 != 0 {
		for ini < fin && vacia(ini) {
			ini++
		}
	}
	if modo&2 != 0 {
		for fin > ini && vacia(fin-1) {
			fin--
		}
	}
	return ini, fin
}

// sortby es SORTBY(array, por1, [orden1], por2, [orden2], …).
func sortby(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 {
		return ErrValue
	}
	m := args[0].Matrix()
	type llave struct {
		vec []Value
		dir int
	}
	var llaves []llave
	for i := 1; i < len(args); i += 2 {
		por := args[i].Matrix()
		vec := fila0(por)
		if len(por) == len(m) {
			vec = primeraCol(por)
		}
		if len(vec) != len(m) {
			return ErrValue
		}
		dir := 1
		if i+1 < len(args) {
			o, err := CoerceNumber(args[i+1].Value())
			if err != "" {
				return err
			}
			if o < 0 {
				dir = -1
			}
		}
		llaves = append(llaves, llave{vec, dir})
	}
	idxs := make([]int, len(m))
	for i := range idxs {
		idxs[i] = i
	}
	sort.SliceStable(idxs, func(a, b int) bool {
		for _, k := range llaves {
			if c := cmpOrden(k.vec[idxs[a]], k.vec[idxs[b]], k.dir); c != 0 {
				return c < 0
			}
		}
		return false
	})
	out := make(Matrix, len(m))
	for i, idx := range idxs {
		out[i] = copiaFila(m[idx])
	}
	return out
}

// filter es FILTER(array, incluir, [si_vacío]): `incluir` es una columna tan alta como el
// array (filtra filas) o una fila tan ancha (filtra columnas).
func filter(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 || len(args) > 3 {
		return ErrValue
	}
	m, inc := args[0].Matrix(), args[1].Matrix()
	filas, cols := dims(m)
	incR, incC := dims(inc)
	porFilas := incC == 1 && incR == filas
	if !porFilas && (incR != 1 || incC != cols) {
		return ErrValue // dimensiones incompatibles
	}
	if !porFilas {
		m, inc = transpose(m), transpose(inc)
	}
	var out Matrix
	for r := range m {
		v := en(inc, r, 0, "")
		if e, ok := v.(Error); ok {
			return e
		}
		quedarse, err := CoerceBoolean(v)
		if err != "" {
			return err
		}
		if quedarse {
			out = append(out, copiaFila(m[r]))
		}
	}
	if len(out) == 0 || len(out[0]) == 0 {
		if len(args) == 3 {
			return args[2].Value()
		}
		return ErrCalc
	}
	if !porFilas {
		return transpose(out)
	}
	return out
}

// minverse es la inversa por Gauss-Jordan con pivotaje parcial; singular → #NUM!.
func minverse(_ *FnCtx, args []*Arg) any {
	if len(args) != 1 {
		return ErrValue
	}
	a, err := numMatrix(args[0])
	if err != "" {
		return err
	}
	if !isSquare(a) {
		return ErrValue
	}
	n := len(a)
	m := make([][]float64, n)
	for i := range m {
		m[i] = append(append([]float64(nil), a[i]...), make([]float64, n)...)
		m[i][n+i] = 1
	}
	for i := range m {
		p := pivote(m, i)
		if m[p][i] == 0 {
			return ErrNum
		}
		m[i], m[p] = m[p], m[i]
		piv := m[i][i]
		for c := range m[i] {
			m[i][c] /= piv
		}
		for r := range m {
			if r == i {
				continue
			}
			f := m[r][i]
			for c := range m[r] {
				m[r][c] -= float64(f * m[i][c])
			}
		}
	}
	out := make(Matrix, n)
	for i := range out {
		out[i] = make([]Value, n)
		for j := range out[i] {
			if !esFinito(m[i][n+j]) {
				return ErrNum
			}
			out[i][j] = m[i][n+j]
		}
	}
	return out
}

// expand es EXPAND(array, filas, [columnas], [relleno]).
func expand(_ *FnCtx, args []*Arg) any {
	if len(args) < 2 || len(args) > 4 {
		return ErrValue
	}
	m := args[0].Matrix()
	filas, cols := dims(m)
	aFilas, err := intOpc(args, 1, 0)
	if err != "" {
		return err
	}
	aCols, err := intOpc(args, 2, float64(cols))
	if err != "" {
		return err
	}
	if aFilas < float64(filas) || aCols < float64(cols) || aFilas < 1 || aCols < 1 {
		return ErrValue
	}
	pad, err := relleno(args, 3)
	if err != "" {
		return err
	}
	if aFilas*aCols > topeCeldas {
		return ErrNum // ponytail: el original no tiene tope aquí y un EXPAND(A1;1E9) cuelga la pestaña
	}
	out := make(Matrix, aInt(aFilas))
	for r := range out {
		out[r] = make([]Value, aInt(aCols))
		for c := range out[r] {
			if out[r][c] = pad; r < filas && c < cols {
				out[r][c] = en(m, r, c, "")
			}
		}
	}
	return out
}

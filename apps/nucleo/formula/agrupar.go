// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// GROUPBY y PIVOTBY (functions/agrupar.ts): agrupar las filas de una matriz por uno o
// varios campos y resumir cada grupo con una función. El motor los despacha aparte del
// registro porque su argumento `función` es un LAMBDA (o el nombre de una).
//
// Mismas decisiones que el original:
//   - Las etiquetas de un grupo se repiten en TODAS sus filas (tabla plana).
//   - Subtotales (total ±2) solo con más de un campo; con uno es igual a ±1.
//   - PIVOTBY resume UNA columna de valores y no calcula subtotales de varios campos.
//   - El total se llama «Total»; las cabeceras generadas (modo 2), «Campo n» y «Valor n».

// agregador resume los valores de UN grupo (una columna). `todos` es la misma columna con
// TODAS las filas filtradas: PERCENTOF la necesita como denominador.
type agregador func(grupo, todos Matrix) Value

// opcionesAgrupar son los argumentos de GROUPBY ya evaluados; los punteros nil son los
// omitidos.
type opcionesAgrupar struct {
	filas, valores Matrix
	agregar        agregador
	cabeceras      *int
	totales        *int
	orden          []int
	filtro         Matrix
}

// opcionesPivot son los de PIVOTBY.
type opcionesPivot struct {
	filas, columnas, valores Matrix
	agregar                  agregador
	cabeceras                *int
	totalFilas, totalCols    *int
	ordenFilas, ordenCols    []int
	filtro                   Matrix
}

const etiquetaTotal = "Total"

// claveDe es una clave estable de una tupla de valores: «1» (número) y «1» (texto) no son
// lo mismo.
func claveDe(partes []Value) string {
	var b strings.Builder
	for _, v := range partes {
		switch x := v.(type) {
		case float64:
			b.WriteString("n:" + jsNumero(x))
		case string:
			fmt.Fprintf(&b, "s:%q", x)
		case bool:
			fmt.Fprintf(&b, "b:%t", x)
		case Error:
			b.WriteString("e:" + string(x))
		default:
			fmt.Fprintf(&b, "?:%v", x)
		}
		b.WriteByte(0)
	}
	return b.String()
}

func comparar(a, b Value) int {
	c, _ := CompareValues(a, b)
	return c
}

// hayCabeceras: ¿la primera fila de los valores es texto y la siguiente número? Es el
// criterio de las hojas de cálculo para «hay cabeceras».
func hayCabeceras(valores Matrix) bool {
	a := en(valores, 0, 0, nil)
	b := en(valores, 1, 0, nil)
	s, esTexto := a.(string)
	_, esNum := b.(float64)
	return esTexto && s != "" && esNum
}

type preparado struct {
	muestraCab           bool
	cabFilas, cabValores []Value
	filas, columnas      Matrix
	valores              Matrix
}

// modoCabeceras: el modo pedido o, si se omitió, el detectado; ok=false si no es 0..3.
func modoCabeceras(pedido *int, valores Matrix) (modo int, ok bool) {
	switch {
	case pedido != nil:
		modo = *pedido
	case hayCabeceras(valores):
		modo = 3
	}
	return modo, modo >= 0 && modo <= 3
}

// nombresDeCampos son los nombres de n campos: los de la cabecera si la hay, y si no los
// generados («Campo 1», «Valor 1»…).
func nombresDeCampos(fila []Value, n int, hayCab bool, prefijo string) []Value {
	out := make([]Value, n)
	for i := range out {
		if hayCab && i < len(fila) {
			out[i] = fila[i]
		} else {
			out[i] = fmt.Sprintf("%s %d", prefijo, i+1)
		}
	}
	return out
}

// filaVerdadera: ¿la fila del filtro vale verdadero (un booleano o un número distinto de 0)?
func filaVerdadera(fila []Value) bool {
	switch t := en(Matrix{fila}, 0, 0, nil).(type) {
	case bool:
		return t
	case float64:
		return t != 0
	}
	return false
}

// preparar separa las cabeceras de los datos, aplica el filtro y comprueba las dimensiones.
func preparar(filas, columnas, valores Matrix, cabeceras *int, filtro Matrix) (*preparado, Error) {
	if len(filas) == 0 || len(valores) == 0 {
		return nil, ErrValue
	}
	if len(filas) != len(valores) || (columnas != nil && len(columnas) != len(valores)) || (filtro != nil && len(filtro) != len(valores)) {
		return nil, ErrValue
	}
	modo, ok := modoCabeceras(cabeceras, valores)
	if !ok {
		return nil, ErrValue
	}
	hayCab := modo == 1 || modo == 3
	p := &preparado{
		muestraCab: modo == 2 || modo == 3,
		cabFilas:   nombresDeCampos(filas[0], len(filas[0]), hayCab, "Campo"),
		cabValores: nombresDeCampos(valores[0], len(valores[0]), hayCab, "Valor"),
	}
	resto := func(x Matrix) Matrix {
		if hayCab && x != nil {
			return x[1:]
		}
		return x
	}
	f, c, v, bueno := resto(filas), resto(columnas), resto(valores), resto(filtro)
	if filtro != nil {
		var f2, c2, v2 Matrix
		for i, r := range bueno {
			if !filaVerdadera(r) {
				continue
			}
			f2, v2 = append(f2, f[i]), append(v2, v[i])
			if c != nil {
				c2 = append(c2, c[i])
			}
		}
		f, c, v = f2, c2, v2
	}
	p.filas, p.columnas, p.valores = f, c, v
	return p, ""
}

// columnaDe son los valores de la columna j de las filas idx, como una columna n×1.
func columnaDe(valores Matrix, idx []int, j int) Matrix {
	out := make(Matrix, len(idx))
	for n, i := range idx {
		out[n] = []Value{en(valores, i, j, "")}
	}
	return out
}

// grupo es un conjunto de filas con la misma clave.
type grupoFilas struct {
	clave []Value
	idx   []int
}

func agruparPor(campos Matrix) []*grupoFilas {
	var out []*grupoFilas
	mapa := map[string]*grupoFilas{}
	for i, fila := range campos {
		id := claveDe(fila)
		if g, ok := mapa[id]; ok {
			g.idx = append(g.idx, i)
			continue
		}
		g := &grupoFilas{clave: fila, idx: []int{i}}
		mapa[id] = g
		out = append(out, g)
	}
	return out
}

// ordenarGrupos ordena por las columnas pedidas de la salida: las ≤ k son partes de la
// clave; las demás, un valor ya resumido.
func ordenarGrupos(grupos []*grupoFilas, k int, orden []int, resumen func(*grupoFilas) []Value) {
	crit := orden
	if len(crit) == 0 {
		for i := 1; i <= k; i++ {
			crit = append(crit, i)
		}
	}
	cache := map[*grupoFilas][]Value{}
	valoresDe := func(g *grupoFilas) []Value {
		r, ok := cache[g]
		if !ok {
			r = resumen(g)
			cache[g] = r
		}
		return r
	}
	sort.SliceStable(grupos, func(a, b int) bool {
		for _, c := range crit {
			j := absInt(c) - 1
			var x, y Value
			if j < k {
				x, y = grupos[a].clave[j], grupos[b].clave[j]
			} else {
				x, y = valoresDe(grupos[a])[j-k], valoresDe(grupos[b])[j-k]
			}
			if d := comparar(x, y); d != 0 {
				if c < 0 {
					return d > 0
				}
				return d < 0
			}
		}
		return false
	})
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func ordenValido(orden []int, tope int) bool {
	for _, c := range orden {
		if c == 0 || absInt(c) > tope {
			return false
		}
	}
	return true
}

func todasLasFilas(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// repetir son n celdas en blanco.
func repetir(n int) []Value {
	return slices.Repeat([]Value{""}, max(n, 0))
}

// resumidor resume conjuntos de filas de unos valores con el agregador.
type resumidor struct {
	valores Matrix
	m       int
	agregar agregador
	todas   []int
}

// columnas resume cada columna de valores de las filas idx.
func (r *resumidor) columnas(idx []int) []Value {
	out := make([]Value, r.m)
	for j := range out {
		out[j] = r.agregar(columnaDe(r.valores, idx, j), columnaDe(r.valores, r.todas, j))
	}
	return out
}

// subtotales emite las hojas de `gs` con un «Total» tras cada prefijo de la clave (de fuera
// adentro): arriba de sus hojas si `arriba`, abajo si no.
func (r *resumidor) subtotales(gs []*grupoFilas, nivel, k int, arriba bool) Matrix {
	fila := func(g *grupoFilas) []Value { return append(slices.Clone(g.clave), r.columnas(g.idx)...) }
	var out Matrix
	if nivel == k-1 {
		for _, g := range gs {
			out = append(out, fila(g))
		}
		return out
	}
	var orden []string
	porPrefijo := map[string][]*grupoFilas{}
	for _, g := range gs {
		id := claveDe(g.clave[:nivel+1])
		if _, ok := porPrefijo[id]; !ok {
			orden = append(orden, id)
		}
		porPrefijo[id] = append(porPrefijo[id], g)
	}
	for _, id := range orden {
		lista := porPrefijo[id]
		hijos := r.subtotales(lista, nivel+1, k, arriba)
		var idx []int
		for _, g := range lista {
			idx = append(idx, g.idx...)
		}
		sub := append(slices.Clone(lista[0].clave[:nivel+1]), etiquetaTotal)
		sub = append(sub, repetir(k-nivel-2)...)
		sub = append(sub, r.columnas(idx)...)
		if arriba {
			out = append(append(out, sub), hijos...)
		} else {
			out = append(append(out, hijos...), sub)
		}
	}
	return out
}

func groupBy(o opcionesAgrupar) (Matrix, Error) {
	p, err := preparar(o.filas, nil, o.valores, o.cabeceras, o.filtro)
	if err != "" {
		return nil, err
	}
	k, m := len(p.cabFilas), len(p.cabValores)
	total := 1
	if o.totales != nil {
		total = *o.totales
	}
	if absInt(total) > 2 || !ordenValido(o.orden, k+m) {
		return nil, ErrValue
	}
	if len(p.filas) == 0 {
		return nil, ErrCalc // el filtro lo dejó todo fuera
	}
	r := &resumidor{valores: p.valores, m: m, agregar: o.agregar, todas: todasLasFilas(len(p.filas))}
	grupos := agruparPor(p.filas)
	ordenarGrupos(grupos, k, o.orden, func(g *grupoFilas) []Value { return r.columnas(g.idx) })

	var cuerpo Matrix
	if absInt(total) == 2 && k > 1 {
		cuerpo = r.subtotales(grupos, 0, k, total < 0)
	} else {
		for _, g := range grupos {
			cuerpo = append(cuerpo, append(slices.Clone(g.clave), r.columnas(g.idx)...))
		}
	}
	if total != 0 {
		f := append([]Value{etiquetaTotal}, repetir(k-1)...)
		f = append(f, r.columnas(r.todas)...)
		if total < 0 {
			cuerpo = append(Matrix{f}, cuerpo...)
		} else {
			cuerpo = append(cuerpo, f)
		}
	}
	if !p.muestraCab {
		return cuerpo, ""
	}
	return append(Matrix{append(slices.Clone(p.cabFilas), p.cabValores...)}, cuerpo...), ""
}

// cruce es lo que PIVOTBY necesita para pintar una fila: qué filas de datos caen en cada
// columna y cómo resumirlas.
type cruce struct {
	enCol   map[string]map[int]bool
	resumir func(idx []int) Value
	todas   []int
	kr      int
	cols    []*grupoFilas // nil = la columna «Total»
}

func (c *cruce) fila(gf *grupoFilas) []Value {
	var out []Value
	if gf != nil {
		out = slices.Clone(gf.clave)
	} else {
		out = append([]Value{etiquetaTotal}, repetir(c.kr-1)...)
	}
	for _, gc := range c.cols {
		switch {
		case gf != nil && gc != nil:
			var idx []int
			for _, i := range gf.idx {
				if c.enCol[claveDe(gc.clave)][i] {
					idx = append(idx, i)
				}
			}
			if len(idx) == 0 {
				out = append(out, "")
			} else {
				out = append(out, c.resumir(idx))
			}
		case gf != nil:
			out = append(out, c.resumir(gf.idx)) // total de la fila
		case gc != nil:
			out = append(out, c.resumir(gc.idx)) // total de la columna
		default:
			out = append(out, c.resumir(c.todas)) // total general
		}
	}
	return out
}

// cabeceraPivot es una fila por campo de columna; en la última, los nombres de los campos
// de fila sobre las etiquetas de fila.
func cabeceraPivot(p *preparado, cols []*grupoFilas, kc int) Matrix {
	kr := len(p.cabFilas)
	cab := make(Matrix, 0, kc)
	for i := range kc {
		izq := repetir(kr)
		if i == kc-1 && p.muestraCab {
			izq = slices.Clone(p.cabFilas)
		}
		for _, gc := range cols {
			switch {
			case gc != nil:
				izq = append(izq, gc.clave[i])
			case i == 0:
				izq = append(izq, etiquetaTotal)
			default:
				izq = append(izq, "")
			}
		}
		cab = append(cab, izq)
	}
	return cab
}

// totalOk: un total válido es 0, ±1 o, con un solo campo, ±2.
func totalOk(t, k int) bool { return absInt(t) <= 1 || (absInt(t) == 2 && k == 1) }

func valorOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func pivotBy(o opcionesPivot) (Matrix, Error) {
	p, err := preparar(o.filas, o.columnas, o.valores, o.cabeceras, o.filtro)
	if err != "" {
		return nil, err
	}
	kr, kc := len(p.cabFilas), 0
	if len(o.columnas) > 0 {
		kc = len(o.columnas[0])
	}
	totF, totC := valorOr(o.totalFilas, 1), valorOr(o.totalCols, 1)
	if kc == 0 || len(p.cabValores) != 1 || !totalOk(totF, kr) || !totalOk(totC, kc) ||
		!ordenValido(o.ordenFilas, kr+1) || !ordenValido(o.ordenCols, kc+1) {
		return nil, ErrValue // una columna de valores; sin subtotales de varios campos
	}
	if len(p.filas) == 0 {
		return nil, ErrCalc
	}
	todas := todasLasFilas(len(p.filas))
	todosV := columnaDe(p.valores, todas, 0)
	resumir := func(idx []int) Value { return o.agregar(columnaDe(p.valores, idx, 0), todosV) }

	gFilas, gCols := agruparPor(p.filas), agruparPor(p.columnas)
	// El criterio kr+1 / kc+1 ordena por el total de esa fila / columna.
	ordenarGrupos(gFilas, kr, o.ordenFilas, func(g *grupoFilas) []Value { return []Value{resumir(g.idx)} })
	ordenarGrupos(gCols, kc, o.ordenCols, func(g *grupoFilas) []Value { return []Value{resumir(g.idx)} })

	c := &cruce{enCol: map[string]map[int]bool{}, resumir: resumir, todas: todas, kr: kr, cols: slices.Clone(gCols)}
	for _, g := range gCols {
		set := map[int]bool{}
		for _, i := range g.idx {
			set[i] = true
		}
		c.enCol[claveDe(g.clave)] = set
	}
	switch {
	case totC < 0:
		c.cols = append([]*grupoFilas{nil}, c.cols...)
	case totC > 0:
		c.cols = append(c.cols, nil)
	}
	cuerpo := make(Matrix, 0, len(gFilas)+1)
	for _, g := range gFilas {
		cuerpo = append(cuerpo, c.fila(g))
	}
	switch {
	case totF < 0:
		cuerpo = append(Matrix{c.fila(nil)}, cuerpo...)
	case totF > 0:
		cuerpo = append(cuerpo, c.fila(nil))
	}
	return append(cabeceraPivot(p, c.cols, kc), cuerpo...), ""
}

// lectorArgs lee los argumentos de GROUPBY / PIVOTBY: los omitidos (nVacio) son «no dado».
// El primer error de una opción numérica se guarda en `fallo` y se mira al final.
type lectorArgs struct {
	e     *Engine
	a     []*nodo
	env   *entorno
	fallo Error
}

func (l *lectorArgs) dado(i int) *nodo {
	if i < len(l.a) && l.a[i].clase != nVacio {
		return l.a[i]
	}
	return nil
}

func (l *lectorArgs) matriz(i int) Matrix {
	if x := l.dado(i); x != nil {
		return l.e.evalNodeMatrix(x, l.env)
	}
	return nil
}

func (l *lectorArgs) falla(err Error) {
	if l.fallo == "" {
		l.fallo = err
	}
}

func (l *lectorArgs) entero(i int) *int {
	x := l.dado(i)
	if x == nil {
		return nil
	}
	v, err := CoerceNumber(l.e.evalNode(x, l.env))
	if err != "" {
		l.falla(err)
		return nil
	}
	t := aInt(math.Trunc(v))
	return &t
}

func (l *lectorArgs) lista(i int) []int {
	var out []int
	for _, fila := range l.matriz(i) {
		for _, v := range fila {
			x, err := CoerceNumber(v)
			if err != "" {
				l.falla(err)
				return nil
			}
			out = append(out, aInt(math.Trunc(x)))
		}
	}
	return out
}

// evalAgrupar es GROUPBY y PIVOTBY: lo de aquí es leer los argumentos —las matrices, el
// agregador (un LAMBDA o el nombre de una función, que también puede ser PERCENTOF, la única
// que necesita el total) y las opciones, que se pueden omitir (`GROUPBY(A:A,B:B,SUM,,0)`).
func (e *Engine) evalAgrupar(nombre string, n *nodo, env *entorno) any {
	esPivot := nombre == "PIVOTBY"
	nObl, nMax := 3, 8
	if esPivot {
		nObl, nMax = 4, 11
	}
	if len(n.args) < nObl || len(n.args) > nMax {
		return ErrValue
	}
	l := &lectorArgs{e: e, a: n.args, env: env}
	fnNodo := l.dado(nObl - 1)
	filas := l.matriz(0)
	if fnNodo == nil || filas == nil {
		return ErrValue
	}
	pideTodos := fnNodo.clase == nName && strings.EqualFold(fnNodo.str, "PERCENTOF")
	agregar := func(grupo, todos Matrix) Value {
		args := []Matrix{grupo}
		if pideTodos {
			args = append(args, todos)
		}
		return escalarDe(e.aplicarLambda(fnNodo, args, env))
	}
	if esPivot {
		cols, vals := l.matriz(1), l.matriz(2)
		o := opcionesPivot{
			filas: filas, columnas: cols, valores: vals, agregar: agregar,
			cabeceras: l.entero(4), totalFilas: l.entero(5), ordenFilas: l.lista(6),
			totalCols: l.entero(7), ordenCols: l.lista(8), filtro: l.matriz(9),
		}
		if cols == nil || vals == nil {
			return ErrValue
		}
		if l.fallo != "" {
			return l.fallo
		}
		r, err := pivotBy(o)
		return aMatrizOError(r, err)
	}
	vals := l.matriz(1)
	o := opcionesAgrupar{
		filas: filas, valores: vals, agregar: agregar,
		cabeceras: l.entero(3), totales: l.entero(4), orden: l.lista(5), filtro: l.matriz(6),
	}
	if vals == nil {
		return ErrValue
	}
	if l.fallo != "" {
		return l.fallo
	}
	r, err := groupBy(o)
	return aMatrizOError(r, err)
}

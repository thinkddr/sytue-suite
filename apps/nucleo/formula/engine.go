// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package formula

import (
	"math"
	"math/rand/v2"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Options son las opciones del motor. Reloj y azar inyectables para que NOW, TODAY y
// RAND sean deterministas en los tests.
type Options struct {
	Sheets       []string
	Now          func() time.Time
	Random       func() float64
	ResolveTable TableResolver
}

// clave identifica una celda o, con tipo ≠ 0, la dependencia SINTÉTICA de una columna o
// fila entera: quien referencia A:A la registra y SetCell la invalida al escribir en esa
// columna, para que SUM(A:A) recalcule también cuando aparece contenido más abajo.
type clave struct {
	hoja string
	tipo uint8 // 0 celda, 1 columna entera (col), 2 fila entera (fila), 3 rango grande (hoja)
	col  int
	fila int
}

func celdaK(hoja string, col, fila int) clave { return clave{hoja: hoja, col: col, fila: fila} }
func colK(hoja string, col int) clave         { return clave{hoja: hoja, tipo: 1, col: col} }
func filaK(hoja string, fila int) clave       { return clave{hoja: hoja, tipo: 2, fila: fila} }

// grandeK es la dependencia sintética de «un rango demasiado grande para expandirlo»
// (más de topeDeps celdas): quien lo lee la registra y cualquier escritura en la hoja la
// invalida. Es grosera a propósito —no sabemos qué celdas caen dentro sin recorrerlas—,
// pero correcta: sin ella el rango no tenía ninguna dependencia y su caché quedaba rancia
// para siempre. Es la `grandeDepKey` de la web.
func grandeK(hoja string) clave { return clave{hoja: hoja, tipo: 3} }

// volatiles son las funciones VOLÁTILES: su valor puede cambiar sin que cambie ninguna
// celda que lean. NOW/TODAY/RAND/RANDBETWEEN/RANDARRAY dependen del reloj o del azar;
// OFFSET e INDIRECT construyen la referencia al evaluar, así que el motor no sabe de
// antemano de qué dependen. Una fórmula que llama a alguna, directa o por un nombre
// definido, se recalcula en cada edición. Es el `VOLATILES` de la web.
var volatiles = map[string]struct{}{
	"NOW": {}, "TODAY": {}, "RAND": {}, "RANDBETWEEN": {}, "RANDARRAY": {}, "OFFSET": {}, "INDIRECT": {},
}

// EsVolatil dice si `nombre` (en cualquier caja) es una función volátil.
func EsVolatil(nombre string) bool {
	_, ok := volatiles[strings.ToUpper(nombre)]
	return ok
}

type celda struct {
	hoja      string
	col, fila int
	raw       string
	ast       *nodo // nil si es un literal
	rota      bool  // la fórmula no parsea: vale #NAME?
	deps      []clave
	matriz    bool // fórmula de matriz CSE `{=…}`: evalúa celda a celda
	seq       int  // orden de inserción: el de un Map de JavaScript
	// sub dice si lleva un SUBTOTAL o un AGGREGATE: 0 sin mirar, 1 sí, 2 no.
	sub uint8
	// volatil: llama a NOW, RAND, OFFSET… (ver volatiles): se reevalúa en cada edición.
	volatil bool
}

type entorno struct {
	hoja       string
	col, fila  int
	ctx        *FnCtx
	locales    map[string]Matrix // variables de LET y parámetros de LAMBDA, en mayúsculas
	modoMatriz bool
	// omitidos son los parámetros del LAMBDA en curso que no recibieron argumento (ISOMITTED).
	omitidos map[string]bool
}

// derrame es un array derramado: su rectángulo y la matriz que lo llena.
type derrame struct {
	hoja           string
	c1, r1, c2, r2 int
	matriz         Matrix
}

type ancla struct {
	hoja      string
	col, fila int
	seq       int
}

type nombreDef struct {
	ast  *nodo
	hoja string
}

type usado struct{ maxCol, maxFila int }

// topeDeps es el tope de celdas al expandir un rango como dependencias.
const topeDeps = 200_000

// Engine mantiene el grid de una o varias hojas, parsea las celdas «=», resuelve
// referencias y rangos, evalúa por recursión memorizada recalculando solo lo sucio, y
// detecta ciclos (#CIRC!). No es seguro para uso concurrente: quien lo comparte entre
// gorrutinas lo guarda con su cerrojo.
type Engine struct {
	cells         map[clave]*celda
	cache         map[clave]Value
	revDeps       map[clave]map[clave]struct{}
	computing     map[clave]struct{}
	names         []string
	clock         func() time.Time
	rng           func() float64
	tableResolver TableResolver
	definedNames  map[string]nombreDef
	spills        map[clave]*derrame
	coverage      map[clave]clave
	arrayAnchors  map[clave]ancla
	resolving     map[string]struct{}
	used          map[string]*usado
	seq           int
	// ocultas son las filas que no se ven, por hoja: 1 a mano (esquema), 2 por el
	// filtro. Ver SetHiddenRows.
	ocultas map[string]map[int]uint8
	// volatiles son las celdas volátiles, para invalidarlas en cada edición sin recorrer
	// todas. volatilesSucias dice que hubo una edición desde la última lectura.
	volatiles       map[clave]struct{}
	volatilesSucias bool
	// evaluadas cuenta las fórmulas evaluadas de verdad (no las servidas de la caché): la
	// medida de «una edición solo reevalúa a sus dependientes».
	evaluadas int
	// profundidadLambda cuenta las llamadas anidadas a LAMBDA: una con nombre puede
	// llamarse a sí misma, y sin tope colgaría la pestaña.
	profundidadLambda int
}

// New crea un motor. Sin hojas, nace con «Hoja1».
func New(o Options) *Engine {
	e := &Engine{
		cells:         map[clave]*celda{},
		cache:         map[clave]Value{},
		revDeps:       map[clave]map[clave]struct{}{},
		computing:     map[clave]struct{}{},
		names:         []string{"Hoja1"},
		clock:         time.Now,
		rng:           rand.Float64,
		tableResolver: o.ResolveTable,
		definedNames:  map[string]nombreDef{},
		spills:        map[clave]*derrame{},
		coverage:      map[clave]clave{},
		arrayAnchors:  map[clave]ancla{},
		resolving:     map[string]struct{}{},
		used:          map[string]*usado{},
		volatiles:     map[clave]struct{}{},
	}
	if len(o.Sheets) > 0 {
		e.names = slices.Clone(o.Sheets)
	}
	if o.Now != nil {
		e.clock = o.Now
	}
	if o.Random != nil {
		e.rng = o.Random
	}
	return e
}

// SheetNames son los nombres de las hojas, en orden.
func (e *Engine) SheetNames() []string { return slices.Clone(e.names) }

// SetHiddenRows dice qué filas de la hoja no se ven: `filtradas`, las que esconde el
// autofiltro, y `manuales`, las del detalle de un grupo contraído. Solo lo leen
// SUBTOTAL y AGGREGATE; el resto de funciones cuenta todas las filas. Una fila en las
// dos listas cuenta como filtrada. Es el `setHiddenRows` de la web.
//
// ponytail: vacía la caché ENTERA en vez de solo las celdas con resumen, como la web.
// Se llama al contraer un grupo o filtrar, no al teclear.
func (e *Engine) SetHiddenRows(sheet string, filtradas, manuales []int) {
	m := map[int]uint8{}
	for _, r := range manuales {
		m[r] = 1
	}
	for _, r := range filtradas {
		m[r] = 2
	}
	if len(m) == 0 {
		delete(e.ocultas, sheet)
	} else {
		if e.ocultas == nil {
			e.ocultas = map[string]map[int]uint8{}
		}
		e.ocultas[sheet] = m
	}
	clear(e.cache)
	clear(e.spills)
	clear(e.coverage)
}

func (e *Engine) siguiente() int {
	e.seq++
	return e.seq
}

// SetCell escribe el texto crudo de una celda (fórmula o literal); "" la vacía. Falla
// solo si a1 no es una referencia.
func (e *Engine) SetCell(sheet, a1, raw string) error {
	r, err := ParseCellWord(a1, "")
	if err != nil {
		return err
	}
	e.SetCellAt(sheet, r.Col, r.Row, raw)
	return nil
}

// SetCellAt es SetCell por coordenadas 0-based.
func (e *Engine) SetCellAt(sheet string, col, row int, raw string) {
	if !slices.Contains(e.names, sheet) {
		e.names = append(e.names, sheet)
	}
	k := celdaK(sheet, col, row)
	old := e.cells[k]
	if old != nil {
		for _, d := range old.deps {
			delete(e.revDeps[d], k)
		}
	}
	delete(e.volatiles, k)
	hadArrays := len(e.arrayAnchors) > 0
	delete(e.arrayAnchors, k)
	if raw == "" {
		delete(e.cells, k)
	} else {
		e.growUsed(sheet, col, row)
		c := &celda{hoja: sheet, col: col, fila: row, raw: raw}
		if old != nil {
			c.seq = old.seq // un Map conserva la posición de una clave que ya existía
		} else {
			c.seq = e.siguiente()
		}
		// Fórmula normal «=…» o de matriz clásica CSE «{=…}» (se le quitan las llaves).
		c.matriz = strings.HasPrefix(raw, "{=") && strings.HasSuffix(raw, "}")
		if c.matriz || strings.HasPrefix(raw, "=") {
			src := raw
			if c.matriz {
				src = raw[1 : len(raw)-1]
			}
			if ast, err := parseFormula(src); err != nil {
				c.rota = true
			} else {
				c.ast = ast
				c.deps = e.collectDeps(ast, Here{sheet, col, row})
				if c.volatil = e.esVolatil(ast, map[string]struct{}{}); c.volatil {
					e.volatiles[k] = struct{}{}
				}
				if c.matriz || mightSpill(ast) {
					e.arrayAnchors[k] = ancla{sheet, col, row, e.siguiente()}
				}
			}
		}
		e.cells[k] = c
		e.anotarDeps(k, c.deps)
	}
	e.invalidate(k)
	// Invalida a quien vigila la COLUMNA o la FILA entera de esta celda (A:A / 1:1).
	e.invalidate(colK(sheet, col))
	e.invalidate(filaK(sheet, row))
	// Y a quien lee un rango demasiado grande para vigilarlo celda a celda.
	e.invalidate(grandeK(sheet))
	// Las volátiles se reevalúan en cada edición, aunque no lean nada de lo editado. Se
	// invalidan al LEER, no aquí: cargar 100.000 celdas con 10.000 RAND() serían mil
	// millones de invalidaciones.
	if len(e.volatiles) > 0 {
		e.volatilesSucias = true
	}
	switch {
	case !hadArrays && len(e.arrayAnchors) > 0:
		// Aparece el primer array: purga la caché por si algún "" vacío quedó cacheado y
		// ahora un derrame lo cubrirá.
		clear(e.cache)
		clear(e.spills)
		clear(e.coverage)
	case len(e.arrayAnchors) > 0:
		// Con arrays presentes, cualquier edición puede crear o romper una colisión de
		// derrame: revalida las anclas.
		// ponytail: revalida TODAS las anclas por edición; índice de solapamiento si algún
		// día hay muchísimos arrays en una misma hoja.
		for _, a := range e.anclasEnOrden() {
			e.invalidate(celdaK(a.hoja, a.col, a.fila))
		}
	}
}

func (e *Engine) anotarDeps(k clave, deps []clave) {
	for _, d := range deps {
		s := e.revDeps[d]
		if s == nil {
			s = map[clave]struct{}{}
			e.revDeps[d] = s
		}
		s[k] = struct{}{}
	}
}

func (e *Engine) anclasEnOrden() []ancla {
	out := make([]ancla, 0, len(e.arrayAnchors))
	for _, a := range e.arrayAnchors {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq < out[j].seq })
	return out
}

/* ---------- Nombres definidos ---------- */

var (
	reNombre  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	reCeldoso = regexp.MustCompile(`^\$?[A-Za-z]{1,3}\$?\d+$`)
)

func isValidName(name string) bool {
	if !reNombre.MatchString(name) || reCeldoso.MatchString(name) {
		return false
	}
	switch strings.ToUpper(name) {
	case "TRUE", "FALSE", "R", "C":
		return false
	}
	return true
}

// DefineName registra (o redefine) un nombre. definition es una expresión: un rango, un
// valor o una fórmula. scopeSheet es la hoja base de las referencias sin cualificar (por
// defecto, la primera). Falso si el nombre no es un identificador válido (o parece una
// referencia) o si la definición no parsea.
func (e *Engine) DefineName(name, definition, scopeSheet string) bool {
	return e.definir(name, definition, scopeSheet, false)
}

// DefineSheetName es DefineName con ámbito de HOJA (`defineName(…, true)` en la web): el
// nombre solo existe en las fórmulas de sheet y allí tapa al del libro que se llame igual.
func (e *Engine) DefineSheetName(name, definition, sheet string) bool {
	return slices.Contains(e.names, sheet) && e.definir(name, definition, sheet, true)
}

func (e *Engine) definir(name, definition, scopeSheet string, local bool) bool {
	if !isValidName(name) {
		return false
	}
	ast, err := parseFormula(definition)
	if err != nil {
		return false
	}
	hoja := e.names[0]
	if scopeSheet != "" && slices.Contains(e.names, scopeSheet) {
		hoja = scopeSheet
	}
	clave := strings.ToUpper(name)
	if local {
		clave = hoja + sepNombre + clave
	}
	e.definedNames[clave] = nombreDef{ast, hoja}
	e.rebuildAllDeps()
	return true
}

// sepNombre separa la hoja del nombre en la clave de un nombre de hoja.
const sepNombre = "\x00"

// nombre es el nombre up visto desde una fórmula de sheet: el de esa hoja gana al del libro.
func (e *Engine) nombre(up, sheet string) (string, nombreDef, bool) {
	if def, ok := e.definedNames[sheet+sepNombre+up]; ok {
		return sheet + sepNombre + up, def, true
	}
	def, ok := e.definedNames[up]
	return up, def, ok
}

// DeleteName borra un nombre definido y dice si existía.
func (e *Engine) DeleteName(name string) bool {
	up := strings.ToUpper(name)
	if _, ok := e.definedNames[up]; !ok {
		return false
	}
	delete(e.definedNames, up)
	e.rebuildAllDeps()
	return true
}

// DefinedNameList son los nombres definidos (en mayúsculas, sin repetir los de hoja),
// ordenados.
func (e *Engine) DefinedNameList() []string {
	out := make([]string, 0, len(e.definedNames))
	for n := range e.definedNames {
		if i := strings.LastIndex(n, sepNombre); i >= 0 {
			n = n[i+len(sepNombre):]
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return slices.Compact(out)
}

/* ---------- Lectura ---------- */

// GetRaw es el texto crudo tal cual se escribió; "" si la celda está vacía.
func (e *Engine) GetRaw(sheet, a1 string) (string, error) {
	r, err := ParseCellWord(a1, "")
	if err != nil {
		return "", err
	}
	return e.RawAt(sheet, r.Col, r.Row), nil
}

// RawAt es GetRaw por coordenadas.
func (e *Engine) RawAt(sheet string, col, row int) string {
	if c := e.cells[celdaK(sheet, col, row)]; c != nil {
		return c.raw
	}
	return ""
}

// GetValue es el valor calculado (número, texto, booleano o error); "" si está vacía.
func (e *Engine) GetValue(sheet, a1 string) (Value, error) {
	r, err := ParseCellWord(a1, "")
	if err != nil {
		return nil, err
	}
	e.refrescarVolatiles()
	return e.evalCell(sheet, r.Col, r.Row), nil
}

// ValueAt es GetValue por coordenadas.
func (e *Engine) ValueAt(sheet string, col, row int) Value {
	e.refrescarVolatiles()
	return e.evalCell(sheet, col, row)
}

// refrescarVolatiles, si hubo una edición desde la última lectura, descarta la caché de las
// volátiles y de lo que depende de ellas. Dentro de una misma lectura son consistentes: se
// calculan una vez y se cachean, como en un recálculo de Excel.
func (e *Engine) refrescarVolatiles() {
	if !e.volatilesSucias {
		return
	}
	e.volatilesSucias = false
	for k := range e.volatiles {
		e.invalidate(k)
	}
}

// GetDisplay es el texto a mostrar: errores como su código y números formateados.
func (e *Engine) GetDisplay(sheet, a1 string) (string, error) {
	v, err := e.GetValue(sheet, a1)
	if err != nil {
		return "", err
	}
	return Display(v), nil
}

// Display es el texto «General» de un valor.
func Display(v Value) string {
	switch x := v.(type) {
	case Error:
		return string(x)
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		return FormatNumber(x)
	case string:
		return x
	}
	return ""
}

// Evaluaciones son las fórmulas que el motor ha evaluado de verdad desde que existe (las
// que sirve la caché no cuentan). Sirve para comprobar, y medir, que una edición solo
// reevalúa lo que depende de ella.
func (e *Engine) Evaluaciones() int { return e.evaluadas }

// esVolatil dice si el árbol llama a una función volátil, directamente o a través de un
// nombre definido. `vistos` corta los nombres que se referencian a sí mismos.
func (e *Engine) esVolatil(n *nodo, vistos map[string]struct{}) bool {
	if n == nil {
		return false
	}
	switch n.clase {
	case nCall:
		if EsVolatil(n.str) {
			return true
		}
		for _, a := range n.args {
			if e.esVolatil(a, vistos) {
				return true
			}
		}
		// Un nombre definido que se llama como función (LAMBDA con nombre) es volátil si lo es su cuerpo.
		up := strings.ToUpper(n.str)
		def, ok := e.definedNames[up]
		if _, visto := vistos[up]; !ok || visto || registro[up] != nil {
			return false
		}
		vistos[up] = struct{}{}
		return e.esVolatil(def.ast, vistos)
	case nName:
		up := strings.ToUpper(n.str)
		def, ok := e.definedNames[up]
		if _, visto := vistos[up]; !ok || visto {
			return false
		}
		vistos[up] = struct{}{}
		return e.esVolatil(def.ast, vistos)
	case nUnary, nPercent:
		return e.esVolatil(n.izq, vistos)
	case nBinary:
		return e.esVolatil(n.izq, vistos) || e.esVolatil(n.der, vistos)
	default:
	}
	return false
}

// Recalc fuerza el cálculo de todas las celdas de fórmula.
func (e *Engine) Recalc() {
	e.refrescarVolatiles()
	var fs []*celda
	for _, c := range e.cells {
		if c.ast != nil || c.rota {
			fs = append(fs, c)
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].seq < fs[j].seq })
	for _, c := range fs {
		e.evalCell(c.hoja, c.col, c.fila)
	}
}

/* ---------- Interno ---------- */

// invalidate borra la caché de k y, recursivamente, la de todo lo que dependa de él.
func (e *Engine) invalidate(k clave) {
	pila := []clave{k}
	visto := map[clave]struct{}{}
	for len(pila) > 0 {
		cur := pila[len(pila)-1]
		pila = pila[:len(pila)-1]
		if _, ok := visto[cur]; ok {
			continue
		}
		visto[cur] = struct{}{}
		delete(e.cache, cur)
		// Si cur era el ancla de un derrame, desmóntalo: libera sus celdas cubiertas y
		// propágales la invalidación.
		if sp := e.spills[cur]; sp != nil {
			delete(e.spills, cur)
			for r := sp.r1; r <= sp.r2; r++ {
				for c := sp.c1; c <= sp.c2; c++ {
					ck := celdaK(sp.hoja, c, r)
					if cubre, ok := e.coverage[ck]; ok && cubre == cur && (r != sp.r1 || c != sp.c1) {
						delete(e.coverage, ck)
						pila = append(pila, ck)
					}
				}
			}
		}
		for d := range e.revDeps[cur] {
			pila = append(pila, d)
		}
	}
}

// rebuildAllDeps recomputa las dependencias de todas las celdas (tras cambiar un
// nombre) y purga caché y derrames: los valores se recalculan perezosamente.
func (e *Engine) rebuildAllDeps() {
	clear(e.revDeps)
	clear(e.volatiles)
	for k, c := range e.cells {
		c.deps = nil
		if c.ast != nil {
			// Un nombre redefinido puede hacer volátil (o dejar de serlo) a quien lo usa.
			if c.volatil = e.esVolatil(c.ast, map[string]struct{}{}); c.volatil {
				e.volatiles[k] = struct{}{}
			}
			c.deps = e.collectDeps(c.ast, Here{c.hoja, c.col, c.fila})
			e.anotarDeps(k, c.deps)
		}
	}
	clear(e.cache)
	clear(e.spills)
	clear(e.coverage)
}

func (e *Engine) evalCell(sheet string, col, row int) Value {
	k := celdaK(sheet, col, row)
	if hit, ok := e.cache[k]; ok {
		return hit
	}
	if _, ok := e.computing[k]; ok {
		return ErrCirc // ciclo: no se cachea aquí
	}
	c := e.cells[k]
	switch {
	case c == nil:
		v := e.coveredValue(sheet, col, row, k)
		// Un "" vacío NO se cachea si hay arrays vivos: un derrame posterior podría cubrir
		// esta celda y el "" cacheado quedaría obsoleto.
		if v != "" || len(e.arrayAnchors) == 0 {
			e.cache[k] = v
		}
		return v
	case c.rota:
		e.cache[k] = ErrName
		return ErrName
	case c.ast == nil:
		v := parseLiteral(c.raw)
		e.cache[k] = v
		return v
	}
	e.computing[k] = struct{}{}
	e.evaluadas++
	v := e.evaluar(c, k)
	delete(e.computing, k)
	e.cache[k] = v
	return v
}

// sinRed hace que un pánico dentro de una función de librería se propague en vez de
// convertirse en #VALUE!. Solo para los tests, que quieren VER el pánico.
var sinRed bool

// evaluar calcula una celda de fórmula. Un pánico en una función —un índice fuera de
// rango con una entrada que nadie previó— se queda en #VALUE! en ESA celda: una hoja que
// no abre por una fórmula es peor que una celda con error. El original tampoco lo
// captura, y allí lo que cae es la pestaña.
func (e *Engine) evaluar(c *celda, k clave) (v Value) {
	if !sinRed {
		defer func() {
			if recover() != nil {
				v = ErrValue
			}
		}()
	}
	env := &entorno{hoja: c.hoja, col: c.col, fila: c.fila, ctx: &FnCtx{e, Here{c.hoja, c.col, c.fila}}, modoMatriz: c.matriz}
	return e.finishCell(c.hoja, c.col, c.fila, k, e.evalNodeMatrix(c.ast, env))
}

// finishCell da el resultado de nivel superior de una celda: un array 1×1 colapsa a su
// valor y uno de varias celdas se DERRAMA hacia abajo y a la derecha desde el ancla. Si
// el área destino choca con contenido u otro derrame → #SPILL!.
func (e *Engine) finishCell(sheet string, col, row int, k clave, m Matrix) Value {
	filas, cols := len(m), 0
	if filas > 0 {
		cols = len(m[0])
	}
	if filas <= 1 && cols <= 1 {
		delete(e.arrayAnchors, k)
		if filas == 0 || cols == 0 {
			return ""
		}
		return m[0][0]
	}
	if _, ok := e.arrayAnchors[k]; !ok {
		e.arrayAnchors[k] = ancla{sheet, col, row, e.siguiente()}
	}
	c2, r2 := col+cols-1, row+filas-1
	for r := row; r <= r2; r++ {
		for c := col; c <= c2; c++ {
			if r == row && c == col {
				continue // el ancla no cuenta como choque
			}
			tk := celdaK(sheet, c, r)
			_, ocupada := e.cells[tk]
			cubre, cubierta := e.coverage[tk]
			_, derramada := e.spills[tk]
			if ocupada || (cubierta && cubre != k) || derramada {
				return ErrSpill
			}
		}
	}
	e.spills[k] = &derrame{sheet, col, row, c2, r2, m}
	e.growUsed(sheet, c2, r2) // el derrame ocupa celdas: cuenta como rango usado
	for r := row; r <= r2; r++ {
		for c := col; c <= c2; c++ {
			if r != row || c != col {
				e.coverage[celdaK(sheet, c, r)] = k
			}
		}
	}
	return m[0][0]
}

// siguienteAncla emula el iterador VIVO de un Map: la primera ancla con seq mayor que
// el último visitado, exista desde antes o haya nacido durante el recorrido.
// ponytail: O(anclas) por paso; un índice ordenado si una hoja llega a miles de arrays.
func (e *Engine) siguienteAncla(tras int) (ancla, bool) {
	mejor, hay := ancla{}, false
	for _, a := range e.arrayAnchors {
		if a.seq > tras && (!hay || a.seq < mejor.seq) {
			mejor, hay = a, true
		}
	}
	return mejor, hay
}

// coveredValue es el valor de una celda SIN contenido propio: el de un array que la
// cubra, o "".
func (e *Engine) coveredValue(sheet string, col, row int, k clave) Value {
	anchor, ok := e.coverage[k]
	if !ok && len(e.arrayAnchors) > 0 {
		// Puede que un array aún no evaluado derrame aquí: evalúa las candidatas de arriba
		// a la izquierda de la misma hoja (memorizado: barato tras la primera vez).
		for a, hay := e.siguienteAncla(0); hay; a, hay = e.siguienteAncla(a.seq) {
			if a.hoja != sheet || a.col > col || a.fila > row {
				continue
			}
			ak := celdaK(a.hoja, a.col, a.fila)
			_, derramada := e.spills[ak]
			_, calculando := e.computing[ak]
			if ak != k && !derramada && !calculando {
				e.evalCell(a.hoja, a.col, a.fila)
			}
		}
		anchor, ok = e.coverage[k]
	}
	if ok {
		if sp := e.spills[anchor]; sp != nil {
			if r, c := row-sp.r1, col-sp.c1; r >= 0 && r < len(sp.matriz) && c >= 0 && c < len(sp.matriz[r]) {
				return sp.matriz[r][c]
			}
		}
	}
	return ""
}

// resolveTableRef resuelve `Tabla[Columna]` con el resolutor inyectado. Sin resolutor o
// si la tabla o la columna no existen → #REF!; otro error del resolutor se propaga.
func (e *Engine) resolveTableRef(n *nodo, here Here) (*RefRange, Error) {
	if e.tableResolver == nil {
		return nil, ErrRef
	}
	rr, err := e.tableResolver(n.tabla, here)
	if err != "" {
		return nil, err
	}
	if rr == nil {
		return nil, ErrRef
	}
	return rr, ""
}

func (e *Engine) parseRefText(text, defSheet string) *RefRange {
	n, err := parseFormula(text)
	if err != nil {
		return nil
	}
	switch n.clase {
	case nRef:
		rr := toRefRange(n.a, defSheet)
		return &rr
	case nRange:
		rr := e.rangeToRefRange(n, defSheet)
		return &rr
	default: // lo demás no nombra un rango
	}
	return nil
}

func hojaDe(ref CellRef, defSheet string) string {
	if ref.Sheet != "" {
		return ref.Sheet
	}
	return defSheet
}

func toRefRange(ref CellRef, defSheet string) RefRange {
	return RefRange{hojaDe(ref, defSheet), ref.Col, ref.Row, ref.Col, ref.Row}
}

// UsedRange es `usedRange` de la web: la última columna y la última fila usadas de una
// hoja —celdas con contenido y derrames—, o (-1, -1) si está vacía. Lo usa quien recorre
// «hasta donde hay datos» (el autofiltro): debajo todo está vacío.
//
// ponytail: aquí es una marca de agua que SOLO CRECE (growUsed), mientras que la web la
// recalcula al vaciar celdas. Techo: tras borrar las últimas filas sigue contando hasta
// la que hubo, hasta que el motor se rehace. Se releva recalculándola al borrar, como
// `usado()` en engine.ts.
func (e *Engine) UsedRange(sheet string) (maxCol, maxRow int) {
	if u := e.used[sheet]; u != nil {
		return u.maxCol, u.maxFila
	}
	return -1, -1
}

// growUsed amplía la marca de agua del rango usado de una hoja. Solo crece.
func (e *Engine) growUsed(sheet string, col, row int) {
	u := e.used[sheet]
	if u == nil {
		e.used[sheet] = &usado{col, row}
		return
	}
	u.maxCol, u.maxFila = max(u.maxCol, col), max(u.maxFila, row)
}

// rangeToRefRange traduce un nodo de rango a rectángulo. Una columna entera va de la
// fila 0 a la última USADA de la hoja (JAMÁS 1.048.575: invariante de escala) y una fila
// entera, igual con las columnas. En una hoja vacía el límite es -1: rango vacío.
func (e *Engine) rangeToRefRange(n *nodo, defSheet string) RefRange {
	s := hojaDe(n.a, defSheet)
	rr := RefRange{s, min(n.a.Col, n.z.Col), min(n.a.Row, n.z.Row), max(n.a.Col, n.z.Col), max(n.a.Row, n.z.Row)}
	u := e.used[s]
	if u == nil {
		u = &usado{-1, -1}
	}
	switch n.eje {
	case ejeCol:
		rr.R1, rr.R2 = 0, u.maxFila
	case ejeFila:
		rr.C1, rr.C2 = 0, u.maxCol
	}
	return rr
}

// maxCeldasRango acota lo que se materializa de UN rango escrito con sus celdas
// (FYE-02): `A1:A99999999999999999999` llegaba a pedir una matriz de dos mil
// millones de filas. Los rangos de eje entero ya se acotan a lo usado.
const maxCeldasRango = 4_000_000

func dimensionRango(inicio, fin, tope int) (int, bool) {
	if tope <= 0 || fin < inicio {
		return 0, false
	}
	delta := fin - inicio
	// Con fin >= inicio, una diferencia negativa indica desbordamiento.
	// delta < tope garantiza que sumar la celda final tampoco desborda.
	if delta < 0 || delta >= tope {
		return 0, false
	}
	return delta + 1, true
}

func dimensionesRango(rr RefRange, tope int) (filas, cols int, ok bool) {
	filas, ok = dimensionRango(rr.R1, rr.R2, tope)
	if !ok {
		return 0, 0, false
	}
	cols, ok = dimensionRango(rr.C1, rr.C2, tope)
	if !ok || filas > tope/cols {
		return 0, 0, false
	}
	return filas, cols, true
}

func (e *Engine) resolveRange(rr RefRange) Matrix {
	if rr.R2 < rr.R1 || rr.C2 < rr.C1 {
		return Matrix{}
	}
	filas, cols, ok := dimensionesRango(rr, maxCeldasRango)
	if !ok {
		return Matrix{{ErrRef}}
	}
	out := make(Matrix, 0, filas)
	for ri := 0; ri < filas; ri++ {
		r := rr.R1 + ri
		fila := make([]Value, 0, cols)
		for ci := 0; ci < cols; ci++ {
			c := rr.C1 + ci
			fila = append(fila, e.evalCell(rr.Sheet, c, r))
		}
		out = append(out, fila)
	}
	return out
}

func clavesDe(rr RefRange) []clave {
	if rr.R2 < rr.R1 || rr.C2 < rr.C1 {
		return nil
	}
	filas, cols, ok := dimensionesRango(rr, topeDeps)
	if !ok {
		// ponytail: se vigila la hoja entera, no el rectángulo; si importara, un índice de
		// rectángulos en SetCellAt.
		return []clave{grandeK(rr.Sheet)}
	}
	out := make([]clave, 0, filas*cols)
	for ri := 0; ri < filas; ri++ {
		r := rr.R1 + ri
		for ci := 0; ci < cols; ci++ {
			c := rr.C1 + ci
			out = append(out, celdaK(rr.Sheet, c, r))
		}
	}
	return out
}

// collectDeps expande las referencias del AST a claves (para invalidar). Un nombre se
// expande a las dependencias de su definición, anclada en su hoja y con guarda de ciclos.
func (e *Engine) collectDeps(raiz *nodo, here Here) []clave {
	var keys []clave
	guarda := map[string]struct{}{}
	var walk func(n *nodo, sheet string)
	// caminarNombre recorre la definición de un nombre (una sola vez: los recursivos no se cuelgan),
	// resuelto desde la hoja `sheet`: un nombre de hoja manda sobre el del libro.
	caminarNombre := func(up, sheet string) {
		if clave, def, ok := e.nombre(up, sheet); ok {
			if _, visto := guarda[clave]; !visto {
				guarda[clave] = struct{}{}
				walk(def.ast, def.hoja)
			}
		}
	}
	walk = func(n *nodo, sheet string) {
		switch n.clase {
		case nRef:
			keys = append(keys, celdaK(hojaDe(n.a, sheet), n.a.Col, n.a.Row))
		case nRange:
			rr := e.rangeToRefRange(n, sheet)
			// Eje entero: se vigila por COLUMNA o FILA (clave sintética), no celda a celda.
			switch n.eje {
			case ejeCol:
				for c := rr.C1; c <= rr.C2; c++ {
					keys = append(keys, colK(rr.Sheet, c))
				}
			case ejeFila:
				for r := rr.R1; r <= rr.R2; r++ {
					keys = append(keys, filaK(rr.Sheet, r))
				}
			default:
				keys = append(keys, clavesDe(rr)...)
			}
		case nStructured:
			// ponytail: si la tabla se REDIMENSIONA sin editar esta fórmula, el editor
			// (dueño del mapa) debe recalcular; el motor resuelve en cada evaluación.
			if rr, err := e.resolveTableRef(n, Here{sheet, here.Col, here.Row}); err == "" {
				keys = append(keys, clavesDe(*rr)...)
			}
		case nName:
			caminarNombre(strings.ToUpper(n.str), sheet)
		case nUnary, nPercent:
			walk(n.izq, sheet)
		case nBinary:
			walk(n.izq, sheet)
			walk(n.der, sheet)
		case nCall:
			for _, a := range n.args {
				walk(a, sheet)
			}
			// Una LAMBDA con nombre llamada como función: lo que lee su cuerpo también cuenta.
			if up := strings.ToUpper(n.str); registro[up] == nil {
				caminarNombre(up, sheet)
			}
		default: // una hoja del árbol (número, texto, booleano) no depende de nada
		}
	}
	walk(raiz, here.Sheet)
	return keys
}

/* ---------- Evaluación del AST ---------- */

func (e *Engine) evalNode(n *nodo, env *entorno) Value {
	switch n.clase {
	case nNum:
		return n.num
	case nStr:
		return n.str
	case nBool:
		return n.b
	case nArray:
		return n.matriz[0][0]
	case nName:
		m := e.evalNameNode(n, env)
		if m == nil {
			return ErrName
		}
		return collapse(m)
	case nRef:
		return e.evalCell(hojaDe(n.a, env.hoja), n.a.Col, n.a.Row)
	case nRange, nStructured:
		return collapse(e.evalNodeMatrix(n, env))
	case nUnary, nPercent:
		v, err := CoerceNumber(e.evalNode(n.izq, env))
		if err != "" {
			return err
		}
		return unario(n, v)
	case nBinary:
		return applyBinary(n.str, e.evalNode(n.izq, env), e.evalNode(n.der, env))
	case nCall:
		return aEscalar(e.evalCall(n, env))
	case nVacio:
		return "" // argumento omitido: en blanco
	}
	return ErrValue
}

func unario(n *nodo, v float64) float64 {
	switch {
	case n.clase == nPercent:
		return v / 100
	case n.str == "-":
		return -v
	}
	return v
}

// evalBinaryMatrix aplica un operador con difusión (broadcast) celda a celda: un escalar
// se difunde, un vector fila o columna se estira contra una matriz, y las dimensiones
// incompatibles dan #N/A en esa celda. Es lo que permite «A1:A3*2» o «A1:A3>2».
func (e *Engine) evalBinaryMatrix(n *nodo, env *entorno) Matrix {
	izq := e.evalNodeMatrix(n.izq, env)
	der := e.evalNodeMatrix(n.der, env)
	lr, lc := dims(izq)
	rr, rc := dims(der)
	if lr <= 1 && lc <= 1 && rr <= 1 && rc <= 1 {
		return Matrix{{applyBinary(n.str, en(izq, 0, 0, ""), en(der, 0, 0, ""))}}
	}
	filas, cols := max(lr, rr), max(lc, rc)
	out := make(Matrix, filas)
	for r := range out {
		out[r] = make([]Value, cols)
		for c := range out[r] {
			out[r][c] = applyBinary(n.str, difundido(izq, lr, lc, r, c), difundido(der, rr, rc, r, c))
		}
	}
	return out
}

// dims son las filas y las columnas (las de la primera fila) de una matriz.
func dims(m Matrix) (int, int) {
	if len(m) == 0 {
		return 0, 0
	}
	return len(m), len(m[0])
}

// en es `m[r]?.[c] ?? def`.
func en(m Matrix, r, c int, def Value) Value {
	if r < 0 || r >= len(m) || c < 0 || c >= len(m[r]) {
		return def
	}
	return m[r][c]
}

// difundido lee la celda (r, c) de una matriz estirando la dimensión que mida 1; lo que
// falta es #N/A.
func difundido(m Matrix, filas, cols, r, c int) Value {
	if filas == 1 {
		r = 0
	}
	if cols == 1 {
		c = 0
	}
	return en(m, r, c, ErrNA)
}

// agrupadoras son GROUPBY y PIVOTBY: reciben una función (un LAMBDA o el nombre de una).
var agrupadoras = map[string]bool{"GROUPBY": true, "PIVOTBY": true}

// esLambda dice si el nodo es una llamada a LAMBDA.
func esLambda(n *nodo) bool {
	return n != nil && n.clase == nCall && strings.EqualFold(n.str, "LAMBDA")
}

// aMatrizOError devuelve el resultado de aplicar una función como lo espera evalCall: la
// matriz, o el error como valor.
func aMatrizOError(m Matrix, err Error) any {
	if err != "" {
		return err
	}
	return m
}

// ordenSuperior son las que reciben un LAMBDA: no caben en el registro, que entrega
// valores ya evaluados, y se despachan en evalCall como LET.
var ordenSuperior = map[string]bool{"MAP": true, "REDUCE": true, "SCAN": true, "MAKEARRAY": true, "BYROW": true, "BYCOL": true}

func (e *Engine) evalCall(n *nodo, env *entorno) any {
	nombre := strings.ToUpper(n.str)
	switch {
	case nombre == "LET":
		return e.evalLet(n, env)
	case ordenSuperior[nombre]:
		return e.evalOrdenSuperior(nombre, n, env)
	case agrupadoras[nombre]:
		return e.evalAgrupar(nombre, n, env)
	case nombre == "ISOMITTED":
		return e.evalIsOmitted(n, env)
	case nombre == "LAMBDA":
		return ErrCalc // suelta no es un valor: sin función que la aplique no hay qué calcular
	}
	f := registro[nombre]
	if f == nil {
		// Un nombre definido que guarda un LAMBDA se llama como una función: =DOBLE(A1).
		if def, ok := e.definedNames[nombre]; ok && esLambda(def.ast) {
			valores := make([]Matrix, len(n.args))
			omitidos := make([]bool, len(n.args))
			for i, a := range n.args {
				valores[i] = e.evalNodeMatrix(a, env)
				omitidos[i] = a.clase == nVacio
			}
			limpio := *env
			limpio.hoja, limpio.locales, limpio.omitidos = def.hoja, nil, nil
			return aMatrizOError(e.ejecutarLambda(def.ast, valores, omitidos, &limpio, nil))
		}
		return ErrName
	}
	// En una fórmula de matriz CSE, las funciones escalares se difunden celda a celda.
	if env.modoMatriz && elementwise[nombre] {
		return e.vectorizeCall(f, n.args, env)
	}
	return f(env.ctx, e.makeArgs(n.args, env))
}

func (e *Engine) makeArgs(nodos []*nodo, env *entorno) []*Arg {
	args := make([]*Arg, len(nodos))
	for i, a := range nodos {
		args[i] = &Arg{e: e, n: a, env: env}
	}
	return args
}

// vectorizeCall difunde una función escalar sobre sus argumentos-array (semántica CSE).
// Si ningún argumento es array, es una llamada normal.
func (e *Engine) vectorizeCall(f fn, nodos []*nodo, env *entorno) any {
	mats := make([]Matrix, len(nodos))
	filas, cols := 1, 1
	for i, a := range nodos {
		mats[i] = e.evalNodeMatrix(a, env)
		mr, mc := dims(mats[i])
		filas, cols = max(filas, mr), max(cols, mc)
	}
	if filas <= 1 && cols <= 1 {
		return f(env.ctx, e.makeArgs(nodos, env))
	}
	out := make(Matrix, filas)
	for r := range out {
		out[r] = make([]Value, cols)
		for c := range out[r] {
			args := make([]*Arg, len(mats))
			for i, m := range mats {
				mr, mc := dims(m)
				if mr == 0 {
					mc = 1 // `m[0]?.length ?? 1`: solo cae al 1 sin primera fila
				}
				args[i] = scalarArg(difundido(m, mr, mc, r, c))
			}
			out[r][c] = aEscalar(f(env.ctx, args))
		}
	}
	return out
}

// aEscalar colapsa el resultado de una función a un valor.
func aEscalar(r any) Value {
	if m, ok := r.(Matrix); ok {
		return collapse(m)
	}
	return r
}

// aMatriz envuelve el resultado de una función como matriz.
func aMatriz(r any) Matrix {
	if m, ok := r.(Matrix); ok {
		return m
	}
	return Matrix{{r}}
}

// evalLet es LET(nombre1, valor1, [nombre2, valor2, …], cálculo): cada nombre ve los
// definidos antes. Los valores se guardan como matriz: una variable puede ser un array.
func (e *Engine) evalLet(n *nodo, env *entorno) any {
	a := n.args
	if len(a) < 3 || len(a)%2 == 0 {
		return ErrValue
	}
	hijo := *env
	hijo.locales = clonar(env.locales)
	for i := 0; i+1 < len(a)-1; i += 2 {
		if a[i].clase != nName {
			return ErrValue // el nombre debe ser un identificador
		}
		hijo.locales[strings.ToUpper(a[i].str)] = e.evalNodeMatrix(a[i+1], &hijo)
	}
	return e.evalNodeMatrix(a[len(a)-1], &hijo)
}

func clonar(m map[string]Matrix) map[string]Matrix {
	out := make(map[string]Matrix, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// aplicarLambda aplica una función a unos argumentos ya evaluados: un LAMBDA literal
// (cierra sobre las variables de su sitio), el nombre de un LAMBDA definido o el de una
// función de la librería (`BYROW(A1:C3, SUM)`). #VALUE! si el nodo no es nada de eso o la
// aridad no cuadra; #NAME? si el nombre no existe.
func (e *Engine) aplicarLambda(lam *nodo, valores []Matrix, env *entorno) (Matrix, Error) {
	if lam.clase == nName {
		return e.aplicarPorNombre(lam.str, valores, env)
	}
	if !esLambda(lam) {
		return nil, ErrValue
	}
	return e.ejecutarLambda(lam, valores, nil, env, env.locales)
}

func (e *Engine) aplicarPorNombre(nombre string, valores []Matrix, env *entorno) (Matrix, Error) {
	up := strings.ToUpper(nombre)
	if def, ok := e.definedNames[up]; ok && esLambda(def.ast) {
		limpio := *env
		limpio.hoja, limpio.locales, limpio.omitidos = def.hoja, nil, nil
		return e.ejecutarLambda(def.ast, valores, nil, &limpio, nil)
	}
	f := registro[up]
	if f == nil {
		return nil, ErrName
	}
	args := make([]*Arg, len(valores))
	for i, m := range valores {
		args[i] = matrizArg(m)
	}
	r := f(env.ctx, args)
	if err, ok := r.(Error); ok {
		return nil, err
	}
	return aMatriz(r), ""
}

// matrizArg es una matriz de valores como Arg: llamar a una función de la librería por su nombre.
func matrizArg(m Matrix) *Arg {
	return &Arg{sv: collapse(m), svOK: true, mv: m, mvOK: true, refOK: true}
}

// parametroLambda es un parámetro de LAMBDA: su nombre en mayúsculas y si es opcional (`[x]`).
type parametroLambda struct {
	nombre   string
	opcional bool
}

// ejecutarLambda es el cuerpo de un LAMBDA con sus parámetros ligados, como LET. Un
// parámetro entre corchetes, `[x]`, es opcional: si falta el argumento (o llega omitido,
// `F(1,,3)`) vale en blanco y ISOMITTED(x) da VERDADERO. Los opcionales van al final;
// faltar uno obligatorio o sobrar argumentos es #VALUE!. `locales` son las variables que
// el LAMBDA ve además de las suyas: las del sitio donde se escribió si es literal, ninguna
// si viene de un nombre definido.
func (e *Engine) ejecutarLambda(lam *nodo, valores []Matrix, omitidosPorPosicion []bool, env *entorno, locales map[string]Matrix) (Matrix, Error) {
	if len(lam.args) < 1 {
		return nil, ErrValue
	}
	params, cuerpo := lam.args[:len(lam.args)-1], lam.args[len(lam.args)-1]
	if len(valores) > len(params) {
		return nil, ErrValue // sobran argumentos
	}
	nombres := make([]parametroLambda, 0, len(params))
	for _, p := range params {
		switch {
		case p.clase == nName:
			nombres = append(nombres, parametroLambda{strings.ToUpper(p.str), false})
		case p.clase == nStructured && p.tabla.Table == "" && p.tabla.HasColumn:
			nombres = append(nombres, parametroLambda{strings.ToUpper(p.tabla.Column), true})
		default:
			return nil, ErrValue // el parámetro debe ser un identificador
		}
	}
	// Un opcional no puede ir antes de uno obligatorio.
	if i := slices.IndexFunc(nombres, func(p parametroLambda) bool { return p.opcional }); i >= 0 {
		if slices.ContainsFunc(nombres[i:], func(p parametroLambda) bool { return !p.opcional }) {
			return nil, ErrValue
		}
	}
	if e.profundidadLambda >= 256 {
		return nil, ErrNum
	}
	hijo := *env
	hijo.locales = clonar(locales)
	hijo.omitidos = map[string]bool{}
	for k := range env.omitidos {
		hijo.omitidos[k] = true
	}
	for i, p := range nombres {
		falta := i >= len(valores) || (i < len(omitidosPorPosicion) && omitidosPorPosicion[i])
		switch {
		case falta && !p.opcional:
			return nil, ErrValue
		case falta:
			hijo.locales[p.nombre] = uno("")
			hijo.omitidos[p.nombre] = true
		default:
			hijo.locales[p.nombre] = valores[i]
			delete(hijo.omitidos, p.nombre)
		}
	}
	e.profundidadLambda++
	defer func() { e.profundidadLambda-- }()
	return e.evalNodeMatrix(cuerpo, &hijo), ""
}

// evalIsOmitted es ISOMITTED(parámetro): ¿el LAMBDA en curso recibió ese parámetro? Solo
// vale con un nombre que sea parámetro del LAMBDA (o variable de LET): otra cosa es #NAME?.
func (e *Engine) evalIsOmitted(n *nodo, env *entorno) any {
	if len(n.args) != 1 || n.args[0].clase != nName {
		return ErrValue
	}
	up := strings.ToUpper(n.args[0].str)
	if _, ok := env.locales[up]; !ok {
		return ErrName
	}
	return env.omitidos[up]
}

// escalarDe es el primer escalar de lo que devolvió una lambda.
func escalarDe(m Matrix, err Error) Value {
	if err != "" {
		return err
	}
	return en(m, 0, 0, ErrValue)
}

func uno(v Value) Matrix { return Matrix{{v}} }

// evalOrdenSuperior son MAP, REDUCE, SCAN, MAKEARRAY, BYROW y BYCOL: recorren algo
// aplicando un LAMBDA y se diferencian en QUÉ recorren y qué hacen con el resultado.
func (e *Engine) evalOrdenSuperior(nombre string, n *nodo, env *entorno) any {
	a := n.args
	quiere := map[string]int{"MAP": 2, "REDUCE": 3, "SCAN": 3, "MAKEARRAY": 3, "BYROW": 2, "BYCOL": 2}[nombre]
	if len(a) != quiere {
		return ErrValue
	}
	switch nombre {
	case "MAP":
		m := e.evalNodeMatrix(a[0], env)
		out := make(Matrix, len(m))
		for r, fila := range m {
			out[r] = make([]Value, len(fila))
			for c, v := range fila {
				out[r][c] = escalarDe(e.aplicarLambda(a[1], []Matrix{uno(v)}, env))
			}
		}
		return out
	case "REDUCE", "SCAN":
		return e.reducir(nombre == "SCAN", a, env)
	case "MAKEARRAY":
		return e.makeArray(a, env)
	}
	m := e.evalNodeMatrix(a[0], env)
	if nombre == "BYROW" {
		// Cada fila entra ENTERA en la lambda y sale un valor: el resultado es una COLUMNA.
		out := make(Matrix, len(m))
		for r, fila := range m {
			out[r] = []Value{escalarDe(e.aplicarLambda(a[1], []Matrix{{fila}}, env))}
		}
		return out
	}
	_, ancho := dims(m)
	salida := make([]Value, ancho)
	for c := range salida {
		columna := make([]Value, len(m))
		for r := range m {
			columna[r] = en(m, r, c, ErrNA)
		}
		salida[c] = escalarDe(e.aplicarLambda(a[1], []Matrix{{columna}}, env))
	}
	return Matrix{salida} // BYCOL devuelve una FILA
}

// reducir es REDUCE (solo el acumulado final) y SCAN (el rastro entero).
func (e *Engine) reducir(rastro bool, a []*nodo, env *entorno) any {
	acc := e.evalNodeMatrix(a[0], env)
	m := e.evalNodeMatrix(a[1], env)
	parciales := make(Matrix, 0, len(m))
	for _, fila := range m {
		linea := make([]Value, 0, len(fila))
		for _, v := range fila {
			r, err := e.aplicarLambda(a[2], []Matrix{acc, uno(v)}, env)
			if err != "" {
				return err
			}
			acc = r
			linea = append(linea, escalarDe(r, ""))
		}
		parciales = append(parciales, linea)
	}
	if rastro {
		return parciales
	}
	return escalarDe(acc, "")
}

func (e *Engine) makeArray(a []*nodo, env *entorno) any {
	const tope = 1_000_000 // el mismo de SEQUENCE: una lambda de un millón de celdas cuelga la hoja
	filas, err := CoerceNumber(e.evalNode(a[0], env))
	if err != "" {
		return err
	}
	cols, err := CoerceNumber(e.evalNode(a[1], env))
	if err != "" {
		return err
	}
	nf, nc := math.Trunc(filas), math.Trunc(cols)
	if nf < 1 || nc < 1 {
		return ErrValue
	}
	if nf*nc > tope {
		return ErrNum
	}
	if math.IsNaN(nf * nc) {
		return Matrix{} // con NaN ninguna comparación del original corta, y sus bucles no giran
	}
	out := make(Matrix, int(nf))
	for r := range out {
		out[r] = make([]Value, int(nc))
		for c := range out[r] {
			// Los índices que ve la lambda son 1-based, como los de una hoja.
			out[r][c] = escalarDe(e.aplicarLambda(a[2], []Matrix{uno(float64(r + 1)), uno(float64(c + 1))}, env))
		}
	}
	return out
}

// evalNameNode resuelve un nombre a una matriz: primero variable local de LET, luego
// nombre definido (evaluado anclado en su hoja base). nil si el nombre no existe.
func (e *Engine) evalNameNode(n *nodo, env *entorno) Matrix {
	up := strings.ToUpper(n.str)
	if local, ok := env.locales[up]; ok {
		return local
	}
	clave, def, ok := e.nombre(up, env.hoja)
	if !ok {
		return nil
	}
	if _, ok := e.resolving[clave]; ok {
		return uno(ErrCirc) // nombre recursivo
	}
	e.resolving[clave] = struct{}{}
	defer delete(e.resolving, clave)
	return e.evalNodeMatrix(def.ast, &entorno{hoja: def.hoja, ctx: env.ctx})
}

func (e *Engine) evalNodeMatrix(n *nodo, env *entorno) Matrix {
	switch n.clase {
	case nArray:
		out := make(Matrix, len(n.matriz))
		for i, f := range n.matriz {
			out[i] = slices.Clone(f)
		}
		return out
	case nRange:
		return e.resolveRange(e.rangeToRefRange(n, env.hoja))
	case nStructured:
		rr, err := e.resolveTableRef(n, Here{env.hoja, env.col, env.fila})
		if err != "" {
			return uno(err)
		}
		return e.resolveRange(*rr)
	case nRef:
		return uno(e.evalCell(hojaDe(n.a, env.hoja), n.a.Col, n.a.Row))
	case nCall:
		return aMatriz(e.evalCall(n, env))
	case nName:
		if m := e.evalNameNode(n, env); m != nil {
			return m
		}
		return uno(ErrName)
	case nBinary:
		return e.evalBinaryMatrix(n, env)
	case nUnary, nPercent:
		m := e.evalNodeMatrix(n.izq, env)
		if mr, mc := dims(m); mr <= 1 && mc <= 1 {
			// El original vuelve a evaluar el operando por el camino escalar; con RAND()
			// dentro se nota, y las trazas cuentan cuántas veces se pidió azar.
			return uno(e.evalNode(n, env))
		}
		out := make(Matrix, len(m))
		for r, fila := range m {
			out[r] = make([]Value, len(fila))
			for c, v := range fila {
				if x, err := CoerceNumber(v); err != "" {
					out[r][c] = err
				} else {
					out[r][c] = unario(n, x)
				}
			}
		}
		return out
	default: // un escalar: por el camino de siempre
	}
	return uno(e.evalNode(n, env))
}

// applyBinary es un operador binario sobre dos valores ya evaluados (ECMA-376).
func applyBinary(op string, lv, rv Value) Value {
	switch op {
	case "&":
		l, err := CoerceText(lv)
		if err != "" {
			return err
		}
		r, err := CoerceText(rv)
		if err != "" {
			return err
		}
		return l + r
	case "=", "<>", "<", ">", "<=", ">=":
		var cmp int
		ln, lok := lv.(float64)
		rn, rok := rv.(float64)
		if !lok || !rok || !mismoA15(ln, rn) {
			var err Error
			if cmp, err = CompareValues(lv, rv); err != "" {
				return err
			}
		}
		return map[string]bool{"=": cmp == 0, "<>": cmp != 0, "<": cmp < 0, ">": cmp > 0, "<=": cmp <= 0, ">=": cmp >= 0}[op]
	}
	l, err := CoerceNumber(lv)
	if err != "" {
		return err
	}
	r, err := CoerceNumber(rv)
	if err != "" {
		return err
	}
	switch op {
	case "+":
		return finito(l + r)
	case "-":
		return finito(l - r)
	case "*":
		return finito(l * r)
	case "/":
		if r == 0 {
			return ErrDiv0
		}
		return finito(l / r)
	case "^":
		return finito(jsPow(l, r))
	}
	return ErrValue
}

// mismoA15 dice si dos números coinciden en 15 cifras significativas (0,1+0,2 = 0,3), como
// en Excel. El atajo descarta casi todo sin formatear.
func mismoA15(a, b float64) bool {
	if a == b {
		return true
	}
	if math.Abs(a-b) > 1e-10*math.Max(math.Abs(a), math.Abs(b)) {
		return false
	}
	pa, _ := strconv.ParseFloat(strconv.FormatFloat(a, 'g', 15, 64), 64)
	pb, _ := strconv.ParseFloat(strconv.FormatFloat(b, 'g', 15, 64), 64)
	return pa == pb
}

// finito es `Number.isFinite(p) ? p : #NUM!`.
func finito(p float64) Value {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return ErrNum
	}
	return p
}

// collapse colapsa una matriz a escalar: 1×1 → su valor; otra cosa → #VALUE!.
func collapse(m Matrix) Value {
	if len(m) == 1 && len(m[0]) == 1 {
		return m[0][0]
	}
	return ErrValue
}

// parseLiteral interpreta el texto crudo de una celda literal: número si parsea limpio
// (admite «%» final), TRUE/FALSE, o el texto tal cual.
func parseLiteral(raw string) Value {
	t := jsTrim(raw)
	if t == "" {
		return ""
	}
	switch jsUpper(t) {
	case "TRUE":
		return true
	case "FALSE":
		return false
	}
	if n, ok := numeroDeTexto(t); ok {
		return n
	}
	return raw
}

// mightSpill «ceba» el ancla de derrame antes de su primera evaluación, para poder
// resolver referencias a celdas derramadas sin haber evaluado antes el ancla. Los falsos
// positivos se limpian solos al evaluar.
func mightSpill(n *nodo) bool {
	switch n.clase {
	case nRange, nName, nStructured:
		return true
	case nArray:
		return len(n.matriz) > 1 || len(n.matriz[0]) > 1
	case nCall:
		return funcionesArray[strings.ToUpper(n.str)]
	default: // un escalar nunca derrama
	}
	return false
}

func conjunto(nombres string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Fields(nombres) {
		out[n] = true
	}
	return out
}

var funcionesArray = conjunto(`SEQUENCE RANDARRAY FILTER SORT SORTBY UNIQUE TRANSPOSE XLOOKUP INDEX OFFSET
	CHOOSE MMULT LET TEXTSPLIT INDIRECT MINVERSE MUNIT TREND`)

// elementwise son las funciones ESCALARES puras que, en una fórmula de matriz CSE, se
// difunden celda a celda. Lista blanca deliberada: nunca una que agregue un rango.
var elementwise = conjunto(`IF IFS IFERROR IFNA NOT SWITCH ABS SIGN SQRT POWER MOD QUOTIENT INT TRUNC ROUND
	ROUNDUP ROUNDDOWN MROUND CEILING FLOOR EXP LN LOG LOG10 FACT SIN COS TAN ASIN ACOS ATAN ATAN2 SINH COSH
	TANH DEGREES RADIANS COMBIN PERMUT LEN LEFT RIGHT MID UPPER LOWER PROPER TRIM TEXT VALUE CHAR CODE
	UNICHAR UNICODE EXACT CLEAN REPT SUBSTITUTE REPLACE FIND SEARCH NUMBERVALUE T N ISEVEN ISODD YEAR MONTH
	DAY HOUR MINUTE SECOND WEEKDAY WEEKNUM DATE TIME EDATE EOMONTH DATEVALUE TIMEVALUE ISOWEEKNUM DATESTR`)

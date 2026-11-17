// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Al insertar o borrar filas o columnas en una hoja, sus PROPIAS fórmulas siguen a su
// dato igual que las de las otras hojas: la web (`applyStructural` en SheetCanvas.tsx) y
// Calcus de Fyne (`Libro.Estructura`) mueven las celdas y reescriben cada fórmula con el
// mismo desplazamiento, así que `=SUMA(A1:A5)` en A10 llega a A11 como `=SUMA(A1:A6)` si
// se inserta una fila encima de la 3. Con la hoja protegida y la fórmula bloqueada, eso
// parecía escribir en una celda bloqueada —ese valor no estaba en la hoja—, y el
// servidor rechazaba la inserción entera a quien no es el propietario aunque tuviera
// concedido insertar filas.
//
// Aquí el servidor REPRODUCE ese desplazamiento en la propia hoja, como hace con sus
// rangos editables (proteccion_desplazar.go), y lo acepta solo si es EXACTAMENTE él:
//
//   - cada línea está donde la deja, con su bloqueo y su ocultación; las nuevas llegan
//     vacías y bloqueadas, y cada literal es el que había;
//   - cada fórmula es la de antes reescrita con él (`ReescribirCeldaEn`, el mismo código
//     que el lienzo nativo), también las que no cambian: una que no se reescribe, o que se
//     reescribe con otro, no es lo que produce una inserción;
//   - los rangos editables y las tablas dinámicas de la hoja quedan donde él los deja;
//   - y el MISMO desplazamiento explica las demás hojas protegidas que la citan
//     (proteccion_ajenas.go): uno solo por guardado para todo el libro.
//
// Solo cambia de otra manera lo que la persona podía editar antes y puede editar después.
// Ningún otro cambio de una celda bloqueada pasa por aquí.
//
// La búsqueda corre dentro de la transacción que bloquea el documento y está acotada
// como las otras dos: los candidatos salen de los bordes (lo que creció la hoja, sus
// rangos, sus dinámicas y los extremos de las referencias de las celdas que cambian),
// las líneas se comparan con holgura para las fórmulas y lo que dejan pasar se
// comprueba primero contra las fórmulas que cambian, con una reescritura por fórmula y
// efecto distinto, y como mucho unas pocas veces contra todo. Lo que se pasa de un tope
// se rechaza: rechazar de más nunca relaja la protección (el propietario sí puede).

// Topes de la búsqueda en la propia hoja: por encima se rechaza.
const (
	maxIntentosPropios       = 4096 // desplazamientos que llegan a mirar las fórmulas
	maxComprobacionesPropias = 4    // comprobaciones completas (toda la hoja y las demás)
)

// hojaPropia es lo que hace falta de una hoja protegida con un permiso estructural
// concedido para explicarla con un desplazamiento de ella misma: la hoja y su `protect`,
// antes y después, y quién escribe (sus rangos editables).
type hojaPropia struct {
	p, n, pp, np map[string]json.RawMessage
	quien        Subject
	// soloPropia: sus rangos editables cambian y no los explica nada más.
	soloPropia bool
}

// posCelda es una celda ya leída: columna y fila, base uno.
type posCelda struct{ c, f int }

// formulaPropia es una celda con fórmula de la hoja antes del desplazamiento, con los
// extremos (base cero, por eje) de sus referencias a la propia hoja.
type formulaPropia struct {
	pos      posCelda
	valor    string
	extremos [2][]int
}

// reescritura identifica la de una fórmula: dos desplazamientos con el mismo efecto
// sobre sus extremos la reescriben igual, y la misma fórmula en mil celdas, también.
type reescritura struct {
	valor string
	k     efecto
}

// desplazamientoPropio es lo que tiene que explicar el desplazamiento de la hoja `x`
// además de sus líneas y sus rangos (otrosDesplazados de rangosHojaMovidos): sus
// fórmulas, sus dinámicas y las demás hojas protegidas del libro.
type desplazamientoPropio struct {
	b   *busqueda
	x   *hojaAjena
	din *dinamicasPropias // nil sin tablas dinámicas
	// formulas son todas las de antes; cambian, las que no llegan tal cual a su sitio (las
	// primeras maxCeldasAjenas), que son el filtro barato.
	formulas, cambian []*formulaPropia
	nuevas            map[posCelda]string
	// abiertas: desbloqueadas antes y después; con los rangos de la persona, lo que podía
	// editar.
	abiertasAntes, abiertasDespues map[posCelda]bool
	viejos, nuevosQuien            RangosEditables
	// bordes de los candidatos (base uno, por eje): las referencias que cambian aquí y
	// en las demás hojas.
	bordesAntes, bordesDespues [2][]int
	hechas                     map[reescritura]string
	intentos, completas        int
	agotado                    bool
}

var _ otrosDesplazados = (*desplazamientoPropio)(nil)

// explicaPropia busca un desplazamiento de la hoja `x`, la que se mueve, que la explique
// entera y deje valer a todas las demás hojas protegidas (b.necesitan son las otras que
// lo necesitan).
func (b *busqueda) explicaPropia(x *hojaAjena) bool {
	antes, despues := b.extremosQueCambian()
	dp, ok := nuevoDesplazamientoPropio(b, x, antes, despues)
	if !ok {
		return false
	}
	b.citar()
	hp := x.propia
	return rangosHojaMovidos(hp.p, hp.n, hp.pp, hp.np, hp.quien, dp, formaDeValores()) && !dp.agotado
}

// nuevoDesplazamientoPropio lee la hoja `x` una vez: sus fórmulas, sus celdas nuevas, lo
// que la persona podía editar y los bordes de los candidatos. `antes` y `despues` son los
// extremos (base cero) de las referencias a ella en las otras hojas que cambian.
//
//nolint:gocognit,gocyclo // leer la hoja antes y después, en una pasada cada una.
func nuevoDesplazamientoPropio(b *busqueda, x *hojaAjena, antes, despues [2]map[int]bool) (*desplazamientoPropio, bool) {
	hp := x.propia
	ps, ok1 := estilos(hp.p)
	ns, ok2 := estilos(hp.n)
	if !ok1 || !ok2 {
		return nil, false
	}
	dp := &desplazamientoPropio{
		b: b, x: x, nuevas: map[posCelda]string{}, hechas: map[reescritura]string{},
		viejos: rangosDe(hp.pp, hp.quien), nuevosQuien: rangosDe(hp.np, hp.quien),
	}
	if !dinamicasVacias(hp.p["pivots"]) || !dinamicasVacias(hp.n["pivots"]) {
		din, ok := nuevasDinamicasPropias(x.nombre, hp.p, hp.n)
		if !ok {
			return nil, false
		}
		dp.din = din
	}
	var ok bool
	if dp.abiertasAntes, ok = abiertas(ps); !ok {
		return nil, false
	}
	if dp.abiertasDespues, ok = abiertas(ns); !ok {
		return nil, false
	}
	for ref, v := range x.nc {
		c, f, ok := leerCelda(ref)
		if !ok {
			return nil, false
		}
		dp.nuevas[posCelda{c, f}] = v
	}
	porRef := map[string]*formulaPropia{}
	for ref, v := range x.pc {
		if !celdaConFormula(v) {
			continue
		}
		c, f, ok := leerCelda(ref)
		if !ok {
			return nil, false
		}
		fp := &formulaPropia{pos: posCelda{c, f}, valor: v}
		suyos := extremosVacios()
		citaPropia(v, x.nombre, suyos)
		for e, xs := range suyos {
			fp.extremos[e] = ordenados(xs)
		}
		dp.formulas = append(dp.formulas, fp)
		porRef[ref] = fp
	}
	// Determinista: lo que se reescribe primero no depende del orden del mapa.
	slices.SortFunc(dp.formulas, func(a, b *formulaPropia) int {
		if a.pos.f != b.pos.f {
			return a.pos.f - b.pos.f
		}
		return a.pos.c - b.pos.c
	})
	// Las celdas con fórmula que no llegan tal cual: el filtro barato y los bordes.
	var refs []string
	for ref := range union(x.pc, x.nc) {
		if x.pc[ref] != x.nc[ref] && (celdaConFormula(x.pc[ref]) || celdaConFormula(x.nc[ref])) {
			refs = append(refs, ref)
		}
	}
	slices.Sort(refs)
	for i, ref := range refs {
		if i >= maxCeldasAjenas {
			break
		}
		citaPropia(x.pc[ref], x.nombre, antes)
		citaPropia(x.nc[ref], x.nombre, despues)
		if fp := porRef[ref]; fp != nil {
			dp.cambian = append(dp.cambian, fp)
		}
	}
	for e := range antes {
		if len(antes[e]) > maxExtremosAjena || len(despues[e]) > maxExtremosAjena {
			return nil, false
		}
		dp.bordesAntes[e], dp.bordesDespues[e] = baseUno(antes[e]), baseUno(despues[e])
	}
	return dp, true
}

// bordes: los de las referencias que cambian (aquí y en las demás hojas) y los de sus
// tablas dinámicas.
func (dp *desplazamientoPropio) bordes(filas bool) (antes, despues []int) {
	e := eje(filas)
	antes = append(antes, dp.bordesAntes[e]...)
	despues = append(despues, dp.bordesDespues[e]...)
	if dp.din != nil {
		a, d := dp.din.bordes(filas)
		antes, despues = append(antes, a...), append(despues, d...)
	}
	return antes, despues
}

// explica: con las líneas y los rangos de la hoja ya explicados por d, ¿lo explica todo?
// Sus dinámicas, sus fórmulas —primero las que cambian; luego, como mucho
// maxComprobacionesPropias veces, todas— y las demás hojas protegidas con el mismo.
func (dp *desplazamientoPropio) explica(d desplazamiento) bool {
	if dp.agotado {
		return false
	}
	dp.intentos++
	if dp.intentos > maxIntentosPropios {
		dp.agotado = true
		return false
	}
	b := dp.b
	// El permiso, en todas las hojas que se llaman como ella: una gemela sin él no se
	// mueve sin que se muevan las fórmulas que la citan.
	if !b.a.puede(b.hoja, d.permiso()) {
		return false
	}
	if dp.din != nil && !dp.din.explica(d) {
		return false
	}
	fd := formula.Desplazamiento{Filas: d.filas, Desde: d.desde - 1, Cantidad: d.n, Tope: d.tope(), Hoja: dp.x.nombre}
	if !dp.formulasBien(dp.cambian, d, fd) {
		return false
	}
	bien, dentro := b.cambianBien(fd)
	if !dentro {
		dp.agotado = true
		return false
	}
	if !bien || !b.quietasBien(fd) {
		return false
	}
	dp.completas++
	if dp.completas > maxComprobacionesPropias {
		dp.agotado = true
		return false
	}
	return dp.formulasBien(dp.formulas, d, fd) && b.todasValen(fd)
}

// formulasBien: ¿llega cada fórmula de la lista a donde la deja d, reescrita con él?
// Salvo donde la persona podía escribir antes y puede escribir después.
func (dp *desplazamientoPropio) formulasBien(lista []*formulaPropia, d desplazamiento, fd formula.Desplazamiento) bool {
	for _, f := range lista {
		destino, vive := moverCelda(f.pos, d)
		if !vive {
			continue // su fila o su columna ya no existe
		}
		dp.b.a.trabajo++
		r := reescritura{f.valor, efectoSobre(f.extremos[eje(d.filas)], fd)}
		quiero, hecha := dp.hechas[r]
		if !hecha {
			dp.b.reescrituras++
			if dp.b.reescrituras > maxReescriturasAjenas {
				dp.agotado = true
				return false
			}
			quiero = fd.ReescribirCeldaEn(f.valor, dp.x.nombre)
			dp.hechas[r] = quiero
		}
		if dp.nuevas[destino] == quiero {
			continue
		}
		antes := dp.abiertasAntes[f.pos] || dp.viejos.Contiene(f.pos.c, f.pos.f)
		despues := dp.abiertasDespues[destino] || dp.nuevosQuien.Contiene(destino.c, destino.f)
		if !antes || !despues {
			return false
		}
	}
	return true
}

// moverCelda es la posición de la celda tras d, o false si su fila o su columna se borra
// o se sale de la rejilla.
func moverCelda(p posCelda, d desplazamiento) (posCelda, bool) {
	i := &p.c
	if d.filas {
		i = &p.f
	}
	n, ok := d.mover(*i, dentroMuere)
	*i = n
	return p, ok
}

// abiertas son las celdas desbloqueadas (`locked: false`); false si alguna clave no es
// una celda.
func abiertas(est map[string]estilo) (map[posCelda]bool, bool) {
	out := map[posCelda]bool{}
	for ref, s := range est {
		c, f, ok := leerCelda(ref)
		if !ok {
			return nil, false
		}
		if s["locked"] == false {
			out[posCelda{c, f}] = true
		}
	}
	return out, true
}

// celdaConFormula: ¿es una fórmula el contenido de la celda? Lo mismo que mira
// ReescribirCeldaEn para decidir si la reescribe.
func celdaConFormula(html string) bool {
	return strings.Contains(html, "=") && strings.HasPrefix(formula.TextoCelda(html), "=")
}

// formaDeValores es la holgura con la que se comparan las líneas de la propia hoja:
// todas las fórmulas dan la misma forma (el desplazamiento solo cambia sus referencias, y
// lo exacto lo comprueba formulasBien); un literal, el suyo. Recuerda lo que ya ha visto:
// la búsqueda compara las mismas celdas una vez por candidato.
func formaDeValores() func(string) string {
	vistas := map[string]bool{}
	return func(v string) string {
		es, ok := vistas[v]
		if !ok {
			es = celdaConFormula(v)
			vistas[v] = es
		}
		if es {
			return "\x00="
		}
		return v
	}
}

// citaPropia anota en `m` los extremos (base cero, por eje) de las referencias de la
// celda a su propia hoja `nombre`: las que no llevan hoja y las que la nombran.
func citaPropia(html, nombre string, m [2]map[int]bool) {
	if !celdaConFormula(html) {
		return
	}
	for _, r := range formula.ExtractRefs(formula.TextoCelda(html)) {
		if r.Hoja != "" && (formula.EsDeOtroLibro(r.Hoja) || !strings.EqualFold(r.Hoja, nombre)) {
			continue
		}
		for _, filas := range []bool{true, false} {
			for _, x := range formula.Extremos(r.Texto, filas) {
				m[eje(filas)][x] = true
			}
		}
	}
}

// baseUno pasa un conjunto de extremos (base cero) a filas o columnas (base uno),
// ordenadas.
func baseUno(set map[int]bool) []int {
	out := ordenados(set)
	for i := range out {
		out[i]++
	}
	return out
}

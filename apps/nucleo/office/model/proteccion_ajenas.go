// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"maps"
	"slices"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// Al insertar o borrar filas en «Ventas», las fórmulas de las OTRAS hojas que la citan
// siguen a su dato (`moverReferenciasAjenas` de refshift.ts): «Resumen» con
// `=Ventas!B4` pasa a `=Ventas!B5`. Si «Resumen» está protegida, esas celdas suelen
// estar bloqueadas, y reescribirlas parecería editar contenido protegido. Pero no lo es:
// quien inserta la fila en «Ventas» no cambia lo que dice la fórmula, solo dónde está
// su dato. Negarlo dejaba la fórmula bloqueada sumando la fila de al lado sin avisar,
// que es justo lo que la protección debería impedir.
//
// Aquí el servidor acepta ese cambio solo si es EXACTAMENTE el desplazamiento:
//
//   - existe UNA inserción o UN borrado de filas o columnas en otra hoja H del libro,
//     que la persona podía hacer (todas las hojas que se llaman como H, sin distinguir
//     mayúsculas, sin proteger o protegidas con el permiso concedido);
//   - aplicado a TODAS las celdas de TODAS las hojas protegidas que la persona no puede
//     editar (`ReescribirCeldaEn`, el mismo código que usa el lienzo nativo), da lo que
//     llega, salvo donde la persona podía escribir de todos modos. Ni una celda bloqueada
//     más, ni una menos: reescribir unas y dejar otras no es lo que produce una
//     inserción, y se rechaza. Y es el MISMO para todas las hojas: «Resumen» y «Resumen2»
//     con `=Ventas!B4` no pueden pasar una a B5 y otra a B9, ni en un guardado ni en dos
//     (la que no cambia tiene que seguir valiendo con el desplazamiento que se alega).
//
// No se exige ver el desplazamiento en H, y no hace falta: quien puede insertar una fila
// en H puede insertarla y volver a escribir las celdas de H como estaban, así que el
// modelo resultante es uno que ya podía producir con acciones permitidas. Si H está
// protegida, la persona necesita su permiso, como para el desplazamiento real.
//
// La búsqueda corre dentro de la transacción que bloquea el documento, así que está
// acotada para que un modelo hecho a mano no la convierta en trabajo sin fin: solo la
// hoja cuyas referencias cambian, candidatos sacados de SUS extremos, primero contra
// las celdas que cambian, un candidato por efecto distinto y unas pocas comprobaciones
// completas. Lo que se pasa de un tope se rechaza: rechazar de más nunca relaja la
// protección (el propietario sí puede).
//
// Las tablas dinámicas que leen de la hoja que se mueve siguen la misma regla: una hoja
// cuyas dinámicas solo se explican así necesita el desplazamiento, y con él las de TODAS
// las hojas protegidas tienen que quedar exactamente como llegan (proteccion_dinamicas.go).
//
// ponytail: un desplazamiento por guardado; dos inserciones o borrados en un mismo
// guardado, en la misma hoja o en dos, se rechazan (el propietario sí puede). Por eso
// los clientes —la web (`useDocument.ts`) y Calcus de Fyne (`app.go`)— mandan cada
// cambio de estructura en su propio guardado, en orden, y no lo agrupan con otro.

// Topes de la búsqueda: por encima se rechaza.
const (
	maxCeldasAjenas         = 64      // celdas cambiadas por hoja de las que salen la hoja citada y los candidatos
	maxExtremosAjena        = 256     // extremos de referencia por eje
	maxHojasAjenas          = 64      // hojas protegidas que necesitan el desplazamiento a la vez
	maxReescriturasAjenas   = 1 << 16 // celdas reescritas al filtrar contra las que cambian
	maxComprobacionesAjenas = 4       // comprobaciones completas (todas las hojas, todas sus celdas)
)

// ajenasLibro reúne, en la transición de un modelo a otro, las hojas protegidas que la
// persona no puede editar: las que solo se explican con el desplazamiento de otra hoja y
// las demás, que tienen que seguir valiendo con ese mismo desplazamiento.
type ajenasLibro struct {
	prev, nuevo *libro
	hojas       []*hojaAjena
	// trabajo cuenta las celdas leídas o reescritas en la búsqueda: lo que cuesta, que
	// es lo que acotan los topes (lo mira el test de la saturación).
	trabajo int
}

// hojaAjena es una hoja protegida tal como la deja la regla de contenido (contenidoHoja).
type hojaAjena struct {
	nombre    string
	pc, nc    map[string]string
	bloqueada func(string) bool
	relajado  bool
	// necesita: su contenido no pasa (celdasPermitidas) sin un desplazamiento.
	necesita bool
	// cambian: las bloqueadas que cambian, en orden (solo si necesita).
	cambian []string
	// citas: lo que cita de la hoja que se mueve (se calcula al hacer falta).
	citas *citasHoja
	// din: sus tablas dinámicas (proteccion_dinamicas.go), que el mismo desplazamiento
	// tiene que dejar como llegan; din.necesita: cambian y solo lo explica él.
	din dinamicasHoja
	// dinExtremos y dinHechas: los extremos de los orígenes que leen de la hoja que se
	// mueve y, por efecto sobre ellos, si las deja como llegan (como celdaQueCambia).
	dinExtremos [2][]int
	dinHechas   map[efecto]bool
	// propia: con un permiso estructural concedido, lo que hace falta para explicarla con
	// un desplazamiento de ella misma (proteccion_propia.go); nil sin él o sin necesitarlo.
	propia *hojaPropia
}

// necesitaAlgo: su contenido o sus dinámicas solo se explican con el desplazamiento.
func (h *hojaAjena) necesitaAlgo() bool { return h.necesita || h.din.necesita || h.propia != nil }

// anotar guarda una hoja protegida ya comprobada en todo lo demás.
func (a *ajenasLibro) anotar(nombre string, pc, nc map[string]string, bloqueada func(string) bool, relajado, necesita bool, din dinamicasHoja, propia *hojaPropia) {
	h := &hojaAjena{nombre: nombre, pc: pc, nc: nc, bloqueada: bloqueada, relajado: relajado, necesita: necesita, din: din, propia: propia}
	if necesita {
		for ref := range union(pc, nc) {
			if pc[ref] != nc[ref] && bloqueada(ref) {
				h.cambian = append(h.cambian, ref)
			}
		}
		slices.Sort(h.cambian) // determinista: los candidatos no dependen del orden del mapa
	}
	a.hojas = append(a.hojas, h)
}

// resolver decide si las hojas que lo necesitan se explican con UN desplazamiento en otra
// hoja que deja valer a todas las demás. Devuelve el ámbito por el que se rechaza, o ""
// si vale.
func (a *ajenasLibro) resolver() string {
	slices.SortFunc(a.hojas, func(x, y *hojaAjena) int { return strings.Compare(x.nombre, y.nombre) })
	var necesitan []*hojaAjena
	for _, h := range a.hojas {
		if h.necesitaAlgo() {
			necesitan = append(necesitan, h)
		}
	}
	if len(necesitan) == 0 {
		return ""
	}
	rechazo := AmbitoHoja(necesitan[0].nombre)
	if len(necesitan) > maxHojasAjenas {
		return rechazo
	}
	hoja, propia, ok := hojaQueSeMueve(necesitan)
	if !ok {
		return rechazo
	}
	if propia != nil {
		// La que se mueve es una de ellas: sus propias reglas, y las demás con el mismo.
		otras := slices.DeleteFunc(slices.Clone(necesitan), func(h *hojaAjena) bool { return h == propia })
		b := &busqueda{a: a, hoja: hoja, real: a.nombreDe(hoja), necesitan: otras}
		if !b.explicaPropia(propia) {
			return rechazo
		}
		return ""
	}
	b := &busqueda{a: a, hoja: hoja, real: a.nombreDe(hoja), necesitan: necesitan}
	if !b.explica() {
		return rechazo
	}
	return ""
}

// puede: ¿podía la persona abrir o cerrar ese tramo (permiso de `desplazamiento`) en la
// hoja `hoja`? Una fórmula sigue a TODAS las hojas que se llaman así sin distinguir
// mayúsculas («Ventas» y «VENTAS»), así que hace falta en todas: con mirar solo la
// primera, una gemela sin proteger delante abría la protegida.
func (a *ajenasLibro) puede(hoja, permiso string) bool {
	if len(hojasLlamadas(a.nuevo, hoja)) == 0 {
		return false
	}
	for _, ph := range hojasLlamadas(a.prev, hoja) {
		if !protegido(ph["protect"]) {
			continue // sin proteger: cualquier editor puede
		}
		pp := map[string]json.RawMessage{}
		_ = json.Unmarshal(ph["protect"], &pp)
		if !dice(pp, permiso, true) {
			return false
		}
	}
	return true // las recién creadas, sin proteger
}

// nombreDe es el nombre de verdad de la hoja `hoja` en el libro nuevo, si solo hay una
// que se llame así sin distinguir mayúsculas; si no, `hoja` tal cual. El origen de una
// tabla dinámica se compara con él tal cual, como en la web.
func (a *ajenasLibro) nombreDe(hoja string) string {
	out, n := hoja, 0
	for _, nombre := range a.nuevo.nombres {
		if strings.EqualFold(nombre, hoja) {
			out, n = nombre, n+1
		}
	}
	if n != 1 {
		return hoja
	}
	return out
}

// crece: cuánto cambió la última fila (o columna) con contenido de esa hoja.
func (a *ajenasLibro) crece(hoja string, filas bool) int {
	ultima := func(l *libro) int {
		hs := hojasLlamadas(l, hoja)
		if len(hs) == 0 {
			return 0
		}
		cel, _ := celdas(hs[0])
		u := 0
		for ref := range cel {
			c, f, ok := leerCelda(ref)
			switch {
			case !ok:
			case filas:
				u = max(u, f)
			default:
				u = max(u, c)
			}
		}
		return u
	}
	return abs(ultima(a.nuevo) - ultima(a.prev))
}

// hojasLlamadas son las hojas del libro que se llaman `hoja` sin distinguir mayúsculas.
func hojasLlamadas(l *libro, hoja string) []map[string]json.RawMessage {
	if l == nil {
		return nil
	}
	var out []map[string]json.RawMessage
	for i, n := range l.nombres {
		if strings.EqualFold(n, hoja) {
			out = append(out, l.hojas[i])
		}
	}
	return out
}

// hojaQueSeMueve es la única hoja cuyas referencias cambian en las celdas bloqueadas que
// cambian. Un desplazamiento solo reescribe las referencias a SU hoja, así que una celda
// que cambia en dos hojas, en las suyas propias o fuera de las referencias no la explica
// ninguno; y todas tienen que cambiar en la misma, porque el desplazamiento es uno.
//
// Lo que no explica otra hoja puede explicarlo la propia, si tiene concedido mover sus
// celdas (proteccion_propia.go): entonces es ella la que se mueve, y se devuelve. Solo
// una: dos hojas que se mueven son dos desplazamientos.
func hojaQueSeMueve(necesitan []*hojaAjena) (hoja string, propia *hojaAjena, ok bool) {
	for _, h := range necesitan {
		dif, citada := "", false
		if h.propia == nil || !h.propia.soloPropia {
			dif, citada = h.hojaCitada()
		}
		if !citada || strings.EqualFold(dif, h.nombre) {
			if h.propia == nil || propia != nil {
				return "", nil, false
			}
			propia, dif = h, h.nombre
		}
		if hoja != "" && !strings.EqualFold(hoja, dif) {
			return "", nil, false
		}
		hoja = dif
	}
	if hoja == "" || formula.EsDeOtroLibro(hoja) {
		return "", nil, false
	}
	for _, h := range necesitan {
		if h != propia && strings.EqualFold(h.nombre, hoja) {
			return "", nil, false // sus propias referencias no se explican con otra hoja
		}
	}
	return hoja, propia, true
}

// hojaCitada es la única hoja cuyas referencias cambian en sus celdas bloqueadas que
// cambian y de la que leen sus dinámicas que cambian; false si no hay una sola.
func (h *hojaAjena) hojaCitada() (string, bool) {
	hoja := ""
	for i, ref := range h.cambian {
		if i >= maxCeldasAjenas {
			break
		}
		dif, ok := hojaQueCambia(h.pc[ref], h.nc[ref])
		if !ok || (hoja != "" && !strings.EqualFold(hoja, dif)) {
			return "", false
		}
		hoja = dif
	}
	if h.din.necesita {
		dif, ok := h.din.hojaCitada(h.nombre)
		if !ok || (hoja != "" && !strings.EqualFold(hoja, dif)) {
			return "", false
		}
		hoja = dif
	}
	return hoja, hoja != ""
}

// hojaQueCambia: la única hoja cuyas referencias (en orden) difieren entre las dos
// versiones de una celda; false si no hay ninguna, si hay más de una o si son las de la
// propia hoja (sin prefijo). Agrupar por minúsculas en vez de EqualFold puede separar
// dos nombres que una fórmula trata como uno: entonces se rechaza de más, nunca de menos,
// porque quien decide es la reescritura de verdad.
func hojaQueCambia(antes, despues string) (string, bool) {
	type lista struct {
		hoja string
		a, d []string
	}
	grupos := map[string]*lista{}
	var orden []string
	pon := func(html string, despues bool) {
		for _, r := range formula.ExtractRefs(formula.TextoCelda(html)) {
			k := strings.ToLower(r.Hoja)
			g := grupos[k]
			if g == nil {
				g = &lista{hoja: r.Hoja}
				grupos[k] = g
				orden = append(orden, k)
			}
			if despues {
				g.d = append(g.d, r.Texto)
			} else {
				g.a = append(g.a, r.Texto)
			}
		}
	}
	pon(antes, false)
	pon(despues, true)
	hoja, n := "", 0
	for _, k := range orden {
		if g := grupos[k]; !slices.Equal(g.a, g.d) {
			hoja, n = g.hoja, n+1
		}
	}
	return hoja, n == 1 && hoja != ""
}

// busqueda es la de un desplazamiento en `hoja` que explique a la vez todas las hojas.
type busqueda struct {
	a    *ajenasLibro
	hoja string
	// real es el nombre de verdad de `hoja` (nombreDe), con el que se mueven las dinámicas.
	real      string
	necesitan []*hojaAjena
	// cambian: las celdas bloqueadas que cambian de las que salen los candidatos.
	cambian []*celdaQueCambia
	// extremos de todas las hojas que citan `hoja`, por eje (0 filas, 1 columnas), ordenados.
	extremos [2][]int
	// reescrituras hechas al filtrar contra las celdas que cambian.
	reescrituras int
}

// celdaQueCambia es una celda bloqueada que cambia, con los extremos de sus referencias a
// la hoja que se mueve y, por efecto sobre ellos, si su reescritura da lo que llega: dos
// candidatos con el mismo efecto sobre la celda la reescriben igual, y miles de candidatos
// se quedan en unas pocas reescrituras.
type celdaQueCambia struct {
	h        *hojaAjena
	ref      string
	extremos [2][]int
	hechas   map[efecto]bool
}

// citasHoja es lo que una hoja protegida cita de la hoja que se mueve.
type citasHoja struct {
	// celdas: las que tienen alguna referencia a ella (las únicas que un desplazamiento
	// suyo puede cambiar).
	celdas []string
	// quietas: por eje, el mayor extremo citado desde una celda bloqueada que no cambia
	// (-1 si ninguno). Un desplazamiento que empiece en él o antes la cambiaría.
	quietas [2]int
}

func eje(filas bool) int {
	if filas {
		return 0
	}
	return 1
}

// explica prueba los candidatos: primero contra las celdas que cambian, luego uno por
// efecto distinto, y como mucho maxComprobacionesAjenas contra todo.
func (b *busqueda) explica() bool {
	cands, ok := b.candidatos()
	if !ok {
		return false
	}
	var vivos []formula.Desplazamiento
	for _, d := range cands {
		bien, dentro := b.cambianBien(d)
		if !dentro {
			return false // demasiado trabajo: se rechaza
		}
		if bien {
			vivos = append(vivos, d)
		}
	}
	if len(vivos) == 0 {
		return false
	}
	b.citar()
	vistos := map[efecto]bool{}
	completas := 0
	for _, d := range vivos {
		k := efectoSobre(b.extremos[eje(d.Filas)], d)
		if vistos[k] {
			continue
		}
		vistos[k] = true
		if !b.quietasBien(d) {
			continue
		}
		completas++
		if completas > maxComprobacionesAjenas {
			return false
		}
		if b.todasValen(d) {
			return true
		}
	}
	return false
}

// extremosQueCambian prepara las celdas bloqueadas que cambian (y las dinámicas que
// cambian) de las hojas que lo necesitan, y devuelve los extremos de sus referencias a
// la hoja que se mueve, antes y después, por eje.
func (b *busqueda) extremosQueCambian() (antes, despues [2]map[int]bool) {
	antes, despues = extremosVacios(), extremosVacios()
	for _, h := range b.necesitan {
		for i, ref := range h.cambian {
			if i >= maxCeldasAjenas {
				break
			}
			suyos := extremosVacios()
			citaEn(h.pc[ref], b.hoja, suyos)
			c := &celdaQueCambia{h: h, ref: ref, hechas: map[efecto]bool{}}
			for e, xs := range suyos {
				c.extremos[e] = ordenados(xs)
				maps.Copy(antes[e], xs)
			}
			b.cambian = append(b.cambian, c)
			citaEn(h.nc[ref], b.hoja, despues)
		}
		if h.din.necesita {
			suyos := extremosVacios()
			citaDinamicas(h.din.previas, h.nombre, b.hoja, suyos)
			for e, xs := range suyos {
				h.dinExtremos[e] = ordenados(xs)
				maps.Copy(antes[e], xs)
			}
			h.dinHechas = map[efecto]bool{}
			nuevas, _ := listaDinamicas(h.din.nuevas)
			citaDinamicas(nuevas, h.nombre, b.hoja, despues)
		}
	}
	return antes, despues
}

// candidatos son los desplazamientos en `hoja` que vale la pena probar, ya filtrados por
// permiso: sus cantidades y comienzos salen de los extremos de las referencias a ella en
// las celdas que cambian, antes y después (y de lo que creció o menguó la hoja).
func (b *busqueda) candidatos() ([]formula.Desplazamiento, bool) {
	antes, despues := b.extremosQueCambian()
	var out []formula.Desplazamiento
	for _, filas := range []bool{true, false} {
		ea, ed := antes[eje(filas)], despues[eje(filas)]
		if len(ea) > maxExtremosAjena || len(ed) > maxExtremosAjena {
			return nil, false
		}
		xa, xd := ordenados(ea), ordenados(ed)
		for _, m := range cantidades(xa, xd, b.a.crece(b.hoja, filas)) {
			for _, signo := range []int{1, -1} {
				d := desplazamiento{filas: filas, n: signo * m}
				if !b.a.puede(b.hoja, d.permiso()) {
					continue
				}
				// Como mucho 2 ejes × maxCandidatas × 2 signos × 5·maxExtremosAjena.
				for _, desde := range comienzos(xa, xd, m) {
					out = append(out, formula.Desplazamiento{
						Filas: filas, Desde: desde, Cantidad: signo * m, Tope: d.tope(), Hoja: b.hoja,
					})
				}
			}
		}
	}
	return out, true
}

// cambianBien: ¿las celdas bloqueadas que cambian (las primeras de cada hoja, de las que
// salió el candidato) son su reescritura? Cada celda se reescribe una vez por efecto
// distinto sobre ella. false en el segundo valor si se agota el presupuesto.
func (b *busqueda) cambianBien(d formula.Desplazamiento) (bien, dentro bool) {
	for _, c := range b.cambian {
		k := efectoSobre(c.extremos[eje(d.Filas)], d)
		v, hecha := c.hechas[k]
		if !hecha {
			b.a.trabajo++
			b.reescrituras++
			if b.reescrituras > maxReescriturasAjenas {
				return false, false
			}
			v = d.ReescribirCeldaEn(c.h.pc[c.ref], c.h.nombre) == c.h.nc[c.ref]
			c.hechas[k] = v
		}
		if !v {
			return false, true
		}
	}
	// Y las dinámicas que solo él explica, una vez por efecto sobre sus orígenes.
	for _, h := range b.necesitan {
		if !h.din.necesita {
			continue
		}
		k := efectoSobre(h.dinExtremos[eje(d.Filas)], d)
		v, hecha := h.dinHechas[k]
		if !hecha {
			b.a.trabajo += len(h.din.previas)
			v = h.din.valen(h.nombre, b.real, d)
			h.dinHechas[k] = v
		}
		if !v {
			return false, true
		}
	}
	return true, true
}

// citar lee, una vez, lo que cada hoja protegida cita de `hoja`: sus celdas, los extremos
// (para saber qué candidatos tienen el mismo efecto) y lo que no puede moverse.
func (b *busqueda) citar() {
	todos := extremosVacios()
	for _, h := range b.a.hojas {
		c := &citasHoja{quietas: [2]int{-1, -1}}
		h.citas = c
		if strings.EqualFold(h.nombre, b.hoja) {
			continue // la propia hoja que se mueve sigue sus reglas, no estas
		}
		// Los orígenes de sus dinámicas también cuentan para el efecto de un candidato.
		citaDinamicas(h.din.previas, h.nombre, b.hoja, todos)
		for ref, v := range h.pc {
			// Citar otra hoja exige un «!»: lo demás no se lee.
			if !strings.Contains(v, "!") {
				continue
			}
			b.a.trabajo++
			suyos := extremosVacios()
			if !citaEn(v, b.hoja, suyos) {
				continue
			}
			c.celdas = append(c.celdas, ref)
			quieta := h.bloqueada(ref) && h.nc[ref] == v
			for i, xs := range suyos {
				for x := range xs {
					todos[i][x] = true
					if quieta {
						c.quietas[i] = max(c.quietas[i], x)
					}
				}
			}
		}
		slices.Sort(c.celdas)
	}
	for i := range todos {
		b.extremos[i] = ordenados(todos[i])
	}
}

// extremosVacios es un conjunto de extremos por eje (0 filas, 1 columnas).
func extremosVacios() [2]map[int]bool { return [2]map[int]bool{{}, {}} }

// citaEn anota en `m` los extremos, por eje, de las referencias a `hoja` de la celda; dice
// si tiene alguna.
func citaEn(html, hoja string, m [2]map[int]bool) bool {
	cita := false
	for _, r := range formula.ExtractRefs(formula.TextoCelda(html)) {
		if !strings.EqualFold(r.Hoja, hoja) {
			continue
		}
		cita = true
		for _, filas := range []bool{true, false} {
			for _, x := range formula.Extremos(r.Texto, filas) {
				m[eje(filas)][x] = true
			}
		}
	}
	return cita
}

// efecto identifica lo que un desplazamiento hace a unas referencias a `hoja` (las de una
// celda, o las de todas las hojas protegidas): depende solo de qué extremos quedan antes
// del corte, dentro de lo borrado o después, y, si alguno cae dentro, de dónde empieza
// (ahí se pegan). Dos candidatos con el mismo efecto las reescriben igual: basta con
// probar uno.
type efecto struct {
	filas                   bool
	cantidad, antes, borrar int
	desde                   int
}

// efectoSobre es el efecto de d sobre los extremos xs (ordenados) de su eje.
func efectoSobre(xs []int, d formula.Desplazamiento) efecto {
	k := efecto{filas: d.Filas, cantidad: d.Cantidad, antes: sort.SearchInts(xs, d.Desde), borrar: -1, desde: -1}
	if d.Cantidad < 0 {
		k.borrar = sort.SearchInts(xs, d.Desde-d.Cantidad)
		if k.borrar > k.antes {
			k.desde = d.Desde
		}
	}
	return k
}

// quietasBien descarta sin reescribir nada un candidato que movería una referencia de
// una celda bloqueada que no cambia en una hoja sin permisos estructurales: allí eso
// es exactamente lo que celdasPermitidas rechaza. Con permisos estructurales el contenido
// puede cambiar de sitio y decide la comprobación completa.
func (b *busqueda) quietasBien(d formula.Desplazamiento) bool {
	for _, h := range b.a.hojas {
		if !h.relajado && h.citas.quietas[eje(d.Filas)] >= d.Desde {
			return false
		}
	}
	return true
}

// todasValen: con el desplazamiento aplicado, ¿pasa el contenido de cada hoja protegida
// con las reglas de siempre? Las celdas que no citan `hoja` no cambian y no se tocan;
// las fórmulas repetidas se reescriben una vez.
func (b *busqueda) todasValen(d formula.Desplazamiento) bool {
	for _, h := range b.a.hojas {
		// Unas dinámicas que no se leen no se sabe si las mueve, ni en la propia hoja que
		// se mueve: sus celdas bloqueadas pueden no cambiar y sus dinámicas, sí.
		if !h.din.legibles {
			return false
		}
		// Sus dinámicas, cambien o no, como las deja el desplazamiento (la propia hoja
		// que se mueve las explica con sus reglas, en contenidoHoja).
		if !strings.EqualFold(h.nombre, b.hoja) {
			b.a.trabajo += len(h.din.previas)
			if !h.din.valen(h.nombre, b.real, d) {
				return false
			}
		}
		if len(h.citas.celdas) == 0 {
			continue // no la cita (o es ella): ya pasó tal cual
		}
		movidas := maps.Clone(h.pc)
		hechas := map[string]string{}
		for _, ref := range h.citas.celdas {
			v := h.pc[ref]
			nv, ok := hechas[v]
			if !ok {
				b.a.trabajo++
				nv = d.ReescribirCeldaEn(v, h.nombre)
				hechas[v] = nv
			}
			movidas[ref] = nv
		}
		if !celdasPermitidas(movidas, h.nc, h.bloqueada, h.relajado) {
			return false
		}
	}
	return true
}

// cantidades: lo que creció la hoja citada y lo que se movió cada extremo.
func cantidades(antes, despues []int, crece int) []int {
	set := map[int]bool{}
	for _, x := range antes {
		for _, y := range despues {
			if x != y {
				set[abs(x-y)] = true
			}
		}
	}
	delete(set, crece)
	out := make([]int, 0, len(set)+1)
	if crece > 0 {
		out = append(out, crece) // la primera: es la de casi siempre
	}
	resto := ordenados(set)
	out = append(out, resto...)
	return out[:min(len(out), maxCandidatas)]
}

// comienzos: dónde puede empezar un tramo de m que explique esos extremos (base cero).
// Al insertar, en el primer extremo que se mueve; al borrar, justo antes del que se
// mueve (x-m), en el que se pega al borde (j, j+1) o en uno de los dos bordes del que
// muere (x-m+1, x).
func comienzos(antes, despues []int, m int) []int {
	set := map[int]bool{}
	for _, x := range antes {
		for _, c := range []int{x, x - m, x - m + 1} {
			if c >= 0 {
				set[c] = true
			}
		}
	}
	for _, j := range despues {
		set[j], set[j+1] = true, true
	}
	return ordenados(set)
}

func ordenados(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

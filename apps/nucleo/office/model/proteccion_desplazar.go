// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"reflect"
	"slices"
)

// Los rangos editables (`protect.ranges`) VIAJAN CON SUS CELDAS al insertar o borrar
// filas y columnas (decisión D-CALC-02). Viven dentro de `protect`, así que cambiarlos
// es cambiar la protección y solo lo puede el propietario o quien trae la contraseña
// (transicion en proteccion.go). Pero si quien tiene concedido insertar filas no puede
// moverlos, el rango se queda atrás: tras insertar una fila encima, el rango de Ana
// apunta a la fila de arriba —con el contenido bloqueado que ha bajado a ella— y deja
// fuera la última de las suyas. Eso relaja la protección justo donde no debe.
//
// Aquí el servidor REPRODUCE el desplazamiento: un cambio de `ranges` sin autorización
// solo se acepta si existe una inserción o un borrado de filas o columnas, con su permiso
// concedido, que explica a la vez
//
//   - los rangos nuevos: los de antes movidos con las reglas de refshift.ts
//     (`moverReferencia`: crecen, encogen o desaparecen), en el mismo orden y sin tocar
//     nada más de cada uno (personas, grupos, título);
//   - el contenido: cada celda está donde la deja ese desplazamiento, salvo las que la
//     persona podía editar antes y puede editar después;
//   - el bloqueo y la ocultación: cada estilo `locked`/`hidden` sigue a su celda, y las
//     filas o columnas nuevas nacen bloqueadas.
//
// Lo segundo y lo tercero son lo que impide estirar un rango fingiendo una inserción
// que no ha ocurrido: si las celdas no se han movido, no hay desplazamiento que las
// explique. Un modelo indistinguible del que produce una inserción real es, a todos los
// efectos, esa inserción, que la persona podía hacer.
//
// ponytail: el desplazamiento se busca con candidatos (el crecimiento de la hoja y lo que
// se movieron los rangos); un borrado que se lleva rangos enteros más allá de todo el
// contenido, junto a ediciones en el mismo guardado, puede no encontrarlo y se rechaza
// (el propietario sí puede). Rechazar de más nunca relaja la protección.

// desplazamiento es el `Desplazamiento` de refshift.ts en coordenadas 1-based.
type desplazamiento struct {
	filas bool // eje: filas o columnas
	desde int  // primera fila o columna afectada
	n     int  // >0 se insertan, <0 se borran
}

// permiso es el del cuadro «Proteger hoja» que lo autoriza.
func (d desplazamiento) permiso() string {
	switch {
	case d.filas && d.n > 0:
		return "insertRows"
	case d.filas:
		return "deleteRows"
	case d.n > 0:
		return "insertCols"
	default:
		return "deleteCols"
	}
}

func (d desplazamiento) tope() int {
	if d.filas {
		return maxFila
	}
	return maxColumna
}

// Cómo trata `mover` una posición que cae en lo borrado (`Dentro` de refshift.ts).
const (
	dentroMuere = iota
	dentroSujeta
	dentroSujetaFin
)

// mover es `mover` de refshift.ts: la nueva posición, o false si deja de existir.
func (d desplazamiento) mover(i, dentro int) (int, bool) {
	if i < d.desde {
		return i, true
	}
	if d.n > 0 {
		nuevo := i + d.n
		return nuevo, nuevo <= d.tope()
	}
	m := -d.n
	if i < d.desde+m {
		switch dentro {
		case dentroSujeta:
			return d.desde, true
		case dentroSujetaFin:
			return d.desde - 1, true
		default:
			return 0, false
		}
	}
	return i - m, true
}

// moverRango es `moverReferencia` sobre un rango ya leído. Un eje ausente (columna o
// fila entera) no se toca. El rango muere si se le borran sus dos extremos o si se sale
// de la rejilla.
func (d desplazamiento) moverRango(r rangoEditable) (rangoEditable, bool) {
	a, b := &r.c1, &r.c2
	if d.filas {
		a, b = &r.f1, &r.f2
	}
	if *a == 0 {
		return r, true
	}
	_, vivaA := d.mover(*a, dentroMuere)
	_, vivaB := d.mover(*b, dentroMuere)
	if !vivaA && !vivaB {
		return rangoEditable{}, false
	}
	na, ok1 := d.mover(*a, dentroSujeta)
	nb, ok2 := d.mover(*b, dentroSujetaFin)
	if !ok1 || !ok2 {
		return rangoEditable{}, false
	}
	*a, *b = na, nb
	return r, true
}

// rangosCrudos lee `protect.ranges` como lista de objetos; false si no lo es.
func rangosCrudos(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	if isNullJSON(raw) {
		return nil, true
	}
	var l []map[string]json.RawMessage
	if json.Unmarshal(raw, &l) != nil {
		return nil, false
	}
	return l, true
}

// refDe es la referencia de un rango y si el servidor la entiende.
func refDe(g map[string]json.RawMessage) (rangoEditable, bool) {
	var ref string
	if json.Unmarshal(g["ref"], &ref) != nil {
		return rangoEditable{}, false
	}
	return leerRangoEditable(ref)
}

// rangosMovidos: ¿son `nuevos` exactamente los `previos` movidos por d?
func rangosMovidos(previos, nuevos []map[string]json.RawMessage, d desplazamiento) bool {
	j := 0
	for _, p := range previos {
		rp, legible := refDe(p)
		if !legible {
			// Una referencia que el servidor no lee no concede nada (deny-by-default) y
			// no se sabe moverla: tiene que seguir sin leerse y con todo lo demás igual.
			if j >= len(nuevos) || !reflect.DeepEqual(decodificar(p, "ref"), decodificar(nuevos[j], "ref")) {
				return false
			}
			if _, ok := refDe(nuevos[j]); ok {
				return false
			}
			j++
			continue
		}
		rn, vive := d.moverRango(rp)
		if !vive {
			continue // su dato ya no existe: el rango desaparece con él
		}
		if j >= len(nuevos) || !reflect.DeepEqual(decodificar(p, "ref"), decodificar(nuevos[j], "ref")) {
			return false
		}
		if got, ok := refDe(nuevos[j]); !ok || got != rn {
			return false
		}
		j++
	}
	return j == len(nuevos)
}

// celdaHoja es una posición de la hoja con su contenido y sus marcas de protección.
type celdaHoja struct {
	valor           string
	abierta, oculta bool // locked:false, hidden:true
}

// lineas agrupa las celdas de una hoja por fila (o columna) y, dentro, por la otra
// coordenada. false si alguna clave no es una celda, o si dos claves («B5» y « b5»)
// nombran la misma: no se razona sobre lo que no se lee, y con dos claves para una
// posición cuál de las dos se compara dependería del orden del mapa, así que un
// modelo hecho a mano podría enseñar aquí una y al editor la otra.
func lineas(cel map[string]string, est map[string]estilo, filas bool) (map[int]map[int]celdaHoja, bool) {
	out := map[int]map[int]celdaHoja{}
	type pos struct{ c, f int }
	vistas := map[pos]bool{}
	pon := func(ref string, f func(*celdaHoja)) bool {
		c, fl, ok := leerCelda(ref)
		if !ok || vistas[pos{c, fl}] {
			return false
		}
		vistas[pos{c, fl}] = true
		l, o := fl, c
		if !filas {
			l, o = c, fl
		}
		if out[l] == nil {
			out[l] = map[int]celdaHoja{}
		}
		x := out[l][o]
		f(&x)
		out[l][o] = x
		return true
	}
	for ref, v := range cel {
		if !pon(ref, func(x *celdaHoja) { x.valor = v }) {
			return nil, false
		}
	}
	clear(vistas) // celdas y estilos se cuentan aparte: una posición tiene uno de cada
	for ref, s := range est {
		if !pon(ref, func(x *celdaHoja) { x.abierta, x.oculta = s["locked"] == false, s["hidden"] == true }) {
			return nil, false
		}
	}
	return out, true
}

// hojaEnLineas es una hoja (la previa o la nueva) ya agrupada, con los rangos que la
// persona tiene concedidos en ella. `forma`, en la previa, compara los valores con
// holgura: dos que dan la misma forma pueden venir uno del otro, y lo que los distingue
// lo comprueba exactamente quien la pone (las fórmulas de la propia hoja reescritas con
// el desplazamiento, proteccion_propia.go). Sin ella, los valores se comparan tal cual.
type hojaEnLineas struct {
	l      map[int]map[int]celdaHoja
	rangos RangosEditables
	filas  bool
	forma  func(string) string
}

// editable: ¿puede la persona escribir en la posición `o` de la línea `ln`?
func (h hojaEnLineas) editable(ln, o int) bool {
	if h.l[ln][o].abierta {
		return true
	}
	if h.filas {
		return h.rangos.Contiene(o, ln)
	}
	return h.rangos.Contiene(ln, o)
}

// lineaCoincide: ¿puede la línea `ln` del modelo nuevo venir de la `lp` del previo?
// lp == 0 es una línea recién insertada; lp fuera de la rejilla, una que llega vacía.
// Las marcas de bloqueo y ocultación viajan tal cual (las nuevas nacen bloqueadas), y
// el contenido solo cambia donde la persona podía escribir antes y puede escribir ahora.
func lineaCoincide(prev, nuevo hojaEnLineas, lp, ln int) bool {
	origen, destino := prev.l[lp], nuevo.l[ln]
	coincide := func(o int) bool {
		p, q := origen[o], destino[o]
		if p.abierta != q.abierta || p.oculta != q.oculta {
			return false
		}
		if p.valor == q.valor || (prev.forma != nil && prev.forma(p.valor) == prev.forma(q.valor)) {
			return true
		}
		return (lp <= 0 || prev.editable(lp, o)) && nuevo.editable(ln, o)
	}
	for o := range destino {
		if !coincide(o) {
			return false
		}
	}
	for o := range origen {
		if _, ok := destino[o]; !ok && !coincide(o) {
			return false
		}
	}
	return true
}

// otrosDesplazados es lo que, además de los rangos y del contenido, tiene que explicar
// el MISMO desplazamiento de la hoja protegida: sus tablas dinámicas
// (proteccion_dinamicas.go). Sus bordes (base uno) dan candidatos a la búsqueda.
type otrosDesplazados interface {
	bordes(filas bool) (antes, despues []int)
	explica(d desplazamiento) bool
}

// rangosHojaMovidos decide si el cambio de `protect.ranges` de una hoja protegida lo
// explica una inserción o un borrado con su permiso concedido (ver arriba). pp y np son
// el `protect` previo y el nuevo; p y n, la hoja. Con `otros`, el mismo desplazamiento
// tiene que explicarlos también (los rangos pueden no haber cambiado: entonces tienen
// que seguir donde ese desplazamiento los deja). Con `forma`, los valores de las líneas
// se comparan con esa holgura y `otros` comprueba exactamente lo que deja pasar.
func rangosHojaMovidos(p, n, pp, np map[string]json.RawMessage, quien Subject, otros otrosDesplazados, forma func(string) string) bool {
	previos, ok1 := rangosCrudos(pp["ranges"])
	nuevos, ok2 := rangosCrudos(np["ranges"])
	pc, ok3 := celdas(p)
	nc, ok4 := celdas(n)
	ps, ok5 := estilos(p)
	ns, ok6 := estilos(n)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || len(previos) > maxRangosDesplazables || len(nuevos) > maxRangosDesplazables {
		return false
	}
	viejos, nuevosQuien := rangosDe(pp, quien), rangosDe(np, quien)
	for _, filas := range []bool{true, false} {
		lp, okp := lineas(pc, ps, filas)
		ln, okn := lineas(nc, ns, filas)
		if !okp || !okn {
			return false
		}
		prev := hojaEnLineas{l: lp, rangos: viejos, filas: filas, forma: forma}
		nuevo := hojaEnLineas{l: ln, rangos: nuevosQuien, filas: filas}
		var cl *comparacionLineas // lo que no depende del desplazamiento, al hacer falta
		for _, cant := range candidatas(lp, ln, previos, nuevos, filas, otros) {
			for _, signo := range []int{1, -1} {
				d := desplazamiento{filas: filas, n: signo * cant}
				if !dice(pp, d.permiso(), true) {
					continue
				}
				if cl == nil {
					cl = compararLineas(prev, nuevo, d.tope())
				}
				if buscarDesde(d, cl, previos, nuevos, otros) {
					return true
				}
			}
		}
	}
	return false
}

// comparacionLineas es lo que buscarDesde comparte entre los candidatos de un eje: lo
// que no depende del desplazamiento. Quedarse o ser nueva solo puede fallar en una línea
// con algo, antes o después, así que eso se mira una vez; moverse, en cada candidato.
type comparacionLineas struct {
	prev, nuevo hojaEnLineas
	// hi: la primera línea que no puede quedarse donde estaba (el tope si ninguna); el
	// desplazamiento empieza en ella o antes.
	hi int
	// nuevasMal: las líneas, en orden, que no pueden ser recién insertadas.
	nuevasMal []int
	// las líneas con algo, de la última a la primera.
	clavesNuevo, clavesPrev []int
}

func compararLineas(prev, nuevo hojaEnLineas, tope int) *comparacionLineas {
	cl := &comparacionLineas{prev: prev, nuevo: nuevo, hi: tope}
	descendentes := func(l map[int]map[int]celdaHoja) []int {
		out := make([]int, 0, len(l))
		for k := range l {
			out = append(out, k)
		}
		slices.Sort(out)
		slices.Reverse(out)
		return out
	}
	cl.clavesNuevo, cl.clavesPrev = descendentes(nuevo.l), descendentes(prev.l)
	todas := append(slices.Clone(cl.clavesNuevo), cl.clavesPrev...)
	slices.Sort(todas)
	for _, k := range slices.Compact(todas) {
		if !lineaCoincide(prev, nuevo, k, k) {
			cl.hi = k // la primera: las de encima ya no importan
			break
		}
	}
	for _, k := range slices.Backward(cl.clavesNuevo) {
		if !lineaCoincide(prev, nuevo, 0, k) {
			cl.nuevasMal = append(cl.nuevasMal, k)
		}
	}
	return cl
}

// loMover es el primer comienzo posible de d según las líneas que se mueven: cada línea
// desde el comienzo (más lo insertado) tiene que venir de la suya. Basta con la última
// que no puede: se recorren de la última a la primera y se para en ella.
func (cl *comparacionLineas) loMover(d desplazamiento) int {
	tope := d.tope()
	i, j := 0, 0
	for i < len(cl.clavesNuevo) || j < len(cl.clavesPrev) {
		// La siguiente línea, de mayor a menor, entre las que tienen algo después y las
		// que lo reciben de antes al moverse.
		k := -1
		if i < len(cl.clavesNuevo) {
			k = cl.clavesNuevo[i]
		}
		if j < len(cl.clavesPrev) {
			if movida := cl.clavesPrev[j] + d.n; movida > tope {
				j++
				continue
			} else if movida >= k {
				if movida == k {
					i++
				}
				k = movida
				j++
			} else {
				i++
			}
		} else {
			i++
		}
		if k < 1 {
			break
		}
		// Lo que llega de fuera de la rejilla llega vacío: una línea que nadie tiene.
		origen := k - d.n
		if origen < 1 || origen > tope {
			origen = -1
		}
		if !lineaCoincide(cl.prev, cl.nuevo, origen, k) {
			if d.n > 0 {
				return max(1, k-d.n+1) // moverse vale desde comienzo+n
			}
			return k + 1 // al borrar, moverse vale desde el comienzo
		}
	}
	return 1
}

// candidatas son las cantidades de filas (o columnas) que vale la pena probar: lo que
// creció o menguó la hoja y lo que se movió cada extremo de cada rango.
func candidatas(lp, ln map[int]map[int]celdaHoja, previos, nuevos []map[string]json.RawMessage, filas bool, otros otrosDesplazados) []int {
	extremos := func(l map[int]map[int]celdaHoja, rs []map[string]json.RawMessage) (int, []int) {
		ult := 0
		for k := range l {
			ult = max(ult, k)
		}
		var bordes []int
		for _, g := range rs {
			if r, ok := refDe(g); ok {
				a, b := r.c1, r.c2
				if filas {
					a, b = r.f1, r.f2
				}
				if a > 0 {
					bordes = append(bordes, a, b)
					ult = max(ult, b)
				}
			}
		}
		return ult, bordes
	}
	up, bp := extremos(lp, previos)
	un, bn := extremos(ln, nuevos)
	if otros != nil {
		a, b := otros.bordes(filas)
		bp, bn = append(bp, a...), append(bn, b...)
	}
	set := map[int]bool{}
	for _, x := range bp {
		for _, y := range bn {
			if x != y {
				set[abs(x-y)] = true
			}
		}
	}
	delete(set, abs(up-un))
	out := make([]int, 0, len(set)+1)
	if up != un {
		out = append(out, abs(up-un)) // la primera: es la de casi siempre
	}
	resto := make([]int, 0, len(set))
	for k := range set {
		resto = append(resto, k)
	}
	slices.Sort(resto)
	out = append(out, resto...)
	return out[:min(len(out), maxCandidatas)]
}

// Topes para que un modelo hecho a mano no convierta la búsqueda en trabajo sin fin:
// por encima se rechaza (rechazar de más nunca relaja la protección). Un libro real
// tiene unos pocos rangos y una cantidad evidente.
const (
	maxRangosDesplazables = 64
	maxCandidatas         = 16
)

// buscarDesde busca la fila (o columna) donde empieza el desplazamiento de cantidad
// d.n. El contenido acota un intervalo de comienzos posibles (cada línea solo puede
// quedarse, moverse o ser nueva); los rangos, dentro de él, solo cambian de resultado en
// sus bordes, así que basta con probar esos puntos.
func buscarDesde(d desplazamiento, cl *comparacionLineas, previos, nuevos []map[string]json.RawMessage, otros otrosDesplazados) bool {
	m := abs(d.n)
	lo, hi := cl.loMover(d), cl.hi
	if lo > hi {
		return false
	}
	var nuevasMal []int
	if d.n > 0 {
		nuevasMal = cl.nuevasMal
	}
	valido := func(r int) bool {
		if r < lo || r > hi {
			return false
		}
		// Ninguna de las que no pueden ser nuevas en lo insertado.
		i, _ := slices.BinarySearch(nuevasMal, r)
		return i == len(nuevasMal) || nuevasMal[i] >= r+m
	}
	cand := []int{lo}
	for _, f := range nuevasMal {
		cand = append(cand, f+1)
	}
	var bordes []int
	for _, g := range previos {
		if r, ok := refDe(g); ok {
			a, b := r.c1, r.c2
			if d.filas {
				a, b = r.f1, r.f2
			}
			bordes = append(bordes, a, b)
		}
	}
	if otros != nil {
		a, _ := otros.bordes(d.filas)
		bordes = append(bordes, a...)
	}
	for _, x := range bordes {
		for _, dx := range []int{-m, -m + 1, -m + 2, -1, 0, 1} {
			cand = append(cand, x+dx)
		}
	}
	probados := map[int]bool{}
	for _, r := range cand {
		if probados[r] || !valido(r) {
			continue
		}
		probados[r] = true
		d.desde = r
		if rangosMovidos(previos, nuevos, d) && (otros == nil || otros.explica(d)) {
			return true
		}
	}
	return false
}

// abs es el valor absoluto de un entero: la distancia entre dos bordes de filas.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

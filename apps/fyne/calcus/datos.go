// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// Las operaciones de la pestaña Datos que tocan el libro: ordenar, quitar duplicados,
// texto en columnas e insertar y borrar filas y columnas. Son las de SheetCanvas.tsx,
// cada una en UN paso de deshacer.

// filaDeCeldas es lo que se lleva una fila al moverse: el HTML, el formato de número y
// el estilo de cada celda del tramo de columnas.
type filaDeCeldas []celdaCopia

func (l *Libro) leerFila(f, c1, c2 int) filaDeCeldas {
	out := make(filaDeCeldas, 0, c2-c1+1)
	for c := c1; c <= c2; c++ {
		out = append(out, l.celdaCopia(c, f))
	}
	return out
}

func (l *Libro) ponerFila(f, c1 int, fila filaDeCeldas) {
	h := l.h()
	for i, cc := range fila {
		k := Celda{C: c1 + i, F: f}
		l.poner(l.f.activa, k.C, k.F, cc.html)
		delete(h.formatos, k)
		delete(h.estilos, k)
		if cc.num != nil {
			h.formatos[k] = *cc.num
		}
		if cc.est != nil {
			h.estilos[k] = *cc.est
		}
	}
}

// usado es el rectángulo que ocupan las celdas con algo de la hoja activa.
func (l *Libro) usado() (c1, f1, c2, f2 int, ok bool) {
	c1, f1, c2, f2 = MaxColumnas, MaxFilas, -1, -1
	for k := range l.h().celdas {
		c1, f1, c2, f2 = min(c1, k.C), min(f1, k.F), max(c2, k.C), max(f2, k.F)
	}
	return c1, f1, c2, f2, c2 >= 0
}

// compararOrden es `sortCellCompare`: las vacías siempre al final; luego números,
// textos, booleanos y errores, por ese orden; y dentro de cada clase, la comparación
// del motor. `dir` es 1 (ascendente) o −1.
func compararOrden(a, b formula.Value, dir int) int {
	va, vb := a == "", b == ""
	if va || vb {
		switch {
		case va == vb:
			return 0
		case va:
			return 1
		}
		return -1
	}
	rango := func(v formula.Value) int {
		switch v.(type) {
		case formula.Error:
			return 3
		case float64:
			return 0
		case string:
			return 1
		}
		return 2
	}
	ra, rb := rango(a), rango(b)
	if ra != rb {
		return dir * (ra - rb)
	}
	if ra == 3 {
		return 0
	}
	c, err := formula.CompareValues(a, b)
	if err != "" {
		return 0
	}
	return dir * c
}

// Ordenar es el comando `sort`: las filas del rango por la columna de la celda ancla.
// Con una celda suelta se ordena el rectángulo usado de la hoja entera. Las fórmulas se
// mueven con su texto tal cual, como en Excel. Devuelve false si no había nada que mover.
func (l *Libro) Ordenar(desc bool, ancla Celda, c1, f1, c2, f2 int) bool {
	if c1 == c2 && f1 == f2 {
		var ok bool
		if c1, f1, c2, f2, ok = l.usado(); !ok || f1 == f2 {
			return false
		}
	}
	dir := 1
	if desc {
		dir = -1
	}
	clave := clamp(ancla.C-c1, c2-c1) + c1
	orden := make([]int, 0, f2-f1+1)
	for f := f1; f <= f2; f++ {
		orden = append(orden, f)
	}
	slices.SortStableFunc(orden, func(a, b int) int { return compararOrden(l.Valor(clave, a), l.Valor(clave, b), dir) })
	if slices.IsSorted(orden) {
		return false // ya estaba ordenado
	}
	filas := make([]filaDeCeldas, len(orden))
	for i, desde := range orden {
		filas[i] = l.leerFila(desde, c1, c2)
	}
	l.anotar()
	for i, fila := range filas {
		l.ponerFila(f1+i, c1, fila)
	}
	return true
}

// QuitarDuplicados es el comando `dedupe`: deja la primera de cada grupo de filas
// iguales —comparando lo que SE VE—, sube las demás y vacía lo que sobra al final.
// Devuelve cuántas quitó.
func (l *Libro) QuitarDuplicados(c1, f1, c2, f2 int) int {
	if f1 == f2 {
		return 0
	}
	vistas := map[string]bool{}
	var quedan []int
	for f := f1; f <= f2; f++ {
		partes := make([]string, 0, c2-c1+1)
		for c := c1; c <= c2; c++ {
			partes = append(partes, formula.Display(l.Valor(c, f)))
		}
		k := strings.Join(partes, "\x00")
		if !vistas[k] {
			vistas[k] = true
			quedan = append(quedan, f)
		}
	}
	quitadas := f2 - f1 + 1 - len(quedan)
	if quitadas == 0 {
		return 0
	}
	filas := make([]filaDeCeldas, len(quedan))
	for i, desde := range quedan {
		filas[i] = l.leerFila(desde, c1, c2)
	}
	l.anotar()
	for i, fila := range filas {
		l.ponerFila(f1+i, c1, fila)
	}
	vacia := make(filaDeCeldas, c2-c1+1)
	for f := f1 + len(quedan); f <= f2; f++ {
		l.ponerFila(f, c1, vacia)
	}
	return quitadas
}

// TextoEnColumnas es el comando `text2cols`: el texto de la primera columna del rango,
// partido por el separador, repartido hacia la derecha. Las fórmulas no se parten.
func (l *Libro) TextoEnColumnas(sep string, c1, f1, f2 int) {
	if sep == "" {
		sep = "\t"
	}
	nuevo := map[Celda]string{}
	for f := f1; f <= f2; f++ {
		html := l.HTML(c1, f)
		texto := textoPlano(html)
		if texto == "" || esFormula(html) {
			continue
		}
		partes := strings.Split(texto, sep)
		if len(partes) <= 1 {
			continue
		}
		for i, p := range partes {
			if c1+i >= MaxColumnas {
				break
			}
			nuevo[Celda{C: c1 + i, F: f}] = escaparHTML(strings.TrimSpace(p))
		}
	}
	if len(nuevo) == 0 {
		return
	}
	l.anotar()
	for k, html := range nuevo {
		l.poner(l.f.activa, k.C, k.F, html)
	}
}

// Estructura es el comando `insrow`, `inscol`, `delrow` o `delcol` sobre el rango: se
// mueven las celdas con su formato y se reescriben las fórmulas de la hoja activa, y las
// de las otras hojas que la citan, para que sigan apuntando a su dato
// (`formula.Desplazamiento`, las reglas de Excel).
// `lado` es "above"/"below" o "left"/"right" al insertar.
func (l *Libro) Estructura(cmd, lado string, c1, f1, c2, f2 int) {
	var d formula.Desplazamiento
	switch cmd {
	case "insrow":
		d = formula.Desplazamiento{Filas: true, Desde: f1, Cantidad: 1, Tope: MaxFilas}
		if lado == "below" {
			d.Desde = f2 + 1
		}
	case "delrow":
		d = formula.Desplazamiento{Filas: true, Desde: f1, Cantidad: -(f2 - f1 + 1), Tope: MaxFilas}
	case "inscol":
		d = formula.Desplazamiento{Desde: c1, Cantidad: 1, Tope: MaxColumnas}
		if lado == "right" {
			d.Desde = c2 + 1
		}
	case "delcol":
		d = formula.Desplazamiento{Desde: c1, Cantidad: -(c2 - c1 + 1), Tope: MaxColumnas}
	default:
		return
	}
	mover := func(k Celda) (Celda, bool) {
		i := k.C
		if d.Filas {
			i = k.F
		}
		n, ok := moverIndice(i, d)
		if d.Filas {
			return Celda{C: k.C, F: n}, ok
		}
		return Celda{C: n, F: k.F}, ok
	}
	l.antesDeEstructura(true)
	l.anotar()
	l.pasado[len(l.pasado)-1].estructural = true
	h := l.h()
	// Solo se mueven las referencias que apuntan a esta hoja, estén donde estén.
	d.Hoja = h.nombre
	viejas, formatos, estilos := h.celdas, h.formatos, h.estilos
	h.celdas, h.formatos, h.estilos = map[Celda]string{}, map[Celda]FormatoNum{}, map[Celda]Estilo{}
	for k, html := range viejas {
		if n, ok := mover(k); ok {
			h.celdas[n] = reescribirCelda(html, d)
		}
	}
	for k, v := range formatos {
		if n, ok := mover(k); ok {
			h.formatos[n] = v
		}
	}
	for k, v := range estilos {
		if n, ok := mover(k); ok {
			h.estilos[n] = v
		}
	}
	moverObjetos(h, d)
	// La identidad de las filas o columnas sigue a las suyas (`conEstructura`): un
	// comentario anclado a B4 pasa a B5 al insertar una fila encima.
	if d.Filas {
		h.filasA = h.filasA.mover(d.Desde, d.Cantidad)
	} else {
		h.colsA = h.colsA.mover(d.Desde, d.Cantidad)
	}
	// Las ocultas a mano siguen a su fila o columna, contra los límites de la web.
	if d.Filas {
		h.ocultFilas = tramosONil(analisis.MoverTramos(h.ocultFilas, d.Desde, d.Cantidad, MaxFilas))
	} else {
		h.ocultCols = tramosONil(analisis.MoverTramos(h.ocultCols, d.Desde, d.Cantidad, MaxColumnas))
	}
	for i := range l.f.hojas {
		if i != l.f.activa {
			moverReferenciasAjenas(&l.f.hojas[i], d)
		}
	}
	l.f.nombres = moverNombres(l.f.nombres, h.nombre, l.f.hojas[0].nombre, d)
	l.restaurar(l.f)
	l.trasEstructura(true)
}

// moverReferenciasAjenas es `moverReferenciasAjenas` de refshift.ts: en una hoja que NO
// es la del desplazamiento, las fórmulas que la citan (`=Ventas!B4`) siguen a su dato
// —las celdas y las del formato condicional y las validaciones; sus rangos son de esta
// hoja y no se mueven—. Antes «Resumen» se quedaba mirando la celda vieja sin avisar.
//
// Con la hoja protegida se reescriben TODAS las celdas, también las bloqueadas: seguir a
// su dato no es editarlas, y el servidor lo acepta cuando el cambio es exactamente este
// desplazamiento (apps/nucleo/office/model/proteccion_ajenas.go). Las reglas siguen
// necesitando `objects`, como en `moverObjetos`.
func moverReferenciasAjenas(h *hojaLibro, d formula.Desplazamiento) {
	if strings.EqualFold(h.nombre, d.Hoja) {
		return
	}
	var celdas map[Celda]string
	for k, html := range h.celdas {
		if nuevo := reescribirCeldaEn(html, d, h.nombre); nuevo != html {
			if celdas == nil {
				celdas = maps.Clone(h.celdas)
			}
			celdas[k] = nuevo
		}
	}
	if celdas != nil {
		h.celdas = celdas
	}
	// Las tablas dinámicas que leen de aquella hoja (`sourceSheet`) mueven su origen.
	h.dinamicas = moverDinamicas(h, d, false)
	if h.proteccion != nil && !h.proteccion.Objects {
		return
	}
	// Las reglas son inmutables y la foto las comparte: se ponen listas nuevas.
	if len(h.condiciones) > 0 {
		conds := slices.Clone(h.condiciones)
		for i := range conds {
			conds[i].Formula = d.ReescribirFormulaEn(conds[i].Formula, h.nombre)
		}
		h.condiciones = conds
	}
	if len(h.validaciones) > 0 {
		vals := slices.Clone(h.validaciones)
		for i := range vals {
			vals[i].Formula = d.ReescribirFormulaEn(vals[i].Formula, h.nombre)
		}
		h.validaciones = vals
	}
}

// moverObjetos es `moverObjetos` de `refshift.ts`: las tablas, los minigráficos, los
// gráficos, las tablas dinámicas (`moverDinamicas`), las segmentaciones, el autofiltro, el formato condicional, las validaciones,
// el esquema, las combinadas y los anchos y altos siguen a sus celdas con las reglas de
// las fórmulas; lo que se queda sin dato desaparece.
//
// Con la hoja protegida, lo que gobierna un permiso no concedido no se toca —el formato
// condicional, las validaciones, los gráficos, los minigráficos y las segmentaciones sin
// `objects`; el autofiltro sin `autoFilter`—: el servidor lo veta
// (services/docs/internal/domain/proteccion.go) y rechazaría el guardado entero, celdas
// movidas incluidas. Los rangos editables (`protect.ranges`) sí viajan con sus celdas
// (`moverRangos`).
//
//nolint:gocognit // una rama por clase de objeto, como `moverObjetos` de refshift.ts
func moverObjetos(h *hojaLibro, d formula.Desplazamiento) {
	vivo := func(ref string) (string, bool) {
		r := d.MoverReferencia(ref)
		return r, r != formula.RefRota
	}
	columna := func(c int) (int, bool) { return moverColumna(c, d) }
	h.proteccion = moverRangos(h.proteccion, vivo)
	moverEstructura(h, d, vivo)
	h.tablas = moverTablas(h.tablas, vivo)
	h.dinamicas = moverDinamicas(h, d, true)
	if h.proteccion == nil || h.proteccion.AutoFilter {
		h.filtro = moverAutofiltro(h.filtro, vivo, columna)
	}
	if h.proteccion != nil && !h.proteccion.Objects {
		return
	}
	moverReglas(h, d, vivo)
	var minis []Minigrafico
	for _, m := range h.minis {
		celda, ok1 := vivo(m.Cell)
		rango, ok2 := vivo(m.Range)
		if ok1 && ok2 {
			m.Cell, m.Range = celda, rango
			minis = append(minis, m)
		}
	}
	var graficos []Grafico
	for _, g := range h.graficos {
		if r, ok := vivo(g.Range); ok {
			g.Range = r
			graficos = append(graficos, g)
		}
	}
	var segs []Segmentacion
	for _, s := range h.segs {
		if s.Pivot != "" { // su Col es un campo del origen, no una columna
			segs = append(segs, s)
			continue
		}
		if c, ok := columna(s.Col); ok {
			s.Col = c
			segs = append(segs, s)
		}
	}
	h.minis, h.graficos, h.segs = minis, graficos, segs
}

// moverRangos es la parte de `moverObjetos` de refshift.ts de los rangos editables
// (`protect.ranges`): viajan con sus celdas aunque la hoja esté protegida. Si se quedaran
// atrás, el rango de alguien pasaría a cubrir el contenido bloqueado que ha llegado a sus
// filas y dejaría fuera parte del suyo. Crecen, encogen o desaparecen como cualquier
// rango, y sin ninguno `ranges` no se escribe. El servidor lo acepta de quien no es el
// propietario porque reproduce el mismo desplazamiento
// (apps/nucleo/office/model/proteccion_desplazar.go).
//
// La protección es inmutable —la foto de deshacer la comparte—, así que se pone otra; y
// la misma si ningún rango cambia.
func moverRangos(p *Proteccion, vivo func(string) (string, bool)) *Proteccion {
	if p == nil || len(p.Ranges) == 0 {
		return p
	}
	cambia := false
	var rangos []model.RangoEditable
	for _, g := range p.Ranges {
		r, ok := vivo(g.Ref)
		if !ok {
			cambia = true
			continue
		}
		if r = compacto(r); r != g.Ref {
			g.Ref, cambia = r, true
		}
		rangos = append(rangos, g)
	}
	if !cambia {
		return p
	}
	q := *p
	q.Ranges = rangos
	return &q
}

// moverDinamicas es `moverDinamicas` de refshift.ts: el origen de cada tabla dinámica
// sigue a la hoja de la que lee (la suya, o `SourceSheet`, comparado tal cual como en
// hojaOrigen) y, en la hoja que la aloja (`propia`), su esquina y lo que ocupó. Sin
// origen, o sin esquina ni nada de lo que ocupaba, desaparece; sin esquina pero con
// algo de lo que ocupaba, pasa a la primera celda que queda. Con la hoja protegida se
// mueve igual: seguir a sus datos no es cambiarla, y el servidor acepta exactamente este
// desplazamiento (apps/nucleo/office/model/proteccion_dinamicas.go). Devuelve la misma
// lista si nada cambia (la foto de deshacer la comparte).
func moverDinamicas(h *hojaLibro, d formula.Desplazamiento, propia bool) []analisis.Dinamica {
	if len(h.dinamicas) == 0 {
		return h.dinamicas
	}
	vivo := func(ref string) (string, bool) {
		r := d.MoverReferencia(ref)
		return r, r != formula.RefRota
	}
	cambia := false
	var out []analisis.Dinamica
	for _, p := range h.dinamicas {
		q := p
		origen := p.SourceSheet
		if origen == "" {
			origen = h.nombre
		}
		if d.Hoja == "" || origen == d.Hoja {
			src, ok := vivo(p.Source)
			if !ok {
				cambia = true
				continue
			}
			q.Source = src
		}
		if propia {
			rango, conRango := "", false
			if p.Range != "" {
				rango, conRango = vivo(p.Range)
			}
			esquina, ok := vivo(p.Anchor)
			if !ok && conRango {
				esquina, _, _ = strings.Cut(rango, ":")
				ok = true
			}
			if !ok {
				cambia = true
				continue
			}
			q.Anchor, q.Range = esquina, ""
			if conRango {
				q.Range = rango
			}
		}
		if q.Source != p.Source || q.Anchor != p.Anchor || q.Range != p.Range {
			cambia = true
		}
		out = append(out, q)
	}
	if !cambia {
		return h.dinamicas
	}
	return out
}

func moverTablas(ts []TablaFmt, vivo func(string) (string, bool)) []TablaFmt {
	var out []TablaFmt
	for _, t := range ts {
		if r, ok := vivo(t.Range); ok {
			t.Range = r
			out = append(out, t)
		}
	}
	return out
}

// moverColumna es dónde va una columna (las filas no la tocan), o false si deja de existir.
func moverColumna(c int, d formula.Desplazamiento) (int, bool) {
	if d.Filas {
		return c, true
	}
	return moverIndice(c, d)
}

// reCombinada es la combinada que de verdad combina algo: dos extremos con fila y columna.
var reCombinada = regexp.MustCompile(`[A-Z]+\d+:[A-Z]+\d+`)

// moverEstructura es lo que ningún permiso gobierna: el esquema, las combinadas y los
// anchos y altos, que siguen a sus filas y columnas pase lo que pase.
func moverEstructura(h *hojaLibro, d formula.Desplazamiento, vivo func(string) (string, bool)) {
	if d.Filas {
		h.grupFilas = moverGrupos(h.grupFilas, d)
		h.altos = moverMedidas(h.altos, d)
	} else {
		h.grupCols = moverGrupos(h.grupCols, d)
		h.anchos = moverMedidas(h.anchos, d)
	}
	var fusiones []string
	for _, m := range h.fusiones {
		// Una combinada que queda en una sola celda ya no combina nada.
		if r, ok := vivo(m); ok && reCombinada.MatchString(r) && !esUnaCelda(r) {
			fusiones = append(fusiones, r)
		}
	}
	h.fusiones = fusiones
}

// moverReglas es el formato condicional y las validaciones: su rango, con las reglas de
// un rango, y su fórmula, con `moverFormulaAnclada`.
func moverReglas(h *hojaLibro, d formula.Desplazamiento, vivo func(string) (string, bool)) {
	var conds []ReglaCond
	for _, c := range h.condiciones {
		r, ok := vivo(c.Rango)
		if !ok {
			continue
		}
		if c.Formula != "" {
			c.Formula = moverFormulaAnclada(c.Formula, c.Rango, d)
		}
		c.Rango = compacto(r)
		conds = append(conds, c)
	}
	var vals []Validacion
	for _, v := range h.validaciones {
		r, ok := vivo(v.Rango)
		if !ok {
			continue
		}
		if v.Formula != "" {
			v.Formula = moverFormulaAnclada(v.Formula, v.Rango, d)
		}
		v.Rango = compacto(r)
		vals = append(vals, v)
	}
	h.condiciones, h.validaciones = conds, vals
}

// esUnaCelda dice si el rango «A1:B2» es en realidad una sola celda.
func esUnaCelda(r string) bool {
	a, b, ok := strings.Cut(r, ":")
	return ok && a == b
}

// compacto es `compacto`: un rango que se ha quedado en una celda se escribe como la
// celda, «B5» y no «B5:B5».
func compacto(r string) string {
	if esUnaCelda(r) {
		a, _, _ := strings.Cut(r, ":")
		return a
	}
	return r
}

// moverFormulaAnclada es `moverFormulaAnclada` de refshift.ts: la fórmula de una regla
// está escrita respecto a la celda de arriba a la izquierda de su rango; si lo borrado se
// lleva esa celda, la fórmula pasa a ser la de la primera superviviente —se «copia» a
// ella, como el autorrelleno— antes de aplicar el desplazamiento. Si no, borrar la primera
// fila de `=B2>5` sobre `B2:B10` la dejaría en `=#¡REF!>5`, apagada.
func moverFormulaAnclada(f, rango string, d formula.Desplazamiento) string {
	r, ok := formula.ParseRangeRef(rango)
	if ok && strings.HasPrefix(f, "=") && d.Cantidad < 0 {
		n, ancla := -d.Cantidad, r.C1
		if d.Filas {
			ancla = r.R1
		}
		if ancla >= d.Desde && ancla < d.Desde+n {
			salto := d.Desde + n - ancla
			dc, dr := salto, 0
			if d.Filas {
				dc, dr = 0, salto
			}
			if copiada := formula.ShiftRefs(f, dc, dr); strings.HasPrefix(copiada, "=") {
				return d.ReescribirFormula(copiada)
			}
		}
	}
	return d.ReescribirFormula(f)
}

// moverGrupos es el esquema tras el desplazamiento: cada grupo, con las reglas de un
// rango, crece si se inserta dentro de su detalle, encoge si se le borra un trozo y
// desaparece si se le borra entero. Su fila de resumen (end+1) queda fuera.
func moverGrupos(gs []analisis.Grupo, d formula.Desplazamiento) []analisis.Grupo {
	var out []analisis.Grupo
	for _, g := range gs {
		ref := "A" + strconv.Itoa(g.Start+1) + ":A" + strconv.Itoa(g.End+1)
		if !d.Filas {
			ref = formula.NumToCol(g.Start) + "1:" + formula.NumToCol(g.End) + "1"
		}
		r, ok := formula.ParseRangeRef(d.MoverReferencia(ref))
		if !ok {
			continue
		}
		g.Start, g.End = r.C1, r.C2
		if d.Filas {
			g.Start, g.End = r.R1, r.R2
		}
		out = append(out, g)
	}
	return out
}

// moverMedidas es `medidas`: los anchos o los altos personalizados siguen a su columna o
// a su fila; los de lo borrado se van con ello. nil si no queda ninguno.
func moverMedidas(m map[string]float64, d formula.Desplazamiento) map[string]float64 {
	var out map[string]float64
	for k, v := range m {
		i, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		if n, ok := moverIndice(i, d); ok {
			if out == nil {
				out = map[string]float64{}
			}
			out[strconv.Itoa(n)] = v
		}
	}
	return out
}

// moverAutofiltro es el autofiltro tras el desplazamiento: su fila de cabecera sigue a sus
// celdas y los criterios —valores excluidos y condiciones—, a su columna. nil si la
// cabecera deja de existir.
func moverAutofiltro(af *Autofiltro, vivo func(string) (string, bool), columna func(int) (int, bool)) *Autofiltro {
	if af == nil {
		return nil
	}
	r, ok := vivo(Ref(af.C1, af.Row) + ":" + Ref(af.C2, af.Row))
	if !ok {
		return nil
	}
	k, _ := formula.ParseRangeRef(r)
	nuevo := &Autofiltro{Row: k.R1, C1: k.C1, C2: k.C2}
	for clave, lista := range af.Excl {
		c, err := strconv.Atoi(clave)
		if err != nil || len(lista) == 0 {
			continue
		}
		if n, ok := columna(c); ok {
			if nuevo.Excl == nil {
				nuevo.Excl = map[string][]string{}
			}
			nuevo.Excl[strconv.Itoa(n)] = lista
		}
	}
	for clave, cond := range af.Cond {
		c, err := strconv.Atoi(clave)
		if err != nil {
			continue
		}
		if n, ok := columna(c); ok {
			if nuevo.Cond == nil {
				nuevo.Cond = map[string]CondFiltro{}
			}
			nuevo.Cond[strconv.Itoa(n)] = cond
		}
	}
	return nuevo
}

// moverIndice es el `mapPos` de `applyStructural`: dónde va una celda, o false si cae en
// lo borrado o se sale de la rejilla.
func moverIndice(i int, d formula.Desplazamiento) (int, bool) {
	if d.Cantidad > 0 {
		if i < d.Desde {
			return i, true
		}
		return i + d.Cantidad, i+d.Cantidad < d.Tope
	}
	n := -d.Cantidad
	switch {
	case i < d.Desde:
		return i, true
	case i < d.Desde+n:
		return 0, false
	}
	return i - n, true
}

// reescribirCelda es `reescribirCelda`: la fórmula de la celda con sus referencias
// movidas, conservando el envoltorio. Un literal sale tal cual.
func reescribirCelda(html string, d formula.Desplazamiento) string {
	return reescribirCeldaEn(html, d, d.Hoja)
}

// reescribirCeldaEn es `reescribirCelda` con la hoja en la que vive la celda (`base`).
// Sin cambios, el HTML sale tal cual, como en la web.
func reescribirCeldaEn(html string, d formula.Desplazamiento, base string) string {
	return d.ReescribirCeldaEn(html, base)
}

// Rellenar es `applyFill` (el asa de autorrelleno): copia el bloque origen sobre el
// destino en MOSAICO —no es una serie: «1» arrastrado hacia abajo es «1, 1, 1»—,
// desplazando las referencias relativas de las fórmulas como al pegar; el formato de
// número y el visual viajan tal cual. El destino está a un lado del origen, en UNA
// dirección.
func (l *Libro) Rellenar(origen, destino [4]int) {
	oc1, of1, oc2, of2 := origen[0], origen[1], origen[2], origen[3]
	dc1, df1, dc2, df2 := destino[0], destino[1], destino[2], destino[3]
	nc, nf := oc2-oc1+1, of2-of1+1
	type pieza struct {
		k  Celda
		cc celdaCopia
	}
	var piezas []pieza
	for f := df1; f <= df2; f++ {
		for c := dc1; c <= dc2; c++ {
			sc, sf := c, f
			switch {
			case df1 > of2:
				sf = of1 + (f-of1)%nf // hacia abajo
			case df2 < of1:
				sf = of2 - (of2-f)%nf // hacia arriba
			case dc1 > oc2:
				sc = oc1 + (c-oc1)%nc // a la derecha
			default:
				sc = oc2 - (oc2-c)%nc // a la izquierda
			}
			cc := l.celdaCopia(sc, sf)
			cc.html = desplazarCelda(cc.html, c-sc, f-sf)
			piezas = append(piezas, pieza{Celda{C: c, F: f}, cc})
		}
	}
	l.anotar()
	for _, p := range piezas {
		l.ponerFila(p.k.F, p.k.C, filaDeCeldas{p.cc})
	}
}

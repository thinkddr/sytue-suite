// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
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
	c1, f1, c2, f2 = Columnas, Filas, -1, -1
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
			if c1+i >= Columnas {
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
// mueven las celdas con su formato y se reescriben las fórmulas de la hoja activa para
// que sigan apuntando a su dato (`formula.Desplazamiento`, las reglas de Excel).
// `lado` es "above"/"below" o "left"/"right" al insertar.
func (l *Libro) Estructura(cmd, lado string, c1, f1, c2, f2 int) {
	var d formula.Desplazamiento
	switch cmd {
	case "insrow":
		d = formula.Desplazamiento{Filas: true, Desde: f1, Cantidad: 1, Tope: Filas}
		if lado == "below" {
			d.Desde = f2 + 1
		}
	case "delrow":
		d = formula.Desplazamiento{Filas: true, Desde: f1, Cantidad: -(f2 - f1 + 1), Tope: Filas}
	case "inscol":
		d = formula.Desplazamiento{Desde: c1, Cantidad: 1, Tope: Columnas}
		if lado == "right" {
			d.Desde = c2 + 1
		}
	case "delcol":
		d = formula.Desplazamiento{Desde: c1, Cantidad: -(c2 - c1 + 1), Tope: Columnas}
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
	l.anotar()
	h := l.h()
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
	l.f.nombres = moverNombres(l.f.nombres, h.nombre, l.f.hojas[0].nombre, d)
	l.restaurar(l.f)
}

// moverObjetos es `moverObjetos` de `refshift.ts`: las tablas, los minigráficos, los
// gráficos, las segmentaciones y el autofiltro siguen a sus celdas con las reglas de las
// fórmulas; lo que se queda sin dato desaparece.
//
//nolint:gocognit // una rama por clase de objeto, como `moverObjetos` de refshift.ts
func moverObjetos(h *hojaLibro, d formula.Desplazamiento) {
	vivo := func(ref string) (string, bool) {
		r := d.MoverReferencia(ref)
		return r, r != formula.RefRota
	}
	var tablas []TablaFmt
	for _, t := range h.tablas {
		if r, ok := vivo(t.Range); ok {
			t.Range = r
			tablas = append(tablas, t)
		}
	}
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
	columna := func(c int) (int, bool) {
		if d.Filas {
			return c, true
		}
		return moverIndice(c, d)
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
	h.tablas, h.minis, h.graficos, h.segs = tablas, minis, graficos, segs
	h.filtro = moverAutofiltro(h.filtro, vivo, columna)
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
	if html == "" || !esFormula(html) {
		return html
	}
	p := leerCelda(html)
	p.dentro = escaparHTML(d.ReescribirFormula(textoPlano(html)))
	return p.html()
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

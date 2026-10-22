// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// LAS ÓRDENES DEL EDITOR SOBRE EL MODELO. Son las de RichText.tsx —`execCommand` del
// navegador más las de la tarea—, aplicadas a `Documento` en vez de al DOM, y puras: no
// pintan nada, así que se prueban sin ventana.
//
// Una posición es (bloque, letra): la letra cuenta en runas del texto del bloque, con un
// `<br>` como una letra. Es la misma cuenta que usa el maquetador (`Trozo.Desde`).
//
// Lo que se escribe es lo que escribe Chromium con la barra de la web, para que una nota
// editada aquí se abra igual allí: un párrafo nuevo es un `<div>` (el separador por
// defecto del navegador), partir un `<p>` da dos `<p>`, al final de un título sigue un
// `<div>`, y la negrita es `<b>`, la cursiva `<i>`, el subrayado `<u>` y el tachado `<s>`.

// Pos es un punto del documento: el bloque y la letra dentro de su texto.
type Pos struct{ B, O int }

// antes dice si a va antes que b.
func (a Pos) antes(b Pos) bool { return a.B < b.B || (a.B == b.B && a.O < b.O) }

// ordenar devuelve los dos extremos de una selección en orden.
func ordenar(a, b Pos) (Pos, Pos) {
	if b.antes(a) {
		return b, a
	}
	return a, b
}

// largoTramo son las letras de un tramo: un salto es una, y una imagen, otra.
func largoTramo(t Tramo) int {
	if t.Salto || t.Imagen != nil {
		return 1
	}
	return utf8.RuneCountInString(t.Texto)
}

// largo son las letras de un bloque.
func largo(b Bloque) int {
	n := 0
	for _, t := range b.Tramos {
		n += largoTramo(t)
	}
	return n
}

// copia es un documento que se puede cambiar sin tocar el original: los bloques y sus
// tramos son nuevos; las listas de marcas se comparten porque nunca se cambian en su
// sitio (se sustituyen).
func (d *Documento) copia() *Documento {
	c := &Documento{Bloques: make([]Bloque, len(d.Bloques)), original: d.original, fiel: d.fiel, Imagenes: d.Imagenes, desplegado: d.desplegado}
	for i, b := range d.Bloques {
		b.Tramos = slices.Clone(b.Tramos)
		c.Bloques[i] = b
	}
	return c
}

// inicioEditable es la primera letra en la que se puede escribir: en una tarea, detrás
// del blanco que la web deja entre la casilla y su `<span>` (en el navegador ese blanco
// es un hijo de un `flex` y no se ve ni se pisa).
func inicioEditable(b Bloque) int {
	if b.Tipo != Tarea {
		return 0
	}
	n := 0
	for _, t := range b.Tramos {
		if t.EnSpan || t.Salto || t.Imagen != nil || strings.TrimSpace(t.Texto) != "" {
			break
		}
		n += largoTramo(t)
	}
	return n
}

// partirEn deja una frontera de tramo en la letra `o` del bloque y devuelve el índice del
// tramo que empieza ahí (len si es el final).
func partirEn(b *Bloque, o int) int {
	pos := 0
	for i, t := range b.Tramos {
		n := largoTramo(t)
		switch {
		case o == pos:
			return i
		case o < pos+n && !t.Salto && t.Imagen == nil:
			r := []rune(t.Texto)
			izq, der := t, t
			izq.Texto, der.Texto = string(r[:o-pos]), string(r[o-pos:])
			b.Tramos = slices.Insert(b.Tramos, i+1, der)
			b.Tramos[i] = izq
			return i + 1
		}
		pos += n
	}
	return len(b.Tramos)
}

// mismoEstilo dice si dos tramos se pintan y se escriben igual (para juntarlos).
func mismoEstilo(a, b Tramo) bool {
	return !a.Salto && !b.Salto && a.Imagen == nil && b.Imagen == nil && slices.Equal(a.Marcas, b.Marcas) &&
		a.EnSpan == b.EnSpan && a.Color == b.Color && a.Fondo == b.Fondo && a.Fuente == b.Fuente &&
		a.Tamano == b.Tamano && a.Enlace == b.Enlace && a.Externo == b.Externo
}

// limpiar quita los tramos vacíos y junta los contiguos del mismo estilo.
func limpiar(b *Bloque) {
	out := b.Tramos[:0]
	for _, t := range b.Tramos {
		if !t.Salto && t.Imagen == nil && t.Texto == "" {
			continue
		}
		if n := len(out); n > 0 && mismoEstilo(out[n-1], t) {
			out[n-1].Texto += t.Texto
			continue
		}
		out = append(out, t)
	}
	b.Tramos = out
}

// estiloEn es el estilo que hereda lo que se escribe en `o`: el de la letra de ANTES,
// como en el navegador; al principio del bloque, el de la primera.
func estiloEn(b Bloque, o int) Tramo {
	pos := 0
	var previo *Tramo
	for i := range b.Tramos {
		t := b.Tramos[i]
		n := largoTramo(t)
		texto := !t.Salto && t.Imagen == nil // una imagen no da estilo a lo que se escribe
		if texto && o > pos && o <= pos+n {
			return sinTexto(t)
		}
		if texto && previo == nil && o <= pos {
			previo = &b.Tramos[i]
		}
		pos += n
	}
	if previo != nil {
		return sinTexto(*previo)
	}
	if b.Tipo == Tarea {
		return Tramo{EnSpan: true}
	}
	return Tramo{}
}

func sinTexto(t Tramo) Tramo {
	t.Texto, t.Salto, t.Imagen = "", false, nil
	return t
}

// Insertar escribe `s` en `p` con el estilo `estilo` y devuelve dónde queda el cursor.
// Un «\n» es un salto de línea dentro del bloque (`<br>`), que es lo que el navegador
// hace al pegar texto con varias líneas.
func (d *Documento) Insertar(p Pos, s string, estilo Tramo) Pos {
	if len(d.Bloques) == 0 {
		// El primer texto de una nota vacía va SUELTO, sin envoltorio: es lo que deja
		// el navegador en un `contenteditable` recién estrenado.
		d.Bloques = []Bloque{{Tipo: Parrafo}}
		p = Pos{}
	}
	b := &d.Bloques[p.B]
	p.O = max(p.O, inicioEditable(*b))
	// El `&nbsp;` que la web deja en una tarea vacía es un hueco, no texto: se pisa.
	if b.Tipo == Tarea && soloHueco(*b) {
		b.Tramos = slices.DeleteFunc(b.Tramos, func(t Tramo) bool { return t.EnSpan })
		p.O = inicioEditable(*b)
	}
	// Y el de una celda nueva (`nuevaCelda` de la web), igual: está para que la celda
	// no se colapse, y lo que se escribe va en su lugar.
	if b.Celda.EnTabla() && b.Texto() == "\u00a0" {
		b.Tramos, p.O = nil, 0
	}
	if b.Tipo == Tarea {
		estilo.EnSpan = true // lo escrito en una tarea va en su `<span>`
	}
	i := partirEn(b, p.O)
	var nuevos []Tramo
	for k, linea := range strings.Split(s, "\n") {
		if k > 0 {
			nuevos = append(nuevos, Tramo{Salto: true})
		}
		if linea != "" {
			t := estilo
			t.Texto, t.Salto = linea, false
			nuevos = append(nuevos, t)
		}
	}
	b.Tramos = slices.Insert(b.Tramos, i, nuevos...)
	limpiar(b)
	return Pos{p.B, p.O + utf8.RuneCountInString(s)}
}

// InsertarDocumento pega `otro` en `p` y devuelve dónde queda el cursor, que es lo que
// hace el navegador al pegar HTML en un `contenteditable`:
//
//   - Un solo bloque de texto (lo más común: unas palabras en negrita, un enlace) entra
//     en la línea del cursor, con sus marcas.
//   - Varios bloques parten el actual por el cursor: el primero pegado se une a lo que
//     había delante si es texto, y lo que había detrás del cursor sigue al último.
//
// `otro` no se toca: se pegan copias de sus bloques.
//
//nolint:gocyclo // Cada forma de pegado conserva una semántica de cursor distinta.
func (d *Documento) InsertarDocumento(p Pos, otro *Documento) Pos {
	if otro == nil || len(otro.Bloques) == 0 {
		return p
	}
	nuevos := otro.copia().Bloques
	if len(d.Bloques) == 0 {
		d.Bloques = nuevos
		u := len(d.Bloques) - 1
		return Pos{u, largo(d.Bloques[u])}
	}
	b := &d.Bloques[p.B]
	celda := b.Celda
	if celda.EnTabla() {
		nuevos = enCelda(nuevos, celda)
	}
	o := max(p.O, inicioEditable(*b))
	if !editable(*b) {
		// Sobre una regla o una tabla no se escribe: se pega detrás.
		d.Bloques = slices.Insert(d.Bloques, p.B+1, nuevos...)
		u := p.B + len(nuevos)
		return Pos{u, largo(d.Bloques[u])}
	}
	if len(nuevos) == 1 && nuevos[0].Tipo == Parrafo && nuevos[0].Celda == celda {
		ts := nuevos[0].Tramos
		if b.Tipo == Tarea {
			for i := range ts {
				ts[i].EnSpan = true // lo escrito en una tarea va en su `<span>`
			}
		}
		i := partirEn(b, o)
		b.Tramos = slices.Insert(b.Tramos, i, ts...)
		limpiar(b)
		return Pos{p.B, o + largo(nuevos[0])}
	}
	// Lo que había detrás del cursor: sus tramos, y el bloque que formarían solos, con la
	// forma del actual (como al partir con Intro; una tarea lleva su casilla delante).
	resto := *b
	resto.Tramos = slices.Clone(b.Tramos)
	quitarLetras(&resto, 0, o)
	cola := bloqueNuevo(*b, false)
	for _, t := range resto.Tramos {
		t.EnSpan = t.EnSpan || b.Tipo == Tarea
		cola.Tramos = append(cola.Tramos, t)
	}
	limpiar(&cola)
	quitarLetras(b, o, largo(*b))
	primero := 0
	if nuevos[0].Tipo == Parrafo && nuevos[0].Celda == celda {
		b.Tramos = append(b.Tramos, nuevos[0].Tramos...)
		limpiar(b)
		primero = 1
	}
	nuevos = nuevos[primero:]
	at := p.B + 1
	if primero == 0 && largo(*b) == 0 && b.Tipo == Parrafo {
		// Pegar bloques al principio de una línea vacía: la línea vacía no se queda.
		d.Bloques = slices.Delete(d.Bloques, p.B, p.B+1)
		at = p.B
	}
	d.Bloques = slices.Insert(d.Bloques, at, nuevos...)
	u := at + len(nuevos) - 1
	if len(nuevos) == 0 {
		u = p.B
	}
	fin := Pos{u, largo(d.Bloques[u])}
	switch {
	case len(resto.Tramos) == 0 && editable(d.Bloques[u]):
		// Nada detrás del cursor: el cursor, al final de lo pegado.
	case editable(d.Bloques[u]) && d.Bloques[u].Tipo != Codigo && d.Bloques[u].Celda == celda:
		// Lo que había detrás del cursor sigue a lo último pegado, en su línea.
		d.Bloques[u].Tramos = append(d.Bloques[u].Tramos, resto.Tramos...)
		limpiar(&d.Bloques[u])
	default:
		// Detrás de una tabla, una regla o un bloque de código, en su propia línea (y
		// lo pegado que acaba en una celda no se lleva lo de fuera dentro).
		d.Bloques = slices.Insert(d.Bloques, u+1, cola)
		if !editable(d.Bloques[u]) {
			fin = Pos{u + 1, 0}
		}
	}
	return fin
}

// soloHueco dice si lo escribible de una tarea es solo blanco o `&nbsp;`.
func soloHueco(b Bloque) bool {
	r := []rune(b.Texto())
	resto := string(r[min(inicioEditable(b), len(r)):])
	return strings.TrimSpace(strings.ReplaceAll(resto, "\u00a0", " ")) == ""
}

// quitarLetras borra las letras [desde, hasta) de un bloque.
func quitarLetras(b *Bloque, desde, hasta int) {
	if hasta <= desde {
		return
	}
	i := partirEn(b, desde)
	j := partirEn(b, hasta)
	b.Tramos = slices.Delete(b.Tramos, i, j)
	limpiar(b)
}

// Borrar quita lo que hay entre a y b (en cualquier orden) y devuelve dónde queda el
// cursor. Entre bloques, el primero se queda con el final del último, con SU forma.
func (d *Documento) Borrar(a, b Pos) Pos {
	a, b = ordenar(a, b)
	if a == b || len(d.Bloques) == 0 {
		return a
	}
	if a.B == b.B {
		quitarLetras(&d.Bloques[a.B], a.O, b.O)
		return a
	}
	if d.Bloques[a.B].Celda != d.Bloques[b.B].Celda {
		return d.borrarEntreCeldas(a, b)
	}
	primero := &d.Bloques[a.B]
	quitarLetras(primero, a.O, largo(*primero))
	ultimo := d.Bloques[b.B]
	quitarLetras(&ultimo, 0, b.O)
	primero.Tramos = append(primero.Tramos, ultimo.Tramos...)
	limpiar(primero)
	d.Bloques = slices.Delete(d.Bloques, a.B+1, b.B+1)
	return a
}

// borrarEntreCeldas es borrar una selección que empieza y acaba en celdas distintas (o
// una fuera de la tabla): lo que hace el navegador, que VACÍA lo seleccionado de las
// celdas y deja la rejilla. Lo que en medio no es de ninguna tabla se va entero.
func (d *Documento) borrarEntreCeldas(a, b Pos) Pos {
	quitarLetras(&d.Bloques[a.B], a.O, largo(d.Bloques[a.B]))
	quitarLetras(&d.Bloques[b.B], 0, b.O)
	quedan := slices.Clone(d.Bloques[:a.B+1])
	for k := a.B + 1; k < b.B; k++ {
		bl := d.Bloques[k]
		if bl.Celda.EnTabla() && d.Bloques[k-1].Celda != bl.Celda {
			quedan = append(quedan, Bloque{Tipo: Parrafo, Celda: bl.Celda}) // la celda, vacía
		}
	}
	d.Bloques = append(quedan, d.Bloques[b.B:]...)
	return a
}

// bloqueNuevo es el que nace al partir `b` por `o`: lo que haría Chromium. Se queda en
// la celda de `b`.
func bloqueNuevo(b Bloque, alFinal bool) Bloque {
	n := bloqueNuevoSinCelda(b, alFinal)
	n.Celda = b.Celda
	return n
}

func bloqueNuevoSinCelda(b Bloque, alFinal bool) Bloque {
	n := Bloque{Tipo: b.Tipo, Nivel: b.Nivel, Etiqueta: b.Etiqueta, Alineacion: b.Alineacion}
	switch b.Tipo {
	case Titulo:
		if alFinal {
			n = Bloque{Tipo: Parrafo, Etiqueta: "div"}
		}
	case Tarea:
		n = Bloque{Tipo: Tarea, Etiqueta: b.Etiqueta, Tramos: []Tramo{{Texto: " "}}}
	case Parrafo:
		if b.Etiqueta == "" {
			n.Etiqueta = "div" // detrás del texto suelto de la raíz, un `<div>`
		}
	case Regla, BloqueOpaco:
		n = Bloque{Tipo: Parrafo, Etiqueta: "div"}
	case Cita, Codigo, Vineta, Numerada:
	}
	return n
}

// Partir es Intro: parte el bloque por el cursor. Una tarea o un elemento de lista VACÍOS
// no parten: se convierten en párrafo, que es como se sale de una lista.
func (d *Documento) Partir(p Pos) Pos {
	if len(d.Bloques) == 0 {
		d.Bloques = []Bloque{{Tipo: Parrafo}}
		p = Pos{}
	}
	b := d.Bloques[p.B]
	vacio := strings.TrimSpace(strings.ReplaceAll(b.Texto(), "\u00a0", "")) == ""
	switch {
	case b.Tipo == Tarea && vacio:
		d.Bloques[p.B] = Bloque{Tipo: Parrafo, Etiqueta: "p", Celda: b.Celda}
		return Pos{p.B, 0}
	case esLista(b.Tipo) && vacio:
		d.Bloques[p.B] = Bloque{Tipo: Parrafo, Etiqueta: "div", Alineacion: b.Alineacion, Celda: b.Celda}
		return Pos{p.B, 0}
	}
	o := max(p.O, inicioEditable(b))
	alFinal := o >= largo(b)
	n := bloqueNuevo(b, alFinal)
	resto := b
	resto.Tramos = slices.Clone(b.Tramos)
	quitarLetras(&resto, 0, o)
	if b.Tipo == Tarea {
		// La cola va en el `<span>` de la tarea nueva.
		for _, t := range resto.Tramos {
			t.EnSpan = true
			n.Tramos = append(n.Tramos, t)
		}
	} else {
		n.Tramos = resto.Tramos
	}
	limpiar(&n)
	quitarLetras(&d.Bloques[p.B], o, largo(b))
	d.Bloques = slices.Insert(d.Bloques, p.B+1, n)
	return Pos{p.B + 1, inicioEditable(n)}
}

// Retroceso es la tecla de borrar hacia atrás con el cursor quieto.
func (d *Documento) Retroceso(p Pos) Pos {
	if len(d.Bloques) == 0 {
		return p
	}
	b := d.Bloques[p.B]
	inicio := inicioEditable(b)
	if p.O > inicio {
		return d.Borrar(Pos{p.B, p.O - 1}, p)
	}
	switch b.Tipo {
	case Tarea:
		// Al principio de una tarea, deja de serlo y su texto pasa a un párrafo.
		t := d.Bloques[p.B]
		quitarLetras(&t, 0, inicio)
		for i := range t.Tramos {
			t.Tramos[i].EnSpan = false
		}
		d.Bloques[p.B] = Bloque{Tipo: Parrafo, Etiqueta: "p", Tramos: t.Tramos, Celda: b.Celda}
		limpiar(&d.Bloques[p.B])
		return Pos{p.B, 0}
	case Vineta, Numerada:
		// Al principio de un elemento de lista, sale de la lista.
		d.Bloques[p.B].Tipo, d.Bloques[p.B].Etiqueta, d.Bloques[p.B].Nivel = Parrafo, "div", 0
		return Pos{p.B, 0}
	case Parrafo, Titulo, Cita, Codigo, Regla, BloqueOpaco:
	}
	if p.B > 0 && d.Bloques[p.B-1].Celda != b.Celda {
		// Al principio de una celda no se junta con la de antes, ni lo que sigue a una
		// tabla con su última celda. Un párrafo VACÍO detrás de una tabla sí se va, y el
		// cursor queda al final de la tabla, que es lo que hace el navegador.
		if !b.Celda.EnTabla() && largo(b) == 0 {
			d.Bloques = slices.Delete(d.Bloques, p.B, p.B+1)
			return Pos{p.B - 1, largo(d.Bloques[p.B-1])}
		}
		return p
	}
	if p.B == 0 {
		if b.Tipo != Parrafo {
			d.Bloques[0].Tipo, d.Bloques[0].Etiqueta = Parrafo, "div"
		}
		return p
	}
	previo := d.Bloques[p.B-1]
	if previo.Tipo == Regla || previo.Tipo == BloqueOpaco {
		d.Bloques = slices.Delete(d.Bloques, p.B-1, p.B)
		return Pos{p.B - 1, 0}
	}
	return d.Borrar(Pos{p.B - 1, largo(previo)}, p)
}

// Suprimir es la tecla de borrar hacia delante.
func (d *Documento) Suprimir(p Pos) Pos {
	if len(d.Bloques) == 0 {
		return p
	}
	if p.O < largo(d.Bloques[p.B]) {
		return d.Borrar(p, Pos{p.B, p.O + 1})
	}
	if p.B+1 < len(d.Bloques) {
		siguiente := d.Bloques[p.B+1]
		if siguiente.Celda != d.Bloques[p.B].Celda {
			return p // al final de una celda, o justo antes de una tabla, no se junta
		}
		return d.Borrar(p, Pos{p.B + 1, inicioEditable(siguiente)})
	}
	return p
}

// tramos recorre los tramos de la selección [a, b), partiéndolos en sus bordes, y llama a
// `f` con cada uno.
func (d *Documento) tramos(a, b Pos, f func(*Tramo)) {
	a, b = ordenar(a, b)
	for k := a.B; k <= b.B && k < len(d.Bloques); k++ {
		bl := &d.Bloques[k]
		desde, hasta := 0, largo(*bl)
		if k == a.B {
			desde = max(a.O, inicioEditable(*bl))
		}
		if k == b.B {
			hasta = b.O
		}
		if hasta <= desde {
			continue
		}
		i, j := partirEn(bl, desde), partirEn(bl, hasta)
		for x := i; x < j; x++ {
			if !bl.Tramos[x].Salto {
				f(&bl.Tramos[x])
			}
		}
	}
	for k := a.B; k <= b.B && k < len(d.Bloques); k++ {
		limpiar(&d.Bloques[k])
	}
}

// sinonimos son las etiquetas que pintan lo mismo: para saber si algo ya está en negrita
// da igual que sea `<b>` o `<strong>`.
var sinonimos = map[string][]string{
	"b": {"b", "strong"}, "i": {"i", "em"}, "u": {"u"}, "s": {"s", "strike", "del"},
	"code": {"code"}, "mark": {"mark"},
}

func tieneMarca(t Tramo, m string) bool {
	return slices.ContainsFunc(t.Marcas, func(x string) bool { return slices.Contains(sinonimos[m], x) })
}

func conMarca(t Tramo, m string, poner bool) Tramo {
	marcas := slices.DeleteFunc(slices.Clone(t.Marcas), func(x string) bool { return slices.Contains(sinonimos[m], x) })
	if poner {
		marcas = append(marcas, m)
	}
	if len(marcas) == 0 {
		marcas = nil
	}
	t.Marcas = marcas
	return t
}

// AlternarMarca pone o quita una marca (b, i, u, s, code) en la selección: si TODA la
// selección ya la tiene, se quita; si no, se pone. Es lo que hace `execCommand`.
func (d *Documento) AlternarMarca(a, b Pos, m string) {
	todas := true
	copia := d.copia()
	copia.tramos(a, b, func(t *Tramo) { todas = todas && tieneMarca(*t, m) })
	d.tramos(a, b, func(t *Tramo) { *t = conMarca(*t, m, !todas) })
}

// Pintar cambia el color del texto (`foreColor`); vacío es el de por defecto.
func (d *Documento) Pintar(a, b Pos, color string) {
	d.tramos(a, b, func(t *Tramo) {
		t.Color = color
		t.EnSpan = t.EnSpan || color != "" || t.Fondo != ""
	})
}

// Resaltar pone el fondo del resaltado (`hiliteColor`).
func (d *Documento) Resaltar(a, b Pos, fondo string) {
	d.tramos(a, b, func(t *Tramo) {
		t.Fondo = fondo
		t.EnSpan = true
	})
}

// Enlazar convierte la selección en un enlace. El servicio le pone `target` y `rel` a
// todos al guardar, así que aquí nacen con ellos (`Externo`), o la nota no sería fiel.
func (d *Documento) Enlazar(a, b Pos, href string) {
	d.tramos(a, b, func(t *Tramo) { t.Enlace, t.Externo = href, true })
}

// QuitarFormato es `removeFormat`: fuera las marcas, los colores, la fuente y el tamaño;
// el enlace se queda.
func (d *Documento) QuitarFormato(a, b Pos) {
	d.tramos(a, b, func(t *Tramo) {
		enSpanTarea := t.EnSpan && t.Color == "" && t.Fondo == "" && t.Fuente == "" && t.Tamano == ""
		t.Marcas, t.Color, t.Fondo, t.Fuente, t.Tamano = nil, "", "", "", ""
		t.EnSpan = enSpanTarea
	})
}

// PonerFuente es `fontName`: el `font-family` de la selección, en su `<span>`; vacío, el
// de por defecto.
func (d *Documento) PonerFuente(a, b Pos, fuente string) {
	d.tramos(a, b, func(t *Tramo) { t.Fuente = fuente; t.EnSpan = t.EnSpan || fuente != "" })
}

// PonerTamano es `fontSize`: el `font-size` de la selección (un valor de CSS: «18px»,
// «14pt»), en su `<span>`; vacío, el de por defecto.
func (d *Documento) PonerTamano(a, b Pos, tamano string) {
	d.tramos(a, b, func(t *Tramo) { t.Tamano = tamano; t.EnSpan = t.EnSpan || tamano != "" })
}

// InsertarImagen mete una imagen en `p` y devuelve dónde queda el cursor, detrás de ella:
// el `<img src="…" alt="" style="max-width: 100%">` que escribe la barra de Mail en la
// web. Entra en la línea, como el `insertHTML` del navegador.
func (d *Documento) InsertarImagen(p Pos, src string) Pos {
	im := Tramo{Imagen: []html.Attribute{{Key: "src", Val: src}, {Key: "alt"}, {Key: "style", Val: "max-width: 100%"}}}
	return d.InsertarDocumento(p, &Documento{Bloques: []Bloque{{Tipo: Parrafo, Tramos: []Tramo{im}}}})
}

// bloques llama a `f` con cada bloque tocado por la selección.
func (d *Documento) bloques(a, b Pos, f func(*Bloque)) {
	a, b = ordenar(a, b)
	for k := a.B; k <= b.B && k < len(d.Bloques); k++ {
		f(&d.Bloques[k])
	}
}

// Formatear es `formatBlock`: «p», «h1»…«h5», «blockquote» o «pre».
func (d *Documento) Formatear(a, b Pos, etiqueta string) {
	d.bloques(a, b, func(bl *Bloque) {
		if bl.Tipo == Regla || bl.Tipo == BloqueOpaco {
			return
		}
		if bl.Tipo == Tarea {
			quitarLetras(bl, 0, inicioEditable(*bl))
			for i := range bl.Tramos {
				bl.Tramos[i].EnSpan = false
			}
		}
		bl.Etiqueta, bl.Nivel = etiqueta, 0
		switch etiqueta {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			bl.Tipo, bl.Nivel = Titulo, int(etiqueta[1]-'0')
		case "blockquote":
			bl.Tipo = Cita
		case "pre":
			bl.Tipo = Codigo
		default:
			bl.Tipo, bl.Etiqueta = Parrafo, "p"
		}
		limpiar(bl)
	})
}

// EtiquetaDe es la etiqueta de bloque que enseña el desplegable: la del bloque del
// cursor, o «p» si no es ninguna de las ocho.
func EtiquetaDe(bl Bloque) string {
	switch bl.Tipo {
	case Titulo:
		return bl.Etiqueta
	case Cita:
		return "blockquote"
	case Codigo:
		return "pre"
	case Parrafo, Vineta, Numerada, Tarea, Regla, BloqueOpaco:
	}
	return "p"
}

// AlternarLista es `insertUnorderedList` / `insertOrderedList`: si todos los bloques ya
// son de esa lista, salen de ella; si no, entran.
func (d *Documento) AlternarLista(a, b Pos, tipo TipoBloque) {
	todos := true
	d.bloques(a, b, func(bl *Bloque) { todos = todos && bl.Tipo == tipo })
	d.bloques(a, b, func(bl *Bloque) {
		if bl.Tipo == Regla || bl.Tipo == BloqueOpaco {
			return
		}
		if bl.Tipo == Tarea {
			quitarLetras(bl, 0, inicioEditable(*bl))
			for i := range bl.Tramos {
				bl.Tramos[i].EnSpan = false
			}
		}
		switch {
		case todos:
			bl.Tipo, bl.Etiqueta, bl.Nivel = Parrafo, "div", 0
		case esLista(bl.Tipo):
			bl.Tipo = tipo // de viñetas a números conserva el nivel
		default:
			bl.Tipo, bl.Etiqueta, bl.Nivel = tipo, "li", 1
		}
		limpiar(bl)
	})
}

// Sangrar mete (+1) o saca (−1) un nivel los elementos de lista de la selección. Fuera
// de una lista no hace nada.
func (d *Documento) Sangrar(a, b Pos, paso int) {
	d.bloques(a, b, func(bl *Bloque) {
		if esLista(bl.Tipo) {
			bl.Nivel = min(max(bl.Nivel+paso, 1), 6)
		}
	})
}

// Alinear pone el `text-align` de los bloques de la selección.
func (d *Documento) Alinear(a, b Pos, alineacion string) {
	d.bloques(a, b, func(bl *Bloque) {
		if bl.Tipo != Regla && bl.Tipo != BloqueOpaco {
			bl.Alineacion = alineacion
		}
	})
}

// PonerTarea es el botón de tarea: un bloque vacío se convierte en tarea; si tiene
// texto, la tarea nace detrás.
func (d *Documento) PonerTarea(p Pos) Pos {
	tarea := Bloque{Tipo: Tarea, Etiqueta: "div", Tramos: []Tramo{{Texto: " "}}}
	if len(d.Bloques) == 0 {
		d.Bloques = []Bloque{tarea}
		return Pos{0, 1}
	}
	tarea.Celda = d.Bloques[p.B].Celda
	if b := d.Bloques[p.B]; strings.TrimSpace(b.Texto()) == "" && b.Tipo != Tarea && b.Tipo != Regla {
		d.Bloques[p.B] = tarea
		return Pos{p.B, 1}
	}
	d.Bloques = slices.Insert(d.Bloques, p.B+1, tarea)
	return Pos{p.B + 1, 1}
}

// Regla parte por el cursor y mete una línea horizontal en medio.
func (d *Documento) Regla(p Pos) Pos {
	if len(d.Bloques) == 0 {
		d.Bloques = []Bloque{{Tipo: Regla}, {Tipo: Parrafo, Etiqueta: "div"}}
		return Pos{1, 0}
	}
	q := d.Partir(p)
	d.Bloques = slices.Insert(d.Bloques, q.B, Bloque{Tipo: Regla, Celda: d.Bloques[q.B].Celda})
	return Pos{q.B + 1, q.O}
}

// MarcarTarea cambia la casilla de una tarea del cuerpo.
func (d *Documento) MarcarTarea(bloque int, marcada bool) {
	if bloque >= 0 && bloque < len(d.Bloques) && d.Bloques[bloque].Tipo == Tarea {
		d.Bloques[bloque].Marcada = marcada
	}
}

// TextoEntre es el texto plano de la selección, con los bloques separados por saltos:
// es lo que va al portapapeles.
func (d *Documento) TextoEntre(a, b Pos) string {
	a, b = ordenar(a, b)
	var out []string
	for k := a.B; k <= b.B && k < len(d.Bloques); k++ {
		r := []rune(d.Bloques[k].Texto())
		desde, hasta := 0, len(r)
		if k == a.B {
			desde = min(a.O, len(r))
		}
		if k == b.B {
			hasta = min(b.O, len(r))
		}
		out = append(out, string(r[desde:max(hasta, desde)]))
	}
	return strings.ReplaceAll(strings.Join(out, "\n"), string(Objeto), "")
}

// ParaGuardar es el HTML que se manda: el del modelo, con lo que el navegador pondría para
// que la nota se vea igual al volver. Un bloque vacío lleva su `<br>` (un `<p></p>` no
// ocupa ni una línea), y un salto al final de un bloque, otro (el último `<br>` de un
// bloque no se pinta).
func (d *Documento) ParaGuardar() string {
	c := d.copia()
	for i := range c.Bloques {
		b := &c.Bloques[i]
		if b.Tipo == Regla || b.Tipo == BloqueOpaco {
			continue
		}
		n := len(b.Tramos)
		if largo(*b)-inicioEditable(*b) == 0 || (n > 0 && b.Tramos[n-1].Salto) {
			b.Tramos = append(b.Tramos, Tramo{Salto: true})
		}
	}
	return c.HTML()
}

// ParaEditar es la copia que se edita: sin el `<br>` final que el navegador no pinta
// (y que `ParaGuardar` vuelve a poner).
func (d *Documento) ParaEditar() *Documento {
	c := d.copia()
	c.desplegado = true
	for i := range c.Bloques {
		b := &c.Bloques[i]
		if n := len(b.Tramos); n > 0 && b.Tramos[n-1].Salto {
			b.Tramos = b.Tramos[:n-1]
		}
	}
	return c
}

// marcadorTarea es la expresión de la tarea en `client/markdown.ts`.
var marcadorTarea = regexp.MustCompile(`^(\[\]|\[\s?\]|[-*]\s*\[\s?\])$`)

// atajoMarkdown es `client/markdown.ts`: lo que dispara un marcador al principio de la
// línea seguido de un espacio («- », «1. », «# »…«##### », «> », «``` », «[] ») o de
// Intro («---»). `orden` es la de la barra; `borrar`, las letras del marcador.
func atajoMarkdown(linea string, intro bool) (orden, valor string, borrar int, ok bool) {
	n := utf8.RuneCountInString(linea)
	if intro {
		if n >= 3 && (strings.Trim(linea, "-") == "" || strings.Trim(linea, "*") == "") {
			return "regla", "", n, true
		}
		return "", "", 0, false
	}
	switch {
	case linea == "-" || linea == "*" || linea == "+":
		return "vinetas", "", 1, true
	case linea == "1." || linea == "1)":
		return "numeros", "", 2, true
	case n >= 1 && n <= 5 && strings.Trim(linea, "#") == "":
		return "bloque", "h" + string(rune('0'+n)), n, true
	case linea == ">":
		return "bloque", "blockquote", 1, true
	case linea == "```":
		return "bloque", "pre", 3, true
	case marcadorTarea.MatchString(linea):
		return "tarea", "", n, true
	}
	return "", "", 0, false
}

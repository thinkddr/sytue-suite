// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"maps"
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Buscar» con una hoja: el panel del carril es el de `editor` y lo que busca son las
// celdas de la hoja ACTIVA, fila a fila y de izquierda a derecha, en el texto crudo —el
// literal o la fórmula, lo que enseña la barra de fórmulas—. La web hace lo mismo con
// `sheets/search.ts`; antes buscaba solo en la celda enfocada.
//
// Lo que no se encuentra, aquí y en la web: lo que el autofiltro esconde y lo que la
// protección no deja seleccionar, porque no se podría llevar a la vista, y las
// fórmulas OCULTAS de una hoja protegida, porque buscar en su texto la delataría.
//
// Reemplazar escribe por el mismo camino que teclear (`Rejilla.Escribir`): respeta la
// solo lectura, lo bloqueado y las validaciones estrictas, y cada reemplazo es un paso
// de deshacer. «Reemplazar todo» es UN paso, y se salta —avisando una vez— lo que no se
// puede escribir.

// hallazgo es un trozo del texto de una celda que casa con la búsqueda, en runas.
type hallazgo struct {
	c, f, desde, hasta int
}

// antes ordena como se recorre la hoja: fila, columna y posición en el texto.
func (h hallazgo) antes(o hallazgo) bool {
	if h.f != o.f {
		return h.f < o.f
	}
	if h.c != o.c {
		return h.c < o.c
	}
	return h.desde < o.desde
}

// busqueda es el [editor.Buscable] de una hoja.
type busqueda struct {
	lz        *Lienzo
	hallazgos []hallazgo
	activa    int
	// De qué libro y qué hoja salieron los hallazgos, y con qué consulta: si cualquiera
	// de las dos cosas ha cambiado desde entonces, se vuelve a buscar antes de mover la
	// selección o de escribir, o se escribiría en la celda equivocada.
	version        uint64
	hoja           int
	q              string
	mayus, palabra bool
}

var _ editor.Buscable = (*busqueda)(nil)

// panelesDe es el `Paneles` del contexto de Calcus: «Buscar» sobre la hoja; el panel se
// monta una vez, para que lo tecleado se conserve al cerrar y abrir.
func panelesDe(lz *Lienzo, t func(string, i18n.Vars) string) func(editor.Panel) fyne.CanvasObject {
	b := &busqueda{lz: lz}
	lz.busqueda = b
	var panel *editor.Buscador
	return func(p editor.Panel) fyne.CanvasObject {
		if p != editor.PanelBuscar {
			return nil
		}
		if panel == nil {
			panel = editor.NuevoBuscador(b, t)
		}
		return panel.Objeto()
	}
}

// Buscar recorre las celdas con algo de la hoja activa.
func (b *busqueda) Buscar(q string, mayus, palabra bool) int {
	l, rj := b.lz.Libro, b.lz.Rejilla
	b.hallazgos, b.activa = nil, 0
	b.version, b.hoja, b.q, b.mayus, b.palabra = l.version, l.Activa(), q, mayus, palabra
	aguja := []rune(q)
	ocultas := l.FilasOcultas()
	for _, k := range l.celdasEnOrden() {
		if k.C >= rj.Cols || k.F >= rj.Fils || (k.F < len(ocultas) && ocultas[k.F]) ||
			!l.Seleccionable(k.C, k.F) || l.FormulaOculta(k.C, k.F) {
			continue
		}
		for _, m := range editor.Casar([]rune(b.lz.Libro.Crudo(k.C, k.F)), aguja, mayus, palabra) {
			if len(b.hallazgos) == editor.MaxCoincidencias {
				b.resaltar()
				return len(b.hallazgos)
			}
			b.hallazgos = append(b.hallazgos, hallazgo{k.C, k.F, m[0], m[1]})
		}
	}
	b.resaltar()
	return len(b.hallazgos)
}

// resaltar pasa a la rejilla las celdas con coincidencias, para pintarlas.
func (b *busqueda) resaltar() {
	var rs map[Celda]bool
	if len(b.hallazgos) > 0 {
		rs = make(map[Celda]bool, len(b.hallazgos))
		for _, h := range b.hallazgos {
			rs[Celda{C: h.c, F: h.f}] = true
		}
	}
	if len(rs) == 0 && len(b.lz.Rejilla.resaltes) == 0 {
		return
	}
	b.lz.Rejilla.resaltes = rs
	b.lz.Rejilla.Refresh()
}

// alDia vuelve a buscar si el libro o la hoja han cambiado desde la última búsqueda.
// Devuelve false si lo ha hecho: lo que se iba a hacer apuntaba a otra cosa.
func (b *busqueda) alDia() bool {
	l := b.lz.Libro
	if b.version == l.version && b.hoja == l.Activa() {
		return true
	}
	b.Buscar(b.q, b.mayus, b.palabra)
	return false
}

// IrACoincidencia selecciona la celda de la i-ésima y la lleva a la vista. La celda que
// estuviera abierta se guarda antes, como al pulsar fuera de ella en la web (`blur`).
func (b *busqueda) IrACoincidencia(i int) {
	b.lz.Rejilla.guardarEdicion()
	b.alDia()
	n := len(b.hallazgos)
	if n == 0 {
		return
	}
	b.activa = (i%n + n) % n
	h := b.hallazgos[b.activa]
	b.lz.Rejilla.Seleccionar(h.c, h.f, false)
}

// Activa es la coincidencia que se está viendo.
func (b *busqueda) Activa() int { return b.activa }

// Coincidencias es cuántas hay.
func (b *busqueda) Coincidencias() int { return len(b.hallazgos) }

// Editable es la hoja que no es de solo lectura; lo bloqueado se mira celda a celda.
func (b *busqueda) Editable() bool { return !b.lz.Rejilla.SoloLectura }

// reemplazado es el texto de la celda con ese trozo cambiado.
func reemplazado(rs []rune, desde, hasta int, por string) []rune {
	out := append(slices.Clone(rs[:desde]), []rune(por)...)
	return append(out, rs[hasta:]...)
}

// Reemplazar cambia la activa y pasa a la siguiente: la primera que empieza detrás de lo
// recién escrito (lo nuevo no se vuelve a encontrar aunque contenga lo buscado). Si la
// celda no deja escribir, lo dice y pasa igual a la siguiente, para no quedarse parado.
func (b *busqueda) Reemplazar(por, q string, mayus, palabra bool) {
	rj := b.lz.Rejilla
	rj.guardarEdicion()
	if !b.Editable() || !b.alDia() || len(b.hallazgos) == 0 {
		return
	}
	h := b.hallazgos[b.activa]
	rj.Seleccionar(h.c, h.f, false)
	sigue := hallazgo{h.c, h.f, h.hasta, 0}
	if rj.Escribir(string(reemplazado([]rune(b.lz.Libro.Crudo(h.c, h.f)), h.desde, h.hasta, por))) {
		sigue.desde = h.desde + len([]rune(por))
	}
	b.Buscar(q, mayus, palabra)
	for i, o := range b.hallazgos {
		if !o.antes(sigue) {
			b.IrACoincidencia(i)
			return
		}
	}
	b.IrACoincidencia(0)
}

// ReemplazarTodo las cambia todas en UN paso de deshacer. Lo bloqueado y lo que una
// validación estricta no admite se queda como estaba, y se avisa una vez de lo primero
// que se saltó.
func (b *busqueda) ReemplazarTodo(por, q string, mayus, palabra bool) {
	lz, rj := b.lz, b.lz.Rejilla
	rj.guardarEdicion()
	if !b.Editable() || !b.alDia() || len(b.hallazgos) == 0 {
		return
	}
	l := lz.Libro
	nuevos := map[Celda][]rune{}
	var orden []Celda
	for i := len(b.hallazgos) - 1; i >= 0; i-- { // de atrás adelante: las posiciones de delante siguen valiendo
		h := b.hallazgos[i]
		k := Celda{C: h.c, F: h.f}
		rs, ok := nuevos[k]
		if !ok {
			rs = []rune(b.lz.Libro.Crudo(h.c, h.f))
			orden = append(orden, k)
		}
		nuevos[k] = reemplazado(rs, h.desde, h.hasta, por)
	}
	slices.Reverse(orden)
	textos := map[Celda]string{}
	avisado := false
	for _, k := range orden {
		texto := string(nuevos[k])
		switch {
		case conTexto(l.HTML(k.C, k.F), texto) == l.HTML(k.C, k.F): // nada que escribir
		case !l.Editable(k.C, k.F):
			if !avisado && rj.AlProtegida != nil {
				rj.AlProtegida(k)
			}
			avisado = true
		default:
			if msg, no := l.rechaza(k.C, k.F, texto); no {
				if !avisado && rj.AlRechazar != nil {
					rj.AlRechazar(msg)
				}
				avisado = true
				continue
			}
			textos[k] = texto
		}
	}
	if l.EscribirVarias(textos) {
		if rj.AlEditar != nil {
			rj.AlEditar()
		}
		rj.Refresh()
	}
	b.Buscar(q, mayus, palabra)
}

// celdasEnOrden son las celdas con algo de la hoja activa, fila a fila.
func (l *Libro) celdasEnOrden() []Celda {
	ks := slices.Collect(maps.Keys(l.h().celdas))
	slices.SortFunc(ks, func(a, b Celda) int {
		if a.F != b.F {
			return a.F - b.F
		}
		return a.C - b.C
	})
	return ks
}

// EscribirVarias pone el texto crudo de varias celdas de la hoja activa, cada una con su
// formato, en UN paso de deshacer. Devuelve si cambió algo.
func (l *Libro) EscribirVarias(textos map[Celda]string) bool {
	htmls := map[Celda]string{}
	for k, t := range textos {
		if html := conTexto(l.HTML(k.C, k.F), t); html != l.HTML(k.C, k.F) {
			htmls[k] = html
		}
	}
	if len(htmls) == 0 {
		return false
	}
	l.anotar()
	for k, html := range htmls {
		l.poner(l.f.activa, k.C, k.F, html)
	}
	return true
}

var _ editor.ResaltesDeBusqueda = (*Lienzo)(nil)

// OcultarResaltes quita el amarillo de las celdas al cerrar el panel «Buscar».
func (lz *Lienzo) OcultarResaltes() {
	if lz.Rejilla.resaltes != nil {
		lz.Rejilla.resaltes = nil
		lz.Rejilla.Refresh()
	}
}

// MostrarResaltes lo vuelve a pintar al abrirlo, con la búsqueda al día.
func (lz *Lienzo) MostrarResaltes() {
	if b := lz.busqueda; b != nil && b.q != "" && b.alDia() {
		b.resaltar() // si no estaba al día, alDia ya ha buscado y pintado
	}
}

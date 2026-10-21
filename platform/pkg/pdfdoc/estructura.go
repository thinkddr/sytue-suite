// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Elem es un elemento del árbol de estructura lógica (ISO 32000-1, 14.7): lo que un lector
// de pantalla recorre en vez de adivinar el orden de lectura desde la posición del texto.
// Los tipos son los estándar (Document, H1-H6, P, L, LI, Lbl, LBody, Table, TR, TH, TD,
// Figure, Link, Span, BlockQuote): ninguno necesita /RoleMap.
type Elem struct {
	Tipo  string
	padre *Elem

	alt, lang        string
	scope            string // TH: «Row», «Column» o «Both»
	colSpan, rowSpan int
	hijos            []hijo
	obj              int
	reg              bool // ya figura entre los hijos de su padre
}

// hijo es una entrada de /K: otro elemento, un tramo de contenido marcado (MCID de una
// página) o una anotación (OBJR).
type hijo struct {
	e    *Elem
	pg   *pagina
	mcid int
	an   *anotacion
}

// anotacion es una anotación de enlace, ligada al elemento Link que la contiene.
type anotacion struct {
	pg       *pagina
	rect     [4]float64
	uri      string
	contents string
	obj, key int
}

// enlace es un destino saliente ya validado, con el texto que se pinta sobre él.
type enlace struct {
	uri   string
	texto string
}

// nuevo crea un elemento hijo de padre. Con eager se registra ya en el padre (el orden de
// creación es el orden de lectura); si no, al dibujar su primer contenido: Link y Span se
// crean al maquetar el párrafo, antes que el texto que los precede.
func (d *Doc) nuevo(tipo string, padre *Elem, eager bool) *Elem {
	e := &Elem{Tipo: tipo, padre: padre}
	if eager && padre != nil {
		padre.hijos, e.reg = append(padre.hijos, hijo{e: e}), true
	}
	return e
}

// registrar engancha al padre un elemento perezoso, la primera vez que se dibuja.
func (e *Elem) registrar() {
	if e.reg || e.padre == nil {
		return
	}
	e.padre.registrar()
	e.padre.hijos, e.reg = append(e.padre.hijos, hijo{e: e}), true
}

// Abrir abre un elemento contenedor bajo el que irán los siguientes bloques; Cerrar lo
// cierra. Es la forma de que quien maqueta (una tabla, una celda) escriba la estructura.
func (d *Doc) Abrir(tipo string) *Elem {
	e := d.nuevo(tipo, d.contenedor(), true)
	d.pila = append(d.pila, e)
	return e
}

func (d *Doc) Cerrar() {
	if len(d.pila) > 1 {
		d.pila = d.pila[:len(d.pila)-1]
	}
}

func (d *Doc) contenedor() *Elem { return d.pila[len(d.pila)-1] }

// abrirMC abre (si no lo está ya) una secuencia de contenido marcado de e en la página
// actual y registra su MCID como hijo de e y en el ParentTree de la página.
func (d *Doc) abrirMC(e *Elem) {
	p := d.cur
	if p.abierto == e {
		return
	}
	d.cerrarMC()
	e.registrar()
	id := len(p.mcids)
	p.mcids = append(p.mcids, e)
	e.hijos = append(e.hijos, hijo{pg: p, mcid: id})
	fmt.Fprintf(&p.contenido, "/%s <</MCID %d>> BDC\n", e.Tipo, id)
	p.abierto = e
}

func (d *Doc) cerrarMC() {
	if p := d.cur; p.abierto != nil {
		p.contenido.WriteString("EMC\n")
		p.abierto = nil
	}
}

// artefacto escribe contenido que NO es del documento (reglas, subrayados, bordes) fuera de
// la estructura, como pide PDF/UA: o es contenido real o está marcado como artefacto.
func (d *Doc) artefacto(escribir func(b *strings.Builder)) {
	d.cerrarMC()
	var b strings.Builder
	escribir(&b)
	if b.Len() > 0 {
		d.cur.contenido.WriteString("/Artifact BMC\n" + b.String() + "EMC\n")
	}
}

var reIdioma = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// urlSegura admite solo destinos que un documento puede enlazar: nada de javascript:,
// file: ni data:. Devuelve la URL ya escapada (ASCII), apta para una cadena PDF.
func urlSegura(h string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(h))
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto", "tel":
	default:
		return "", false
	}
	return u.String(), true
}

// tipoCabecera normaliza el nivel de un encabezado a lo que PDF/UA admite: el primero es
// H1 y no se salta ningún nivel (7.4.2). El tamaño visual lo decide quien llama; aquí solo
// la etiqueta.
func (d *Doc) tipoCabecera(pedido string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(pedido, "H"))
	if err != nil || n < 1 || n > 6 {
		n = 1
	}
	n = min(n, d.hUltimo+1)
	d.hUltimo = n
	return "H" + strconv.Itoa(n)
}

// emisor escribe el árbol de estructura como objetos del fichero.
type emisor struct {
	s      *salida
	pag    map[*pagina]int
	duenos map[*anotacion]*Elem // elemento Link de cada anotación, para el ParentTree
}

func (em *emisor) asignar(e *Elem) {
	e.obj = em.s.reservar()
	for _, h := range e.hijos {
		if h.e != nil {
			em.asignar(h.e)
		}
	}
}

func (em *emisor) emitir(e *Elem, padre int) {
	var pg *pagina
	for _, h := range e.hijos {
		if h.pg != nil {
			pg = h.pg
			break
		}
	}
	ks := make([]string, 0, len(e.hijos))
	for _, h := range e.hijos {
		switch {
		case h.e != nil:
			em.emitir(h.e, e.obj)
			ks = append(ks, strconv.Itoa(h.e.obj)+" 0 R")
		case h.pg != nil && h.pg == pg:
			ks = append(ks, strconv.Itoa(h.mcid))
		case h.pg != nil:
			ks = append(ks, "<< /Type /MCR /Pg "+strconv.Itoa(em.pag[h.pg])+" 0 R /MCID "+strconv.Itoa(h.mcid)+" >>")
		case h.an != nil:
			em.duenos[h.an] = e
			ks = append(ks, "<< /Type /OBJR /Obj "+strconv.Itoa(h.an.obj)+" 0 R /Pg "+strconv.Itoa(em.pag[h.an.pg])+" 0 R >>")
		}
	}
	var b strings.Builder
	b.WriteString("<< /Type /StructElem /S /" + e.Tipo + " /P " + strconv.Itoa(padre) + " 0 R")
	if pg != nil {
		b.WriteString(" /Pg " + strconv.Itoa(em.pag[pg]) + " 0 R")
	}
	if len(ks) == 1 {
		b.WriteString(" /K " + ks[0])
	} else if len(ks) > 1 {
		b.WriteString(" /K [" + strings.Join(ks, " ") + "]")
	}
	if e.alt != "" {
		b.WriteString(" /Alt " + cadena(e.alt))
	}
	if e.lang != "" {
		b.WriteString(" /Lang " + cadena(e.lang))
	}
	if e.scope != "" || e.colSpan > 1 || e.rowSpan > 1 {
		b.WriteString(" /A << /O /Table")
		if e.scope != "" {
			b.WriteString(" /Scope /" + e.scope)
		}
		if e.colSpan > 1 {
			b.WriteString(" /ColSpan " + strconv.Itoa(e.colSpan))
		}
		if e.rowSpan > 1 {
			b.WriteString(" /RowSpan " + strconv.Itoa(e.rowSpan))
		}
		b.WriteString(" >>")
	}
	b.WriteString(" >>")
	em.s.objs[e.obj-1] = []byte(b.String())
}

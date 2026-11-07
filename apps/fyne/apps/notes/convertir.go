// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
)

// Paridad web: convertir una línea de texto en tarea y una tarea en texto —`convertirNota`
// y `primeraLinea` de client/diario.ts, y el `convertir` de NoteCard.tsx—. Se pulsa la
// raya de una línea de texto (tarjeta.go) o se elige en el «…» (acciones.go), y va por
// `editar`: se pinta antes de que conteste el servicio, se deshace, y si no llega se
// vuelve a como estaba.

// MaxTituloTarea es el título de una tarea, en runas: el tope de la captura y el
// `maxNoteTitle` del servicio (`MAX_TITULO_TAREA` de la web).
const MaxTituloTarea = 400

// Los bloques que acaban una línea del cuerpo de una nota, y con qué más acaba.
var (
	bloquesDeLinea = conjunto("p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "td", "th")
	finesDeLinea   = conjunto("br", "hr", "ul", "ol", "table", "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "td", "th")
	conMultimedia  = regexp.MustCompile(`(?i)<(img|hr|table)\b`)
)

func conjunto(s ...string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, x := range s {
		m[x] = true
	}
	return m
}

// primeraLinea es la primera línea de un cuerpo HTML como texto, y el cuerpo sin ella con
// su formato. Una línea acaba en un salto (`<br>`) o donde acaba su bloque; lo que queda
// vacío al quitarla (el párrafo, la lista) se va con ella. Es el algoritmo de la web
// sobre el DOM —un rango desde el primer texto hasta lo que acaba la línea—, paso a paso.
func primeraLinea(fuente string) (linea, resto string) {
	nodos, err := fragmento(fuente)
	if err != nil {
		return "", fuente
	}
	raiz := &html.Node{Type: html.DocumentNode}
	for _, n := range nodos {
		raiz.AppendChild(n)
	}
	primero := primerTexto(raiz)
	if primero == nil {
		return "", fuente
	}
	// El bloque de la línea (el `closest` de la web), o todo el cuerpo si va suelta.
	contenedor := raiz
	for a := primero.Parent; a != nil && a != raiz; a = a.Parent {
		if a.Type == html.ElementNode && bloquesDeLinea[a.Data] {
			contenedor = a
			break
		}
	}
	fin := finDeLinea(contenedor, primero)
	// Desde su primer texto con lo que lo envuelve (la negrita de la palabra), no desde el
	// principio del bloque: una imagen que vaya delante no es parte de la línea.
	inicio := primero
	for inicio.Parent != nil && inicio.Parent != contenedor && !contiene(inicio.Parent, "img") {
		inicio = inicio.Parent
	}
	linea = quitarHasta(inicio, fin, contenedor)
	if fin != nil && fin.Data == "br" {
		fin.Parent.RemoveChild(fin)
	}
	for v := contenedor; v != raiz && v.Parent != nil && soloBlanco(v) && !contiene(v, "img", "hr", "table"); {
		padre := v.Parent
		padre.RemoveChild(v)
		v = padre
	}
	var b strings.Builder
	for n := raiz.FirstChild; n != nil; n = n.NextSibling {
		if html.Render(&b, n) != nil {
			return "", fuente
		}
	}
	return linea, strings.TrimSpace(b.String())
}

// primerTexto es el primer texto con algo que leer.
func primerTexto(raiz *html.Node) *html.Node {
	var primero *html.Node
	enOrden(raiz, func(n *html.Node) bool {
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
			primero = n
		}
		return primero == nil
	})
	return primero
}

// finDeLinea es el primer salto o bloque que viene DETRÁS del primer texto: en preorden,
// lo que se visita después de un texto ya no lo contiene. nil, la línea llega al final
// de su bloque.
func finDeLinea(contenedor, primero *html.Node) *html.Node {
	var fin *html.Node
	visto := false
	enOrden(contenedor, func(n *html.Node) bool {
		switch {
		case n == primero:
			visto = true
		case visto && n.Type == html.ElementNode && finesDeLinea[n.Data]:
			fin = n
		}
		return fin == nil
	})
	return fin
}

// quitarHasta es el `deleteContents` del rango [antes de inicio, antes de fin) —o hasta el
// final de `contenedor`—: lo que está entero dentro se quita; lo que contiene a `fin` se
// queda, sin lo suyo de antes. Devuelve su texto, con los blancos juntos (`toString`).
func quitarHasta(inicio, fin, contenedor *html.Node) string {
	var texto strings.Builder
	for n := inicio; n != nil && n != fin; {
		if fin != nil && esAntepasado(n, fin) {
			n = n.FirstChild
			continue
		}
		sig := siguienteEn(n, contenedor)
		escribirTexto(&texto, n)
		n.Parent.RemoveChild(n)
		n = sig
	}
	return strings.Join(strings.Fields(texto.String()), " ")
}

// enOrden recorre en preorden lo que cuelga de `n` (sin `n`) mientras `seguir` diga que sí.
func enOrden(n *html.Node, seguir func(*html.Node) bool) bool {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if !seguir(h) || !enOrden(h, seguir) {
			return false
		}
	}
	return true
}

// siguienteEn es el nodo que va detrás de `n` y de todo lo suyo sin salir de `dentro`.
func siguienteEn(n, dentro *html.Node) *html.Node {
	for ; n != nil && n != dentro; n = n.Parent {
		if n.NextSibling != nil {
			return n.NextSibling
		}
	}
	return nil
}

func esAntepasado(a, n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

// contiene dice si dentro de `n` (sin contarlo) hay alguna de esas etiquetas.
func contiene(n *html.Node, etiquetas ...string) bool {
	hay := false
	enOrden(n, func(h *html.Node) bool {
		hay = h.Type == html.ElementNode && slices.Contains(etiquetas, h.Data)
		return !hay
	})
	return hay
}

func escribirTexto(b *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
	}
	enOrden(n, func(h *html.Node) bool {
		if h.Type == html.TextNode {
			b.WriteString(h.Data)
		}
		return true
	})
}

func soloBlanco(n *html.Node) bool {
	var b strings.Builder
	escribirTexto(&b, n)
	return strings.TrimSpace(b.String()) == ""
}

// sinTextoHTML es un cuerpo sin nada que leer: vacío, o párrafos sin texto.
func sinTextoHTML(s string) bool {
	return htmltext.Text(s) == "" && !conMultimedia.MatchString(s)
}

// conversion es lo que cambia al convertir una línea: su tipo y, si hace falta, de dónde
// sale su texto. `desmarcar` es de tarea a texto: hecha o cancelada, deja de estarlo.
type conversion struct {
	tipo, titulo, contenido string
	desmarcar               bool
}

// convertirNota hace de una cosa apuntada una tarea, o al revés, sin perder nada de lo
// escrito (`convertirNota` de client/diario.ts):
//
//   - De texto a tarea: sin título, la primera línea del texto lo es y el resto se queda
//     debajo, con su formato. Si esa línea no cabe en el título de una tarea, el título
//     lleva su principio y el texto se queda entero.
//   - De tarea a texto: sin nada debajo, el título pasa a ser el texto; con algo, se
//     quedan los dos. Hecha o cancelada, deja de estarlo: un texto no tiene casilla, y al
//     volver a ser tarea sería una cosa por hacer que se vería tachada.
func convertirNota(tipo, titulo, contenido string) conversion {
	titulo = strings.TrimSpace(titulo)
	if tipo == "todo" {
		if titulo != "" && sinTextoHTML(contenido) {
			return conversion{tipo: "text", contenido: "<p>" + htmltext.Escape(titulo) + "</p>", desmarcar: true}
		}
		return conversion{tipo: "text", titulo: titulo, contenido: contenido, desmarcar: true}
	}
	if titulo != "" {
		return conversion{tipo: "todo", titulo: titulo, contenido: contenido}
	}
	linea, resto := primeraLinea(contenido)
	// Por runas y no por grafemas: es como cuenta el servicio (`RuneCountInString`).
	if r := []rune(linea); len(r) > MaxTituloTarea {
		return conversion{tipo: "todo", titulo: string(r[:MaxTituloTarea-1]) + "…", contenido: contenido}
	}
	return conversion{tipo: "todo", titulo: linea, contenido: resto}
}

// puedeConvertir es `puedeConvertir` de NoteCard.tsx: lo que se puede editar entero —ni
// sin red, ni de solo lectura, ni cerrado, ni un documento que no vuelve entero— y que no
// es un separador. Lo que me han compartido no pasa por aquí: su lienzo no tiene «…» ni
// raya pulsable (compartidas.go).
func (p *Pantalla) puedeConvertir(n Nota) bool {
	return p.puedeEscribir() && n.Editable(false, p.bloqueado) && !soloRaya.MatchString(n.Contenido)
}

// convertir hace tarea una línea de texto, o texto una tarea (`convertir` de
// NoteCard.tsx). Si se está escribiendo en ella, lo escrito es lo que se convierte: el
// editor se cierra sin volver a mandarlo, que ya va en la conversión.
func (p *Pantalla) convertir(id string) {
	n, ok := p.nota(id)
	if !ok || !p.puedeConvertir(n) {
		return
	}
	contenido := n.Contenido
	if s := p.edicion; s != nil && s.nota == id && !s.compartida {
		contenido = s.editor.HTML()
		s.html, s.guardado = contenido, contenido
		p.cerrarEditor()
	}
	c := convertirNota(n.Tipo, n.Titulo, contenido)
	pa := Parche{Tipo: &c.tipo, Titulo: &c.titulo, Contenido: &c.contenido}
	if c.desmarcar {
		pa.Marcada, pa.Cancelada = ptr(false), ptr(false)
	}
	// El texto cambia, y con él los enlaces a otras notas que hay en él (`sincronizarEnlaces`
	// de editarNota). En un cuaderno cifrado se quedan los que había.
	if !p.activo.Cifrado {
		pa.Enlaces = enlacesSincronizados(n.Crudo, Leer(c.contenido), p.AppURL)
	}
	lanzar(func() error { return p.editar(id, pa, true) })
}

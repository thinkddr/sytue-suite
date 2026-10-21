// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Las LEYENDAS y las REFERENCIAS CRUZADAS de «Referencias» (`insertarLeyenda`,
// `actualizarLeyendas` e `insertarReferencia` de Referencias.tsx), con el mismo HTML: la
// leyenda es un párrafo `p[data-caption]` con su etiqueta numerada en
// `span[data-caption-label]`, y la referencia, un enlace `a[data-xref]` a ella con el
// texto de su etiqueta.

// estiloLeyenda es el del párrafo de la leyenda: más pequeña, en gris, centrada.
const estiloLeyenda = "font-size:0.9em;color:#475569;text-align:center;margin:.25em 0 .75em"

// Tabla y Figura son las dos clases de leyenda (`CaptionKind`).
const (
	LeyendaTabla  = "tabla"
	LeyendaFigura = "figura"
)

func esLeyenda(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.P && n.Namespace == "" && tieneAtributo(n, "data-caption")
}

func esEtiquetaLeyenda(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Span && n.Namespace == "" && tieneAtributo(n, "data-caption-label")
}

func claseLeyenda(n *xhtml.Node) string {
	if atributo(n, "data-caption") == LeyendaFigura {
		return LeyendaFigura
	}
	return LeyendaTabla
}

// leyendas son los párrafos de leyenda, en orden de documento.
func (d *Documento) leyendas() []*xhtml.Node {
	var out []*xhtml.Node
	var recorrer func(n *xhtml.Node, prof int)
	recorrer = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if esLeyenda(h) {
				out = append(out, h)
				continue
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	return out
}

// InsertarLeyenda es «Leyenda de tabla» o «de figura»: el párrafo con la etiqueta
// «<etiqueta> N» —N, las de su clase más una— y, si lo hay, «. texto»; va debajo de la
// tabla del cursor (si es de tabla) o del bloque de arriba del todo del cursor, y si no, al
// final. Devuelve su lugar.
func (d *Documento) InsertarLeyenda(bloques []Bloque, i int, clase, etiqueta, texto string) (Lugar, bool) {
	num := 1
	n := 0
	for _, p := range d.leyendas() {
		n++
		if claseLeyenda(p) == clase {
			num++
		}
	}
	usados := d.idsUsados()
	for n++; usados["cap-"+strconv.Itoa(n)]; n++ {
	}
	id := "cap-" + strconv.Itoa(n)
	h := `<p data-caption="` + escaparWeb.Replace(clase) + `" id="` + id + `" style="` + estiloLeyenda + `">` +
		`<span data-caption-label style="font-weight:600">` + escaparWeb.Replace(etiqueta+" "+strconv.Itoa(num)) + `</span>`
	if texto = strings.TrimSpace(texto); texto != "" {
		h += ". " + escaparWeb.Replace(texto)
	}
	nodos, err := xhtml.ParseFragment(strings.NewReader(h+`</p>`), cuerpo())
	if err != nil || len(nodos) != 1 {
		return Lugar{}, false
	}
	p := nodos[0]
	var destino *xhtml.Node
	if i >= 0 && i < len(bloques) {
		switch b := bloques[i]; {
		case clase == LeyendaTabla && b.Celda != nil:
			destino = b.Celda.Tabla.nodo
		case b.cont != nil && b.cont.DataAtom != atom.Body:
			destino = superiorDe(b.cont)
		case b.ultimo != nil:
			destino = b.ultimo
		}
	}
	if destino != nil && destino.Parent != nil {
		destino.Parent.InsertBefore(p, destino.NextSibling)
	} else {
		d.raiz.AppendChild(p)
	}
	return Lugar{cont: p, primero: p.FirstChild, ultimo: p.LastChild}, true
}

// ActualizarLeyendas es «Actualizar leyendas y referencias»: renumera las de cada clase
// en orden de documento y pone a cada referencia cruzada el texto de su leyenda. Dice si
// había algo que actualizar.
func (d *Documento) ActualizarLeyendas(etiquetaTabla, etiquetaFigura string) bool {
	cuenta := map[string]int{}
	etiquetas := map[string]string{}
	hay := false
	for _, p := range d.leyendas() {
		clase := claseLeyenda(p)
		cuenta[clase]++
		nombre := etiquetaTabla
		if clase == LeyendaFigura {
			nombre = etiquetaFigura
		}
		if span := etiquetaDe(p); span != nil {
			ponerTexto(span, nombre+" "+strconv.Itoa(cuenta[clase]))
			etiquetas[atributo(p, "id")] = textoDe(span)
			hay = true
		}
	}
	for _, a := range elementosDe(d.raiz, atom.A) {
		if id := atributo(a, "data-xref"); tieneAtributo(a, "data-xref") {
			if texto, ok := etiquetas[id]; ok && texto != "" {
				ponerTexto(a, texto)
				hay = true
			}
		}
	}
	return hay
}

func etiquetaDe(p *xhtml.Node) *xhtml.Node {
	for _, s := range elementosDe(p, atom.Span) {
		if esEtiquetaLeyenda(s) {
			return s
		}
	}
	return nil
}

// ponerTexto es el `textContent = …` de la web: n se queda con solo ese texto.
func ponerTexto(n *xhtml.Node, texto string) {
	vaciar(n)
	n.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: texto})
}

// Leyenda es una fila de «Referencia cruzada» (`LeyendaRef`): su id, su etiqueta y su
// texto sin la etiqueta, recortado a 40.
type Leyenda struct{ ID, Etiqueta, Texto string }

// Leyendas son las del documento, en orden (`leerLeyendas`).
func (d *Documento) Leyendas() []Leyenda {
	ps := d.leyendas()
	out := make([]Leyenda, 0, len(ps))
	for _, p := range ps {
		etiqueta := ""
		if span := etiquetaDe(p); span != nil {
			etiqueta = textoDe(span)
		}
		texto := textoDe(p)
		if len(etiqueta) <= len(texto) {
			texto = texto[len(etiqueta):]
		}
		texto = strings.TrimSpace(strings.TrimLeft(strings.TrimLeft(texto, "."), " \t\n\r\f"))
		if r := []rune(texto); len(r) > 40 {
			texto = string(r[:39]) + "…"
		}
		out = append(out, Leyenda{ID: atributo(p, "id"), Etiqueta: etiqueta, Texto: texto})
	}
	return out
}

// InsertarReferencia es `insertarReferencia`: el enlace `a[data-xref]` a la leyenda, con su
// etiqueta, en pos (y sustituyendo [pos, hasta) si hay selección).
func (e *Edicion) InsertarReferencia(pos, hasta int, l Leyenda) {
	a := elemento("a", xhtml.Attribute{Key: "data-xref", Val: l.ID}, xhtml.Attribute{Key: "href", Val: "#" + l.ID})
	pos, hasta = max(0, min(pos, len(e.runas))), max(0, min(hasta, len(e.runas)))
	e.Sustituir(pos, max(pos, hasta), "")
	runas := []rune(l.Etiqueta)
	cadena := append(fueraDeMarcador(e.hereda(pos)), a)
	cadenas := make([][]*xhtml.Node, 0, len(e.cadenas)+len(runas))
	cadenas = append(cadenas, e.cadenas[:pos]...)
	for range runas {
		cadenas = append(cadenas, cadena)
	}
	e.cadenas = append(cadenas, e.cadenas[pos:]...)
	e.runas = append(append(append([]rune{}, e.runas[:pos]...), runas...), e.runas[pos:]...)
	e.hayPendiente = false
	e.reescribir()
}

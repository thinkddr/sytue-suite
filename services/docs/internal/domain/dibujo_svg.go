// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// El SVG VECTORIAL de un bloque «Dibujo» de Docus, para llevarlo a DOCX y ODT junto a su PNG de
// respaldo y para recuperarlo al importar.
//
// El <svg> de un <figure data-dibujo> viene del HTML del documento (o de un fichero de fuera): es
// entrada no confiable. No se copia: se REESCRIBE con una lista cerrada de elementos y atributos
// —los que genera `@sytue/escena`—, sin scripts, estilos, enlaces, imágenes ni manejadores. Lo
// que no está en la lista no sale.
//
// La escena (el JSON de `data-escena`) viaja dentro del SVG en <metadata id="sytue-escena">: así
// un DOCX u ODT exportado por Sytue vuelve a abrirse como dibujo EDITABLE y no como una imagen.
// Quien abra el fichero en Word o LibreOffice ve el dibujo vectorial y ignora el metadato.

const (
	maxSVGNodos  = 20000
	maxSVGFondo  = 64
	maxSVGValor  = 200000 // un `d` o `points` de un trazo largo
	maxEscenaSVG = 1 << 20
	idEscenaSVG  = "sytue-escena"
)

// Elementos SVG admitidos, con su nombre canónico (el parser HTML los deja como vienen o en
// minúsculas según el caso).
var svgElementos = map[string]string{
	"svg": "svg", "g": "g", "title": "title", "desc": "desc", "rect": "rect", "ellipse": "ellipse",
	"circle": "circle", "line": "line", "polygon": "polygon", "polyline": "polyline", "path": "path",
	"text": "text", "tspan": "tspan",
}

// Atributos admitidos (clave en minúsculas → nombre canónico).
var svgAtributos = map[string]string{
	"viewbox": "viewBox", "x": "x", "y": "y", "width": "width", "height": "height", "rx": "rx", "ry": "ry",
	"cx": "cx", "cy": "cy", "r": "r", "x1": "x1", "y1": "y1", "x2": "x2", "y2": "y2", "d": "d", "points": "points",
	"fill": "fill", "fill-opacity": "fill-opacity", "stroke": "stroke", "stroke-width": "stroke-width",
	"stroke-opacity": "stroke-opacity", "stroke-dasharray": "stroke-dasharray", "stroke-linecap": "stroke-linecap",
	"stroke-linejoin": "stroke-linejoin", "opacity": "opacity", "transform": "transform",
	"font-family": "font-family", "font-size": "font-size", "font-weight": "font-weight", "text-anchor": "text-anchor",
	"role": "role", "aria-label": "aria-label",
}

// svgValorSeguro descarta los valores que podrían cargar o ejecutar algo.
func svgValorSeguro(v string) bool {
	if len(v) > maxSVGValor {
		return false
	}
	l := strings.ToLower(v)
	return !strings.Contains(l, "url(") && !strings.Contains(l, "javascript:") && !strings.Contains(l, "data:") &&
		!strings.ContainsAny(v, "<>")
}

type svgEscritor struct {
	b     strings.Builder
	nodos int
	// fijo: el SVG va a un fichero suelto (Word, LibreOffice), sin CSS que dé valor a
	// `currentColor`: se fija en negro. En el editor se conserva, para seguir al tema.
	fijo bool
}

func (w *svgEscritor) elemento(n *html.Node, fondo int) {
	nombre, ok := svgElementos[strings.ToLower(n.Data)]
	if !ok || nombre == "svg" || fondo > maxSVGFondo || w.nodos >= maxSVGNodos {
		return
	}
	w.nodos++
	w.b.WriteString("<" + nombre)
	for _, a := range n.Attr {
		canon, ok := svgAtributos[strings.ToLower(a.Key)]
		if !ok || a.Namespace != "" || !svgValorSeguro(a.Val) {
			continue
		}
		v := a.Val
		if w.fijo && v == "currentColor" {
			v = "#000000"
		}
		w.b.WriteString(" " + canon + `="` + xmlEscape(v) + `"`)
	}
	w.b.WriteString(">")
	w.hijos(n, fondo)
	w.b.WriteString("</" + nombre + ">")
}

func (w *svgEscritor) hijos(n *html.Node, fondo int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			w.b.WriteString(xmlEscape(c.Data))
		case html.ElementNode:
			w.elemento(c, fondo+1)
		default:
		}
	}
}

// svgCaja lee el viewBox «0 0 ancho alto» de un <svg>.
func svgCaja(n *html.Node) (ancho, alto float64, ok bool) {
	for _, a := range n.Attr {
		if !strings.EqualFold(a.Key, "viewBox") {
			continue
		}
		p := strings.Fields(a.Val)
		if len(p) != 4 {
			return 0, 0, false
		}
		ancho, e1 := strconv.ParseFloat(p[2], 64)
		alto, e2 := strconv.ParseFloat(p[3], 64)
		if e1 != nil || e2 != nil || ancho <= 0 || alto <= 0 || ancho > 100000 || alto > 100000 {
			return 0, 0, false
		}
		return ancho, alto, true
	}
	return 0, 0, false
}

func svgNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// svgDeDibujo reescribe el <svg> de un dibujo como fichero SVG autónomo (tamaño fijo, tinta negra)
// con la escena en su <metadata>. nil si el nodo no es un SVG con viewBox válido.
func svgDeDibujo(svg *html.Node, escena string) []byte {
	ancho, alto, ok := svgCaja(svg)
	if !ok {
		return nil
	}
	w := &svgEscritor{fijo: true}
	w.b.WriteString(xmlHeader)
	w.b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + svgNum(ancho) + ` ` + svgNum(alto) +
		`" width="` + svgNum(ancho) + `" height="` + svgNum(alto) + `">`)
	if escena != "" && len(escena) <= maxEscenaSVG && json.Valid([]byte(escena)) {
		w.b.WriteString(`<metadata id="` + idEscenaSVG + `">` + xmlEscape(escena) + `</metadata>`)
	}
	w.hijos(svg, 0)
	w.b.WriteString(`</svg>`)
	return []byte(w.b.String())
}

// buscarSVG devuelve el primer <svg> de una lista de nodos (en profundidad).
func buscarSVG(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, "svg") {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if s := buscarSVG(c); s != nil {
			return s
		}
	}
	return nil
}

// dibujoDesdeSVG reconstruye el bloque «Dibujo» del editor a partir del SVG de un fichero
// importado, si lleva la escena de Sytue en su <metadata>: `<figure data-dibujo>` con la escena,
// el SVG saneado y la imagen PNG de respaldo. "" si el SVG no es un dibujo de Sytue (el llamante
// se queda entonces con la imagen rasterizada, como con cualquier SVG de terceros).
func dibujoDesdeSVG(raw []byte, respaldo string, w, h int, alt string) string {
	if len(raw) == 0 || len(raw) > 8<<20 {
		return ""
	}
	cuerpo := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodos, err := html.ParseFragment(strings.NewReader(string(raw)), cuerpo)
	if err != nil {
		return ""
	}
	var svg *html.Node
	for _, n := range nodos {
		if svg = buscarSVG(n); svg != nil {
			break
		}
	}
	if svg == nil {
		return ""
	}
	escena := ""
	for c := svg.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && strings.EqualFold(c.Data, "metadata") && attr(c, "id") == idEscenaSVG {
			escena = strings.TrimSpace(nodeText(c))
			break
		}
	}
	var obj map[string]json.RawMessage
	if escena == "" || len(escena) > maxEscenaSVG || json.Unmarshal([]byte(escena), &obj) != nil {
		return ""
	}
	ancho, alto, ok := svgCaja(svg)
	if !ok {
		return ""
	}
	e := &svgEscritor{}
	e.b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + svgNum(ancho) + ` ` + svgNum(alto) +
		`" width="100%" style="display:block;height:auto" role="img" aria-label="` + xmlEscape(alt) + `">`)
	e.hijos(svg, 0)
	e.b.WriteString(`</svg>`)
	img := ""
	if strings.HasPrefix(respaldo, "data:image/png;base64,") && w > 0 && h > 0 {
		ws, hs := strconv.Itoa(w), strconv.Itoa(h)
		img = `<img data-dibujo-respaldo hidden alt="` + xmlEscape(alt) + `" width="` + ws + `" height="` + hs +
			`" data-w="` + ws + `" data-h="` + hs + `" src="` + respaldo + `">`
	}
	return `<figure data-dibujo data-escena="` + xmlEscape(escena) + `" contenteditable="false" tabindex="0" ` +
		`style="margin:1em auto;width:100%">` + e.b.String() + img + `</figure>`
}

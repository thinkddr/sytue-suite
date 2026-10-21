// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// # iWork '09: el índice XML
//
// Un paquete '09 guarda todo el documento en `index.xml` (Pages, Numbers) o `index.apxl`
// (Keynote), a veces comprimido con gzip. Es XML con espacios de nombres propios; lo que
// se lee aquí se observó en los ficheros de prueba (registro de fuentes de
// `docs/security.md` §4.4.1) y está anotado junto a cada lectura.
//
// El índice se carga en un árbol mínimo (nodo09) con topes de tamaño, número de nodos y
// profundidad; si alguno salta, el fichero se abre por el escalón que no falla.

const (
	// maxIndice09 es lo más que se descomprime del índice. Los reales pesan de cientos de
	// KB a pocos MB: casi todo son hojas de estilos repetidas.
	maxIndice09 = 64 << 20
	// maxNodos09 y maxProfundidad09 acotan el árbol: un índice con millones de elementos
	// vacíos o anidado sin fin cabe en 64 MiB y tumbaría la memoria o la pila.
	maxNodos09       = 2_000_000
	maxProfundidad09 = 200
	// maxHerencia09 corta las cadenas de estilos (sf:parent-ident) circulares.
	maxHerencia09 = 32
)

var errIndice09 = errors.New("índice de iWork '09 ilegible o fuera de los topes")

// nodo09 es un elemento del índice, o un tramo de texto si `nombre` es "".
type nodo09 struct {
	// nombre lleva el prefijo de su espacio de nombres: «sf:p», «sl:document».
	nombre string
	// attr, con el mismo prefijo: «sfa:ID», «sf:style»; sin espacio de nombres, solo el nombre.
	attr  map[string]string
	hijos []*nodo09
	texto string
}

// prefijos09 son los espacios de nombres del índice con el prefijo con que se escriben.
var prefijos09 = map[string]string{
	"http://developer.apple.com/namespaces/sf":       "sf",
	"http://developer.apple.com/namespaces/sfa":      "sfa",
	"http://developer.apple.com/namespaces/sl":       "sl",
	"http://developer.apple.com/namespaces/ls":       "ls",
	"http://developer.apple.com/namespaces/keynote2": "key",
}

func nombre09(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	if p, ok := prefijos09[n.Space]; ok {
		return p + ":" + n.Local
	}
	return n.Space + ":" + n.Local
}

// hijo es el primer hijo directo llamado así, o nil.
func (n *nodo09) hijo(nombre string) *nodo09 {
	if n == nil {
		return nil
	}
	for _, h := range n.hijos {
		if h.nombre == nombre {
			return h
		}
	}
	return nil
}

// hijosDe son los hijos de n, o ninguno si n es nil.
func (n *nodo09) hijosDe() []*nodo09 {
	if n == nil {
		return nil
	}
	return n.hijos
}

// recorrer visita el subárbol en orden de documento; si `f` devuelve false, no baja.
func (n *nodo09) recorrer(f func(*nodo09) bool) {
	if n == nil || !f(n) {
		return
	}
	for _, h := range n.hijos {
		h.recorrer(f)
	}
}

// indice09 lee y analiza el índice XML de un paquete '09. nil si no está o se pasa de
// los topes.
func (p *paqueteIWork) indice09() *nodo09 {
	for _, nom := range []string{"index.xml", "index.apxl", "index.xml.gz", "index.apxl.gz"} {
		b := p.leer(nom, maxIndice09)
		if b == nil {
			continue
		}
		if strings.HasSuffix(nom, ".gz") {
			zr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				return nil
			}
			b, err = io.ReadAll(io.LimitReader(zr, maxIndice09+1))
			if err != nil || len(b) > maxIndice09 {
				return nil
			}
		}
		raiz, err := analizarIndice09(b)
		if err != nil {
			return nil
		}
		return raiz
	}
	return nil
}

func analizarIndice09(b []byte) (*nodo09, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var pila []*nodo09
	var raiz *nodo09
	nodos := 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errIndice09, err)
		}
		nodos++
		if nodos > maxNodos09 {
			return nil, errIndice09
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(pila) >= maxProfundidad09 {
				return nil, errIndice09
			}
			n := &nodo09{nombre: nombre09(t.Name)}
			if len(t.Attr) > 0 {
				n.attr = make(map[string]string, len(t.Attr))
				for _, a := range t.Attr {
					n.attr[nombre09(a.Name)] = a.Value
				}
			}
			if len(pila) > 0 {
				padre := pila[len(pila)-1]
				padre.hijos = append(padre.hijos, n)
			} else if raiz == nil {
				raiz = n
			}
			pila = append(pila, n)
		case xml.EndElement:
			if len(pila) > 0 {
				pila = pila[:len(pila)-1]
			}
		case xml.CharData:
			if len(pila) > 0 {
				padre := pila[len(pila)-1]
				padre.hijos = append(padre.hijos, &nodo09{texto: string(t)})
			}
		}
	}
	if raiz == nil {
		return nil, errIndice09
	}
	return raiz, nil
}

// ## Estilos
//
// `sf:paragraphstyle` y `sf:characterstyle` se identifican por `sfa:ID` o `sf:ident` y
// heredan de `sf:parent-ident`. Cada propiedad es un hijo de `sf:property-map`: vacío,
// sin definir (manda el padre); con `sf:null`, definida como nada; si no, su valor
// (`sf:number sfa:number`, `sf:color sfa:r/g/b` en 0..1, `sf:string sfa:string`).

type estilos09 map[string]*nodo09

func leerEstilos09(raiz *nodo09) estilos09 {
	e := estilos09{}
	raiz.recorrer(func(n *nodo09) bool {
		if n.nombre == "sf:paragraphstyle" || n.nombre == "sf:characterstyle" {
			// ponytail: el mismo ident puede venir en varias hojas de estilos (la de la
			// plantilla y la del documento); gana la primera, que en el corpus son iguales.
			for _, k := range []string{"sfa:ID", "sf:ident"} {
				if id := n.attr[k]; id != "" && e[id] == nil {
					e[id] = n
				}
			}
			return false
		}
		return true
	})
	return e
}

// prop es el valor de la propiedad `nombre` del primer estilo de `estilos` que la defina,
// subiendo por la herencia de cada uno. nil si ninguno la define o la define como nada.
func (e estilos09) prop(nombre string, estilos ...string) *nodo09 {
	for _, id := range estilos {
		for i := 0; id != "" && i < maxHerencia09; i++ {
			s := e[id]
			if s == nil {
				break
			}
			if v := s.hijo("sf:property-map").hijo("sf:" + nombre); v != nil {
				for _, h := range v.hijos {
					if h.nombre == "sf:null" {
						return nil
					}
					if h.nombre != "" {
						return h
					}
				}
			}
			id = s.attr["sf:parent-ident"]
		}
	}
	return nil
}

// num es el número de una propiedad, o 0.
func (e estilos09) num(nombre string, estilos ...string) float64 {
	v := e.prop(nombre, estilos...)
	if v == nil {
		return 0
	}
	f, _ := strconv.ParseFloat(v.attr["sfa:number"], 64)
	return f
}

// nombre es el primer `sf:name` de la cadena de herencia del estilo.
func (e estilos09) nombre(id string) string {
	for i := 0; id != "" && i < maxHerencia09; i++ {
		s := e[id]
		if s == nil {
			return ""
		}
		if n := s.attr["sf:name"]; n != "" {
			return n
		}
		id = s.attr["sf:parent-ident"]
	}
	return ""
}

// color es «RRGGBB» de `sf:fontColor`, o "" si no lo define o es negro (el del editor).
func (e estilos09) color(estilos ...string) string {
	v := e.prop("fontColor", estilos...)
	if v == nil || v.nombre != "sf:color" {
		return ""
	}
	var hex strings.Builder
	for _, k := range []string{"sfa:r", "sfa:g", "sfa:b"} {
		f, err := strconv.ParseFloat(v.attr[k], 64)
		if err != nil {
			return ""
		}
		fmt.Fprintf(&hex, "%02X", int(min(max(f, 0), 1)*255+0.5))
	}
	if hex.String() == "000000" {
		return ""
	}
	return hex.String()
}

// tituloDeEstilo09 traduce el nombre del estilo de párrafo al título del editor:
// «Title» y «Heading N» son los de las plantillas.
func tituloDeEstilo09(nombre string) string {
	switch {
	case nombre == "Title":
		return "Heading1"
	case strings.HasPrefix(nombre, "Heading"):
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(nombre, "Heading")))
		if err != nil || n < 1 {
			n = 1
		}
		return "Heading" + strconv.Itoa(min(n, 6))
	}
	return ""
}

// alineacion09: `sf:alignment` 0 izquierda, 1 derecha, 2 centro, 3 justificado (4, natural).
func alineacion09(v float64) string {
	switch v {
	case 1:
		return "right"
	case 2:
		return "center"
	case 3:
		return "justify"
	}
	return ""
}

// ## Texto
//
// Un almacén de texto (`sf:text-storage`) lleva `sf:text-body` > [`sf:section` >
// `sf:layout` >] `sf:p[sf:style]`. Dentro de un párrafo: texto, `sf:span[sf:style]`,
// `sf:br` (fin de párrafo: puede haber varios en un mismo `sf:p`), `sf:lnbr` (salto de
// línea), `sf:tab`, `sf:link[href]`, `sf:footnote[sf:autonumber]`, `sf:page-number`,
// `sf:attachment-ref` (tablas e imágenes en línea) y envoltorios sin formato propio
// (`sf:ghost-text`, el texto de muestra de la plantilla, que se ve y se imprime;
// `sf:annotation-field`, el tramo comentado).

// lector09 convierte almacenes de texto al modelo rico del editor.
type lector09 struct {
	est estilos09
	// notas son las notas al pie por su número (`sf:footnote-mark sf:mark`).
	notas map[string][]xrun
	// usadas, las notas ya serializadas en el orden de sus llamadas.
	usadas []string
	// adjuntos son los objetos anclados en el texto por su `sfa:ID` (`sf:attachments` >
	// `sf:attachment`); de ellos se convierten las tablas.
	adjuntos map[string]*nodo09
	// vinetas: los párrafos con `sf:list-level` son puntos de una lista (el cuerpo de una
	// diapositiva; el nivel 1 es el primero).
	vinetas bool
	// omitido: algo del fichero (una imagen, un gráfico) no se ha convertido.
	omitido bool
}

// bloques son los párrafos y tablas de un almacén de texto.
func (l *lector09) bloques(almacen *nodo09) []xblock {
	var out []xblock
	almacen.hijo("sf:text-body").recorrer(func(n *nodo09) bool {
		if n.nombre == "sf:p" {
			out = l.parrafo(n, out)
			return false
		}
		return true
	})
	return out
}

// tramos son todos los tramos de unos bloques, en orden (sin las tablas).
func tramos(bs []xblock) []xrun {
	var out []xrun
	for _, b := range bs {
		if b.para != nil {
			out = append(out, b.para.runs...)
		}
	}
	return out
}

func (l *lector09) parrafo(p *nodo09, out []xblock) []xblock {
	ep := p.attr["sf:style"]
	base := l.base09(p)
	cur := base
	cerrar := func() {
		c := cur
		out = append(out, xblock{para: &c})
		cur = base
	}
	var enLinea func(n *nodo09, ec, enlace string)
	enLinea = func(n *nodo09, ec, enlace string) {
		for _, h := range n.hijos {
			switch h.nombre {
			case "":
				// Un índice reformateado con sangrías trae blancos con salto de línea entre
				// los elementos; un salto de verdad nunca es texto, siempre un elemento.
				if strings.TrimSpace(h.texto) == "" && strings.Contains(h.texto, "\n") {
					continue
				}
				cur.runs = append(cur.runs, l.tramo(h.texto, ec, ep, enlace, base.style != ""))
			case "sf:br":
				cerrar()
			case "sf:lnbr":
				cur.runs = append(cur.runs, xrun{brk: true})
			case "sf:tab":
				cur.runs = append(cur.runs, l.tramo("\t", ec, ep, enlace, false))
			case "sf:span":
				enLinea(h, h.attr["sf:style"], enlace)
			case "sf:link":
				enLinea(h, ec, h.attr["href"])
			case "sf:page-number":
				cur.runs = append(cur.runs, xrun{field: "PAGE", text: "1"})
			case "sf:footnote":
				if nota, ok := l.notas[h.attr["sf:autonumber"]]; ok {
					l.usadas = append(l.usadas, runsToHTML(nota))
					cur.runs = append(cur.runs, xrun{raw: footnoteMarker(len(l.usadas))})
				}
			case "sf:attachment-ref":
				// Una tabla es un bloque: corta el párrafo en el que está anclada.
				if t := leerTabla09(primerModelo09(l.adjuntos[h.attr["sfa:IDREF"]])); t != nil {
					if len(cur.runs) > 0 {
						cerrar()
					}
					out = append(out, xblock{table: t.xtable()})
				} else {
					l.omitido = true
				}
			case "sf:footnote-mark", "sf:footnotebr", "sf:crbr", "sf:pgbr", "sf:sectbr", "sf:contbr",
				"sf:insertion-point", "sf:selection-start", "sf:selection-end", "sf:page-start", "sf:container-hint":
				// marcas de maquetación o de la selección: no son texto
			default:
				enLinea(h, ec, enlace)
			}
		}
	}
	enLinea(p, "", "")
	if len(cur.runs) > 0 {
		cerrar()
	}
	return out
}

// base09 es el párrafo vacío con el estilo de bloque de `p`: título, alineación y, en el
// cuerpo de una diapositiva, su nivel de lista.
func (l *lector09) base09(p *nodo09) xpara {
	ep := p.attr["sf:style"]
	base := xpara{style: tituloDeEstilo09(l.est.nombre(ep)), align: alineacion09(l.est.num("alignment", ep))}
	if n, err := strconv.Atoi(p.attr["sf:list-level"]); l.vinetas && err == nil && n >= 1 {
		base.list, base.level = "bullet", n-1
	}
	return base
}

// tramo es un texto con el formato de su estilo de carácter `ec` y, por debajo, el del
// párrafo `ep`. En un título la negrita del párrafo no se repite: ya la pone el título.
func (l *lector09) tramo(texto, ec, ep, enlace string, titulo bool) xrun {
	bandera := func(nombre string) bool {
		if titulo {
			return l.est.num(nombre, ec) != 0
		}
		return l.est.num(nombre, ec, ep) != 0
	}
	return xrun{
		text: texto, link: enlace,
		b: bandera("bold"), i: bandera("italic"), u: bandera("underline"), strike: bandera("strikethru"),
		color: l.est.color(ec, ep),
	}
}

// leerNotas09 guarda las notas al pie de un almacén del cuerpo: `sf:footnotes` >
// `sf:text-storage` > un párrafo por nota, que empieza con su `sf:footnote-mark`.
func (l *lector09) leerNotas09(cuerpo *nodo09) {
	l.notas = map[string][]xrun{}
	for _, almacen := range cuerpo.hijo("sf:footnotes").hijosDe() {
		almacen.hijo("sf:text-body").recorrer(func(n *nodo09) bool {
			if n.nombre != "sf:p" {
				return true
			}
			marca := ""
			n.recorrer(func(m *nodo09) bool {
				if m.nombre == "sf:footnote-mark" && marca == "" {
					marca = m.attr["sf:mark"]
				}
				return marca == ""
			})
			runs := tramos(l.parrafo(n, nil))
			if len(runs) > 0 {
				runs[0].text = strings.TrimLeft(runs[0].text, " ")
			}
			if marca != "" && len(runs) > 0 {
				l.notas[marca] = runs
			}
			return false
		})
	}
}

// leerAdjuntos09 indexa los objetos anclados de un almacén por su `sfa:ID`.
func (l *lector09) leerAdjuntos09(almacen *nodo09) {
	if l.adjuntos == nil {
		l.adjuntos = map[string]*nodo09{}
	}
	for _, a := range almacen.hijo("sf:attachments").hijosDe() {
		if id := a.attr["sfa:ID"]; id != "" {
			l.adjuntos[id] = a
		}
	}
}

// region09 es el HTML en línea del primer encabezado o pie con texto de la lista (`sf:headers` >
// `sf:header` > `sf:text-storage`): el del editor es una sola línea, así que los párrafos
// se juntan con un espacio.
func (l *lector09) region09(lista *nodo09) string {
	for _, r := range lista.hijosDe() {
		var runs []xrun
		for _, b := range l.bloques(r.hijo("sf:text-storage")) {
			if b.para == nil {
				continue
			}
			if len(runs) > 0 {
				runs = append(runs, xrun{text: " "})
			}
			runs = append(runs, b.para.runs...)
		}
		if strings.TrimSpace(xpara{runs: runs}.plain()) != "" {
			return runsToHTML(runs)
		}
	}
	return ""
}

// conTexto dice si unos bloques llevan algo que leer: texto o una tabla.
func conTexto(bs []xblock) bool {
	for _, b := range bs {
		if b.table != nil || b.para != nil && strings.TrimSpace(b.para.plain()) != "" {
			return true
		}
	}
	return false
}

// importPages09 convierte el índice de un documento .pages '09: el cuerpo con sus notas y
// tablas, el encabezado y el pie, y el texto de los cuadros de la maquetación libre (que
// van detrás del cuerpo: no tienen sitio en un flujo de texto). "" si no hay texto.
//
// `sl:document` > `sf:text-storage[sf:kind=body]` es el cuerpo: el hijo DIRECTO; los de
// `sl:section-prototypes` son el texto de muestra de la plantilla. Los cuadros de texto
// y las tablas flotantes están en `sl:drawables` > `sl:page-group` (`sl:masters-group` es
// la maqueta de la plantilla).
func importPages09(raiz *nodo09) (cuerpo string, omitido bool) {
	l := &lector09{est: leerEstilos09(raiz)}
	var blocks []xblock
	for _, h := range raiz.hijos {
		if h.nombre == "sf:text-storage" && h.attr["sf:kind"] == "body" {
			l.leerNotas09(h)
			l.leerAdjuntos09(h)
			blocks = append(blocks, l.bloques(h)...)
		}
	}
	for _, g := range raiz.hijo("sl:drawables").hijosDe() {
		if g.nombre != "sl:page-group" {
			continue
		}
		g.recorrer(func(n *nodo09) bool {
			switch n.nombre {
			case "sf:text-storage":
				blocks = append(blocks, l.bloques(n)...)
				return false
			case "sf:tabular-model":
				if t := leerTabla09(n); t != nil {
					blocks = append(blocks, xblock{table: t.xtable()})
				}
				return false
			case "sf:image", "sf:media", "sf:chart-info":
				l.omitido = true
				return false
			}
			return true
		})
	}
	if !conTexto(blocks) {
		return "", l.omitido
	}
	if h := l.region09(raiz.hijo("sf:headers")); h != "" {
		blocks = append([]xblock{{raw: `<div data-doc-header style="` + docHeaderStyle + `">` + h + `</div>`}}, blocks...)
	}
	if f := l.region09(raiz.hijo("sf:footers")); f != "" {
		blocks = append(blocks, xblock{raw: `<div data-doc-footer style="` + docFooterStyle + `">` + f + `</div>`})
	}
	if sec := footnotesSection(l.usadas); sec != "" {
		blocks = append(blocks, xblock{raw: sec})
	}
	return blocksToHTML(blocks), l.omitido
}

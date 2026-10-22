// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// # Pegar con formato (`handlePaste` de RichTextEditor.tsx)
//
// Pegar en el cuerpo conserva el formato, como en la web: el HTML del portapapeles del
// sistema (`apps/nucleo/sistema/portapapeles`) se SANEA con la misma lista blanca que usa
// la web (`sanitizeHtml` de `client/richtext.ts`, portada aquí tal cual), se lee con el
// modelo de Notepads y se pega en el cursor. Sin HTML, o con ⌘/Ctrl + Mayús + V, se pega
// el texto, como allí.
//
// Del HTML saneado entra lo que el modelo de Notepads pinta: negrita, cursiva, subrayado,
// tachado, código, enlaces, colores, tipografía y tamaño, párrafos, títulos, listas, citas,
// reglas, tablas e imágenes: las `https:` y `cid:` tal cual, y las `data:` (lo que copia
// un navegador de una imagen suelta, o un editor de escritorio) como partes del mensaje
// (SacarImagenesEnLinea). Las que el saneado tira (`http:`, SVG) no entran, y se dice
// (ImagenesDescartadas).

// Las listas de la web, en su orden.
var (
	etiquetasPegado = conjunto("p", "br", "div", "span", "b", "strong", "i", "em", "u", "s",
		"strike", "a", "ul", "ol", "li", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6", "pre",
		"code", "hr", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "caption", "colgroup",
		"col", "img", "figure", "figcaption", "sub", "sup")
	estilosPegado = conjunto("color", "background-color", "font-family", "font-size",
		"font-style", "font-weight", "text-align", "text-decoration", "vertical-align",
		"line-height", "letter-spacing", "border", "border-top", "border-right",
		"border-bottom", "border-left", "border-color", "border-style", "border-width",
		"border-collapse", "border-radius", "padding", "padding-top", "padding-right",
		"padding-bottom", "padding-left", "margin", "margin-top", "margin-right",
		"margin-bottom", "margin-left", "width", "height", "max-width", "min-width",
		"white-space")
	atributosPegado = map[string][]string{
		"img":      {"alt", "width", "height", "title"},
		"table":    {"border", "cellpadding", "cellspacing", "width", "align"},
		"td":       {"colspan", "rowspan", "align", "valign", "width", "height"},
		"th":       {"colspan", "rowspan", "align", "valign", "width", "height", "scope"},
		"tr":       {"align", "valign"},
		"col":      {"span", "width"},
		"colgroup": {"span", "width"},
	}
	// soltarConTodo son las que se van con su contenido: nunca viajan en un correo.
	soltarConTodo = conjunto("script", "style", "iframe", "object", "embed", "noscript",
		"template", "link", "meta", "base", "title", "head", "svg", "math", "form", "input",
		"button", "textarea", "select", "option")
	vacias = conjunto("br", "hr", "img", "col")
	// blancos es lo que el navegador junta en un espacio fuera de `<pre>`: los saltos de
	// línea con que Word parte sus párrafos en el HTML no son saltos del texto.
	blancos  = regexp.MustCompile(`[ \t\n\r\f]+`)
	espacios = regexp.MustCompile(` {2,}`)
)

func conjunto(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// SanearHTML es `sanitizeHtml`: la lista blanca de etiquetas, atributos y propiedades de
// estilo de la web; lo desconocido se desenvuelve (se quedan sus hijos) y lo peligroso se
// va con su contenido. Los enlaces solo `http(s)`, `mailto`, rutas y anclas; las imágenes
// solo `https:` y `cid:`; ningún `style` con `url(`, `expression` ni `javascript:`.
func SanearHTML(entrada string) string {
	doc, err := html.Parse(strings.NewReader(entrada))
	if err != nil {
		return ""
	}
	var b strings.Builder
	sanearEn(&b, cuerpoDe(doc), 0, false)
	return b.String()
}

// cuerpoDe es el `<body>` del documento (el analizador lo pone siempre).
func cuerpoDe(n *html.Node) *html.Node {
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if h.Type == html.ElementNode && h.Data == "body" {
			return h
		}
		if c := cuerpoDe(h); c != nil {
			return c
		}
	}
	return nil
}

func sanearEn(b *strings.Builder, n *html.Node, prof int, pre bool) {
	if n == nil || prof > 200 { // lo pegado viene de fuera: sin fondo no se recorre
		return
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		switch h.Type {
		case html.TextNode:
			t := h.Data
			if !pre {
				t = blancos.ReplaceAllString(t, " ")
			}
			b.WriteString(html.EscapeString(t))
			continue
		case html.ElementNode:
		case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
			continue // comentarios (los condicionales de Word incluidos): fuera
		}
		etiqueta := h.Data
		switch {
		case soltarConTodo[etiqueta]:
			continue
		case !etiquetasPegado[etiqueta]:
			sanearEn(b, h, prof+1, pre)
			continue
		}
		atrs := atributosSaneados(etiqueta, h)
		if atrs == nil {
			continue // una imagen sin `src` admisible se va entera: no una imagen rota
		}
		b.WriteString("<" + etiqueta + atrs[0] + ">")
		if vacias[etiqueta] {
			continue
		}
		sanearEn(b, h, prof+1, pre || etiqueta == "pre")
		b.WriteString("</" + etiqueta + ">")
	}
}

// atributosSaneados escribe los atributos que quedan, o nil si el elemento no se admite.
func atributosSaneados(etiqueta string, n *html.Node) []string {
	var b strings.Builder
	poner := func(k, v string) { b.WriteString(" " + k + `="` + html.EscapeString(v) + `"`) }
	switch etiqueta {
	case "a":
		if href, ok := atributoDe(n, "href"); ok && urlSegura(href) {
			poner("href", href)
			poner("rel", "noopener noreferrer")
		}
	case "img":
		src, _ := atributoDe(n, "src")
		if !imagenSegura(src) {
			return nil
		}
		poner("src", src)
	}
	for _, k := range atributosPegado[etiqueta] {
		if v, ok := atributoDe(n, k); ok && !strings.Contains(strings.ToLower(v), "javascript:") {
			poner(k, v)
		}
	}
	if estilo := estiloSaneado(n); estilo != "" {
		poner("style", estilo)
	}
	return []string{b.String()}
}

func atributoDe(n *html.Node, k string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == k {
			return a.Val, true
		}
	}
	return "", false
}

// estiloSaneado es `cleanStyle`: la propiedad EXACTA en la lista, y fuera cualquier valor
// con `url(`, `expression`, `javascript:`, comentarios o escapes.
func estiloSaneado(n *html.Node) string {
	crudo, _ := atributoDe(n, "style")
	var out []string
	for decl := range strings.SplitSeq(crudo, ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		prop, valor = strings.ToLower(strings.TrimSpace(prop)), strings.TrimSpace(valor)
		if !ok || !estilosPegado[prop] || valor == "" {
			continue
		}
		v := strings.ToLower(valor)
		if strings.Contains(v, "url(") || strings.Contains(v, "expression") || strings.Contains(v, "javascript:") ||
			strings.Contains(v, "/*") || strings.Contains(v, `\`) {
			continue
		}
		out = append(out, prop+": "+valor)
	}
	return strings.Join(out, "; ")
}

// urlSegura es `safeUrl`: http(s), mailto, rutas y anclas.
func urlSegura(u string) bool {
	v := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") ||
		strings.HasPrefix(v, "mailto:") || strings.HasPrefix(v, "/") || strings.HasPrefix(v, "#")
}

// imagenSegura es `safeImageSrc`: `https:` y `cid:`; ni `http:` (delata a quien lee) ni
// `data:` (infla el mensaje y es por donde se cuela un SVG con script).
func imagenSegura(u string) bool {
	v := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "cid:")
}

// DocumentoPegado es lo que entra en el cuerpo al pegar ese HTML: saneado, leído con el
// modelo de Notepads y sin el blanco de los bordes de cada línea, que el navegador no
// pinta. nil si no queda nada que pegar.
func DocumentoPegado(h string) *notes.Documento {
	d := notes.Leer(SanearHTML(h))
	for i := range d.Bloques {
		bl := &d.Bloques[i]
		if bl.Tipo == notes.Codigo || len(bl.Tramos) == 0 {
			continue
		}
		// El blanco se junta entre tramos como en el navegador: «Mira <img> esto» deja
		// dos espacios seguidos al quitar la imagen, y se pinta uno.
		espacio := true // al principio de la línea, el blanco no se pinta
		for k := range bl.Tramos {
			tr := &bl.Tramos[k]
			if tr.Salto {
				espacio = true
				continue
			}
			if tr.Imagen != nil { // una imagen es contenido: el blanco de su lado se pinta
				espacio = false
				continue
			}
			tr.Texto = espacios.ReplaceAllString(tr.Texto, " ")
			if espacio {
				tr.Texto = strings.TrimLeft(tr.Texto, " ")
			}
			if tr.Texto != "" {
				espacio = strings.HasSuffix(tr.Texto, " ")
			}
		}
		u := len(bl.Tramos) - 1
		bl.Tramos[u].Texto = strings.TrimRight(bl.Tramos[u].Texto, " ")
		bl.Tramos = slices.DeleteFunc(bl.Tramos, func(t notes.Tramo) bool { return !t.Salto && t.Imagen == nil && t.Texto == "" })
	}
	d.Bloques = slices.DeleteFunc(d.Bloques, func(bl notes.Bloque) bool {
		return len(bl.Tramos) == 0 && bl.Tipo != notes.Regla && bl.Tipo != notes.BloqueOpaco
	})
	if d.Vacio() {
		return nil
	}
	return d
}

// tiposEnLinea son las imágenes `data:` que entran al pegar, con la extensión de su parte:
// las de mapa de bits que pinta cualquier cliente. Un SVG no, que es por donde se cuela
// un script.
var tiposEnLinea = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}

// SacarImagenesEnLinea cambia cada `<img src="data:…;base64,…">` de un HTML pegado por un
// `cid:` (de `nuevoCID`) y devuelve sus bytes como partes del mensaje, que van DENTRO del
// cuerpo como las de «Desde este dispositivo». Así el saneado, que tira las `data:`, las
// deja entrar. Las de dentro de lo que se va entero (un `<noscript>`) no. Sin ninguna, el
// HTML vuelve tal cual.
func SacarImagenesEnLinea(h string, nuevoCID func() string) (string, []eml.Adjunto) {
	doc, err := html.Parse(strings.NewReader(h))
	if err != nil {
		return h, nil
	}
	var partes []eml.Adjunto
	var recorrer func(*html.Node, int)
	recorrer = func(n *html.Node, prof int) {
		for c := n.FirstChild; c != nil && prof < 200; c = c.NextSibling {
			if c.Type == html.ElementNode && soltarConTodo[c.Data] {
				continue // lo que el saneado tira entero: su imagen no se verá
			}
			for i, a := range c.Attr {
				if c.Type != html.ElementNode || c.Data != "img" || a.Namespace != "" || a.Key != "src" {
					continue
				}
				if p, ok := imagenDeDatos(a.Val); ok {
					p.CID = nuevoCID()
					p.Nombre = fmt.Sprintf("image%03d.%s", len(partes)+1, tiposEnLinea[p.Tipo])
					c.Attr[i].Val = "cid:" + p.CID
					partes = append(partes, p)
				}
			}
			recorrer(c, prof+1)
		}
	}
	recorrer(doc, 0)
	var b strings.Builder
	if len(partes) == 0 || html.Render(&b, doc) != nil {
		return h, nil
	}
	return b.String(), partes
}

// imagenDeDatos lee una dirección `data:<tipo>;base64,<datos>` de un tipo que entra.
func imagenDeDatos(src string) (eml.Adjunto, bool) {
	cabeza, datos, ok := strings.Cut(strings.TrimSpace(src), ",")
	cabeza = strings.ToLower(cabeza)
	tipo, b64 := strings.CutSuffix(strings.TrimPrefix(cabeza, "data:"), ";base64")
	if _, admitido := tiposEnLinea[tipo]; !ok || !b64 || !admitido || !strings.HasPrefix(cabeza, "data:") {
		return eml.Adjunto{}, false
	}
	crudo, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(datos), ""))
	if err != nil || len(crudo) == 0 {
		return eml.Adjunto{}, false
	}
	return eml.Adjunto{Tipo: tipo, Datos: crudo}, true
}

// ImagenesDescartadas son las imágenes de ese HTML que no llegan al cuerpo al pegarlo
// —las que tira el saneado—, para decirlo en vez de perderlas en silencio.
func ImagenesDescartadas(h string) int {
	return max(imagenesEn(h)-imagenesEn(SanearHTML(h)), 0)
}

// imagenesEn cuenta los `<img>` de un HTML, sin los de lo que se va entero con su
// contenido (un `<noscript>`), que nunca iban a verse.
func imagenesEn(h string) int {
	doc, err := html.Parse(strings.NewReader(h))
	if err != nil {
		return 0
	}
	n := 0
	var contar func(*html.Node, int)
	contar = func(x *html.Node, prof int) {
		for c := x.FirstChild; c != nil && prof < 200; c = c.NextSibling {
			if c.Type != html.ElementNode || soltarConTodo[c.Data] {
				continue
			}
			if c.Data == "img" {
				n++
			}
			contar(c, prof+1)
		}
	}
	contar(cuerpoDe(doc), 0)
	return n
}

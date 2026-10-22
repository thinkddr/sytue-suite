// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"image"
	_ "image/gif" // imágenes GIF embebidas en Office
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Modelo intermedio rico para exportar un KindDocument con FIDELIDAD. El editor
// guarda el documento como HTML (registro doc:html); aquí ese HTML se parsea a
// una lista de bloques (párrafos con estilo/alineación/listas + tablas) cuyos
// runs conservan negrita/cursiva/subrayado/tachado, color y enlaces. Los
// serializadores a OOXML (docx) y ODF (odt) —y el motor PDF— consumen este
// modelo, de modo que el mapeo del HTML se hace UNA sola vez.
//
// Clean-room: se usa el tokenizador HTML de golang.org/x/net/html (BSD, ya es
// dependencia del repo) porque el HTML del editor no es XML válido y un parser
// a mano perdería robustez. El mapeo a OOXML/ODF es obra original sobre las
// especificaciones públicas ECMA-376 y OASIS ODF.

// xrun es un tramo de texto con su formato inline.
type xrun struct {
	text   string
	b      bool    // negrita
	i      bool    // cursiva
	u      bool    // subrayado
	strike bool    // tachado
	color  string  // "RRGGBB" (sin #); "" = color por defecto
	link   string  // href del enlace; "" = sin enlace
	brk    bool    // salto de línea duro (<br>); text se ignora
	field  string  // campo de página: "PAGE" o "NUMPAGES"; text lleva su valor en caché
	track  *xtrack // revisión (control de cambios) a la que pertenece el tramo
	mark   string  // id del comentario (<mark data-comment-id>) cuyo texto anclado incluye el tramo
	raw    string  // HTML literal construido por el importador (imagen); text se ignora
	image  *ximage // imagen embebida del editor; nunca se descarga una URL externa
	note   *xnote  // nota ligada a su llamada, no una lista ordinaria
	lang   string  // idioma BCP 47 del tramo (atributo lang del HTML); "" = el del documento
	// css son declaraciones de CSS que el importador ha calculado (tipografía, tamaño, fondo…);
	// vert, "sup" o "sub". css solo lo rellena el importador de .docx: el HTML lo lleva al editor.
	css, vert string
	// tam es el tamaño (font-size, en px) que el HTML del editor da al tramo, o a su párrafo;
	// 0 = el de su estilo. Lo lee el exportador de Diapos (export_slides.go).
	tam float64
	// Lo que la exportación a OOXML lee del CSS del editor (conCSS, export_docx_formato.go): la
	// fuente y su genérica, el fondo y el borde del tramo, el tamaño (tamPt), el espaciado entre
	// letras y el desplazamiento en puntos, y mayúsculas, versalitas, oculto y la negrita o la
	// cursiva quitadas a propósito (un título sin negrita).
	fuente, generica, fondo, borde        string
	tamPt, espaciado, posicion            float64
	mayus, versalitas, oculto, sinB, sinI bool
	// pre es `white-space: pre-wrap` (o pre) en el bloque: los blancos y los tabuladores del texto
	// son del documento y no se juntan (los de un .docx importado).
	pre bool
}

// xtrack es una revisión del control de cambios: la inserción («ins») o el borrado («del»)
// de un autor, con su sello de tiempo en milisegundos (0 = sin fecha).
type xtrack struct {
	kind, author string
	ts           int64
}

type xnote struct {
	class string // footnote o endnote
	runs  []xrun
}

type ximage struct {
	src           string
	width, height int
	alt           string
	// svg es la versión vectorial de un dibujo de Docus (saneada, con su escena): DOCX y ODT la
	// llevan junto al PNG, que queda de respaldo para quien no pinte SVG.
	svg []byte
}

type embeddedImage struct {
	ext           string
	data          []byte
	width, height int
}

// La validación es común a DOCX/ODF: solo PNG/JPEG/GIF embebidos, con cabecera
// coherente con el MIME. DecodeConfig no descomprime todos los píxeles.
func (img *ximage) embedded() *embeddedImage {
	ext, data, ok := decodeDataURL(img.src)
	if !ok {
		return nil
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil
	}
	if format != ext && (format != "jpeg" || ext != "jpg") {
		return nil
	}
	width, height := img.width, img.height
	if width <= 0 {
		width = config.Width
	}
	if height <= 0 {
		height = config.Height
	}
	return &embeddedImage{ext: ext, data: data, width: min(width, 10000), height: min(height, 10000)}
}

// hasFmt indica si el run lleva algún formato de carácter (para omitir rPr/span).
func (r xrun) hasFmt() bool {
	return r.b || r.i || r.u || r.strike || r.color != ""
}

// xpara es un párrafo con estilo de bloque, alineación y (si aplica) lista.
type xpara struct {
	style string // "" | "Heading1".."Heading6" | "Quote"
	align string // "" | "center" | "right" | "justify"
	list  string // "" | "bullet" | "number"
	level int    // nivel de anidamiento de lista (0-based)
	runs  []xrun
	css   string // style del bloque calculado por el importador (espaciado, sangrías, fondo…)
	// vineta es la viñeta propia de un elemento de lista importado («-»); "" es la del editor.
	vineta string
	// marco es el CSS del marco de texto en el que va el párrafo (div[data-marco]); "" si no va en
	// ninguno. Lo pone el importador de .docx (framePr) y lo lee la exportación (framePrDocx).
	marco string
}

// text devuelve el texto plano concatenado del párrafo.
func (p xpara) plain() string {
	var b strings.Builder
	for _, r := range p.runs {
		if r.brk {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(r.text)
	}
	return b.String()
}

// xcell es una celda de tabla: uno o más párrafos. colspan/rowspan (0 = 1) la combinan
// con las vecinas; tras normalize las que quedan debajo de otra combinada llevan
// covered y las coordenadas de su celda de origen.
type xcell struct {
	paras            []xpara
	colspan, rowspan int
	covered          bool
	orow, ocol       int
	header           bool   // <th>
	css              string // style de la celda calculado por el importador (bordes, fondo…)
}

// xtable es una tabla rectangular de celdas.
type xtable struct {
	rows [][]xcell
	cols int
	css  string // style de la tabla calculado por el importador (ancho, sangría)
}

// xblock es un bloque de nivel documento: párrafo, tabla o regla horizontal.
type xblock struct {
	para  *xpara
	table *xtable
	hr    bool
	raw   string // HTML literal construido por el importador (sección de notas)
}

// xdoc es un documento listo para exportar: el cuerpo y lo que en OOXML/ODF no va en él, el
// encabezado y el pie (un único párrafo en línea, que es lo que el editor sabe editar) y si
// la primera página lleva el encabezado vacío.
type xdoc struct {
	blocks         []xblock
	header, footer *xpara
	firstDiff      bool
	pagina         *xpagina        // la capa data-sytue-page: tamaño, márgenes y color del papel
	marks          map[string]bool // ids de los <mark data-comment-id> del cuerpo
	// Comentarios, ya repartidos por exportComments.
	comments []xcomment
	byMark   map[string][]int
	points   []int
}

// documentBlocks construye el modelo rico de un documento: del HTML del editor
// (registro doc:html) si existe; si no, de los párrafos de texto plano (RGA).
func documentBlocks(kind Kind, s *State) []xblock {
	if kind == KindDocument {
		if h := strings.TrimSpace(s.documentHTML()); h != "" {
			if blk := parseHTMLBlocks(h); len(blk) > 0 {
				return blk
			}
		}
	}
	var out []xblock
	for _, p := range splitParagraphs(s.PlainText()) {
		para := xpara{}
		if p != "" {
			para.runs = []xrun{{text: p}}
		}
		out = append(out, xblock{para: &para})
	}
	return out
}

// parseHTMLBlocks parsea un fragmento HTML del editor a bloques de documento.
func parseHTMLBlocks(fragment string) []xblock {
	return parseHTMLDoc(fragment, false).blocks
}

// parseHTMLDoc parsea el HTML del editor. Con office, el de una exportación a OOXML/ODF,
// las notas viajan con su llamada, el encabezado y el pie salen del cuerpo y el número de
// página se vuelve campo; PDF y texto plano siguen viéndolo todo en el folio.
func parseHTMLDoc(fragment string, office bool) xdoc { return parseHTMLDocCon(fragment, office, false) }

// parseHTMLDocCon es parseHTMLDoc con imagenes: el PDF accesible quiere las imágenes (y la de
// respaldo de un bloque Dibujo) con su texto alternativo, no un marcador de texto.
func parseHTMLDocCon(fragment string, office, imagenes bool) xdoc {
	ctx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), ctx)
	if err != nil {
		return xdoc{}
	}
	b := &rtBuilder{office: office, imagenes: imagenes}
	if office {
		b.marks = map[string]bool{}
		b.notes, b.noteBodies = collectHTMLNotes(nodes)
	}
	for _, n := range nodes {
		b.walk(n, xrun{})
	}
	b.flush()
	return xdoc{blocks: b.blocks, header: b.header, footer: b.footer, firstDiff: b.firstDiff, pagina: b.pagina, marks: b.marks}
}

// rtBuilder acumula bloques mientras recorre el árbol HTML.
type rtBuilder struct {
	blocks []xblock
	cur    *xpara // párrafo abierto (nil = ninguno)
	lists  []bool // pila de listas activas: true = ordenada (ol)
	// vinetas es, en paralelo a lists, la viñeta propia de cada lista (list-style-type con una
	// cadena, la de un .docx importado); "" es la del editor. Se hereda, como en CSS.
	vinetas []string
	// marco es el CSS del div[data-marco] en el que se está: sus párrafos van en ese marco.
	marco      string
	depth      int // profundidad de recursión (tope anti-desbordamiento de pila)
	notes      map[string]*xnote
	noteBodies map[*html.Node]bool
	office     bool // exportación a OOXML/ODF (ver parseHTMLDoc)
	imagenes   bool // exportación a PDF accesible: las imágenes viajan como imágenes
	header     *xpara
	footer     *xpara
	firstDiff  bool
	pagina     *xpagina
	marks      map[string]bool
}

// xpagina es la página de un documento, en px: la capa div[data-sytue-page] que guardan el
// editor y rico.
type xpagina struct {
	ancho, alto float64
	margenes    [4]float64 // top, right, bottom, left
	color       string     // RRGGBB; "" es el blanco
}

// leerPagina lee la capa de la página; lo que no se entiende vale lo del editor (A4, 96 px).
func leerPagina(n *html.Node) *xpagina {
	num := func(v string, defecto, minimo, maximo float64) float64 {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f < minimo || f > maximo {
			return defecto
		}
		return f
	}
	p := &xpagina{ancho: num(attr(n, "data-width"), 794, 96, 4000), alto: num(attr(n, "data-height"), 1123, 96, 4000)}
	m := strings.Fields(attr(n, "data-margins"))
	for i := range p.margenes {
		p.margenes[i] = 96
		if i < len(m) {
			p.margenes[i] = num(m[i], 96, 0, 2000)
		}
	}
	if c := strings.TrimPrefix(attr(n, "data-color"), "#"); len(c) == 6 && isHex(c) {
		p.color = strings.ToUpper(c)
	}
	return p
}

// maxWalkDepth acota el anidamiento HTML recorrido. El blob del editor está
// capado (~1 MiB) pero podría contener miles de <div> anidados: sin tope, la
// recursión desbordaría la pila y tumbaría el proceso (entrada no confiable).
const maxWalkDepth = 600

func (b *rtBuilder) flush() {
	if b.cur != nil {
		// Un <br> al final de un bloque no abre otra línea en el navegador: es el que pone el
		// editor en un párrafo vacío (<p><br></p>). Sin quitarlo, cada línea en blanco salía al
		// .docx con un salto dentro y medía el doble. Un salto con control de cambios (<del><br></del>)
		// sí es contenido del documento y se conserva.
		if n := len(b.cur.runs); n > 0 && b.cur.runs[n-1].brk && b.cur.runs[n-1].track == nil {
			b.cur.runs = b.cur.runs[:n-1]
		}
		b.blocks = append(b.blocks, xblock{para: b.cur})
		b.cur = nil
	}
}

// para garantiza un párrafo abierto (uno plano si no hay ninguno).
func (b *rtBuilder) para() *xpara {
	if b.cur == nil {
		b.cur = &xpara{marco: b.marco}
	}
	return b.cur
}

// open cierra el párrafo actual y abre uno nuevo con el estilo dado.
func (b *rtBuilder) open(p xpara) {
	b.flush()
	if p.marco == "" {
		p.marco = b.marco
	}
	b.cur = &p
}

var reWS = regexp.MustCompile(`[ \t\r\n\f]+`)

func (b *rtBuilder) addText(s string, f xrun) {
	if s == "" {
		return
	}
	if f.pre {
		f.text = s
		b.para().runs = append(b.para().runs, f)
		return
	}
	collapsed := reWS.ReplaceAllString(s, " ")
	if collapsed == " " {
		// Solo espacios ASCII: preservar UN espacio si ya hay contenido o si es un tramo en
		// línea del documento al exportar a Office (no un salto de indentación entre etiquetas).
		if b.cur != nil && (len(b.cur.runs) > 0 || b.office && !strings.ContainsAny(s, "\r\n")) {
			f.text = " "
			b.cur.runs = append(b.cur.runs, f)
		}
		return
	}
	f.text = collapsed
	p := b.para()
	p.runs = append(p.runs, f)
}

func (b *rtBuilder) walk(n *html.Node, f xrun) {
	if b.noteBodies[n] {
		return // su contenido viaja con la llamada; no duplicarlo en el cuerpo
	}
	switch n.Type {
	case html.TextNode:
		b.addText(n.Data, f)
	case html.ElementNode:
		b.element(n, f)
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			b.walk(c, f)
		}
	}
}

func (b *rtBuilder) children(n *html.Node, f xrun) {
	if b.depth >= maxWalkDepth {
		return
	}
	b.depth++
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.walk(c, f)
	}
	b.depth--
}

//nolint:gocyclo // dispatch de elementos HTML por etiqueta: ramificación inherente al mapeo
func (b *rtBuilder) element(n *html.Node, f xrun) {
	if l := strings.TrimSpace(attr(n, "lang")); l != "" {
		f.lang = l
	}
	f.tam = tamanoCSS(n, f.tam)
	switch n.DataAtom {
	case atom.B, atom.Strong:
		f.b = true
		b.children(n, f)
	case atom.I, atom.Em:
		f.i = true
		b.children(n, f)
	case atom.U, atom.Ins:
		if t := b.trackOf(n); t != nil {
			f.track = t // Word y LibreOffice ya pintan la inserción: no se subraya además
		} else {
			f.u = true
		}
		b.children(n, f)
	case atom.S, atom.Strike, atom.Del:
		if t := b.trackOf(n); t != nil {
			f.track = t
		} else {
			f.strike = true
		}
		b.children(n, f)
	case atom.A:
		b.enlace(n, f)
	case atom.Span, atom.Font:
		b.tramo(n, f)
	case atom.Mark:
		b.children(n, b.markOf(n, f))
	case atom.Br:
		b.para().runs = append(b.para().runs, xrun{brk: true, track: f.track})
	case atom.Sup, atom.Sub:
		b.indice(n, f)
	case atom.Img:
		width := atoiDefault(attr(n, "data-w"), atoiDefault(attr(n, "width"), 0))
		height := atoiDefault(attr(n, "data-h"), atoiDefault(attr(n, "height"), 0))
		f.image = &ximage{src: attr(n, "src"), width: width, height: height, alt: attr(n, "alt")}
		b.para().runs = append(b.para().runs, f)
	case atom.P, atom.Div, atom.Section, atom.Article:
		if b.office && n.DataAtom == atom.Div && b.region(n, f) {
			break
		}
		b.open(xpara{align: styleAlign(n), css: b.cssBloque(n)})
		b.children(n, b.heredado(n, f))
		b.flush()
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		lvl := int(n.Data[1] - '0')
		b.open(xpara{style: "Heading" + strconv.Itoa(lvl), align: styleAlign(n), css: b.cssBloque(n)})
		b.children(n, b.heredado(n, f))
		b.flush()
	case atom.Blockquote:
		b.open(xpara{style: "Quote", align: styleAlign(n), css: b.cssBloque(n)})
		b.children(n, b.heredado(n, f))
		b.flush()
	case atom.Ul, atom.Ol:
		b.lista(n, f)
	case atom.Li:
		b.elementoLista(n, f)
	case atom.Hr:
		if b.office && n.Parent != nil && hasAttr(n.Parent, "data-footnotes") {
			break // la raya de la sección de notas es del editor: las notas viajan con su llamada
		}
		b.flush()
		b.blocks = append(b.blocks, xblock{hr: true})
	case atom.Table:
		b.flush()
		if t := parseTable(n, b.depth, b); t != nil {
			b.blocks = append(b.blocks, xblock{table: t})
		}
	case atom.Figure:
		if hasAttr(n, "data-dibujo") {
			b.dibujo(n, f)
			break
		}
		b.children(n, f)
	case atom.Head, atom.Script, atom.Style:
		// Metadatos y contenido ejecutable nunca forman parte del documento.
	default:
		b.children(n, f)
	}
}

// enlace recorre un <a>. En una exportación a OOXML no hereda el color del párrafo: el editor lo
// pinta con el suyo, y en Word va con el azul de los enlaces salvo que su texto traiga otro.
func (b *rtBuilder) enlace(n *html.Node, f xrun) {
	if href := attr(n, "href"); href != "" {
		f.link = href
		if b.office {
			f.color = ""
		}
	}
	b.children(n, f)
}

// tramo recorre un <span> o un <font>: su color y, en una exportación a OOXML, el resto de su
// formato. El campo de página es el suyo.
func (b *rtBuilder) tramo(n *html.Node, f xrun) {
	if b.office && hasAttr(n, "data-page-field") {
		b.para().runs = append(b.para().runs, pageFieldRuns(nodeText(n), f)...)
		return
	}
	if c := styleColor(n); c != "" {
		f.color = c
	}
	if b.office {
		f = f.conCSS(attr(n, "style"), false)
	}
	b.children(n, f)
}

// indice recorre un <sup> o un <sub>: la llamada a una nota, o un superíndice o subíndice, que
// la exportación a OOXML marca como tal.
func (b *rtBuilder) indice(n *html.Node, f xrun) {
	if n.DataAtom == atom.Sup {
		if note := b.notes[attr(n, "data-footnote-ref")]; note != nil {
			b.para().runs = append(b.para().runs, xrun{note: note})
			return
		}
	}
	if b.office {
		f.vert = map[atom.Atom]string{atom.Sup: "sup", atom.Sub: "sub"}[n.DataAtom]
	}
	b.children(n, f)
}

// lista recorre un <ul> o un <ol>: su tipo y su viñeta propia van a la pila para sus elementos.
func (b *rtBuilder) lista(n *html.Node, f xrun) {
	vineta := ""
	if d := len(b.vinetas); d > 0 {
		vineta = b.vinetas[d-1]
	}
	if v, ok := vinetaDeCSS(attr(n, "style")); ok && b.office {
		vineta = v
	}
	b.lists = append(b.lists, n.DataAtom == atom.Ol)
	b.vinetas = append(b.vinetas, vineta)
	b.children(n, f)
	b.lists = b.lists[:len(b.lists)-1]
	b.vinetas = b.vinetas[:len(b.vinetas)-1]
	b.flush()
}

// elementoLista abre el párrafo de un <li> con el tipo, el nivel y la viñeta de su lista.
func (b *rtBuilder) elementoLista(n *html.Node, f xrun) {
	kind, level, vineta := "bullet", 0, ""
	if d := len(b.lists); d > 0 {
		if b.lists[d-1] {
			kind = "number"
		} else {
			vineta = b.vinetas[d-1]
		}
		level = d - 1
	}
	b.open(xpara{list: kind, level: level, align: styleAlign(n), css: b.cssBloque(n), vineta: vineta})
	b.children(n, b.heredado(n, f))
	b.flush()
}

// cssBloque es el `style` de un bloque, que la exportación a OOXML lee para su pPr (pPrDocx). En
// las demás (PDF, texto) no se usa: queda vacío.
func (b *rtBuilder) cssBloque(n *html.Node) string {
	if !b.office {
		return ""
	}
	return attr(n, "style")
}

// heredado es el formato que un bloque da a su texto: lo heredable de su `style` (tipografía,
// tamaño, color, negrita…), como en CSS. Solo en la exportación a OOXML.
func (b *rtBuilder) heredado(n *html.Node, f xrun) xrun {
	if !b.office {
		return f
	}
	return f.conCSS(attr(n, "style"), true)
}

// dibujo vuelca un bloque «Dibujo» de Docus (<figure data-dibujo>): su imagen de respaldo PNG
// (<img data-dibujo-respaldo>, la pinta el navegador al guardar) en un párrafo propio y centrado.
// Ni el <svg> vecino ni el JSON de la escena se recorren: los rótulos del dibujo saldrían como
// texto suelto en el párrafo. Sin respaldo (el navegador no pudo rasterizar) o en los formatos
// de solo texto (PDF, texto plano) queda un marcador con el texto alternativo, que es lo único
// que un lector sin imagen puede conservar.
func (b *rtBuilder) dibujo(n *html.Node, f xrun) {
	img, alt := dibujoPartes(n)
	b.open(xpara{align: "center"})
	if img != nil && (b.office || b.imagenes) {
		if attr(img, "alt") == "" && alt != "" {
			img.Attr = append(img.Attr, html.Attribute{Key: "alt", Val: alt})
		}
		b.element(img, f)
		if runs := b.para().runs; b.office && len(runs) > 0 && runs[len(runs)-1].image != nil {
			if svg := buscarSVG(n); svg != nil {
				runs[len(runs)-1].image.svg = svgDeDibujo(svg, attr(n, "data-escena"))
			}
		}
	} else {
		marca := "[dibujo]"
		if alt != "" {
			marca = "[dibujo: " + alt + "]"
		}
		f.i = true
		f.text = marca
		b.para().runs = append(b.para().runs, f)
	}
	b.flush()
}

// dibujoPartes busca en un <figure data-dibujo> su imagen de respaldo y su texto alternativo:
// el del propio <img> y, si no lo tiene, la descripción o el título del <svg> vecino.
func dibujoPartes(n *html.Node) (img *html.Node, alt string) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type != html.ElementNode:
		case c.DataAtom == atom.Img && hasAttr(c, "data-dibujo-respaldo"):
			if img == nil {
				img = c
			}
		case c.Data == "svg" && alt == "":
			alt = svgTextoAlternativo(c)
		}
	}
	if img != nil && attr(img, "alt") != "" {
		alt = attr(img, "alt")
	}
	return img, alt
}

// svgTextoAlternativo es el <desc> de un <svg> o, sin él, su <title>.
func svgTextoAlternativo(svg *html.Node) string {
	titulo := ""
	for d := svg.FirstChild; d != nil; d = d.NextSibling {
		if d.Type != html.ElementNode {
			continue
		}
		switch d.Data {
		case "desc":
			if txt := strings.TrimSpace(nodeText(d)); txt != "" {
				return txt
			}
		case "title":
			if titulo == "" {
				titulo = strings.TrimSpace(nodeText(d))
			}
		}
	}
	return titulo
}

// parseTable extrae una tabla rectangular. Cada celda se reduce a sus párrafos.
// `depth` propaga la profundidad de recursión (tablas anidadas cuentan hacia el
// tope maxWalkDepth para no desbordar la pila con entrada no confiable).
func parseTable(n *html.Node, depth int, p *rtBuilder) *xtable {
	if depth >= maxWalkDepth {
		return nil
	}
	t := &xtable{}
	var walkRows func(node *html.Node)
	walkRows = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.DataAtom {
			case atom.Thead, atom.Tbody, atom.Tfoot:
				walkRows(c)
			case atom.Tr:
				if row := parseTableRow(c, depth, p); len(row) > 0 {
					t.rows = append(t.rows, row)
					if len(row) > t.cols {
						t.cols = len(row)
					}
				}
			default:
				// caption/colgroup/col y demás no aportan celdas.
			}
		}
	}
	walkRows(n)
	if len(t.rows) == 0 {
		return nil
	}
	t.css = p.cssBloque(n)
	t.normalize()
	return t
}

// parseTableRow extrae las celdas (<td>/<th>) de una fila <tr>.
func parseTableRow(tr *html.Node, depth int, p *rtBuilder) []xcell {
	var row []xcell
	for cell := tr.FirstChild; cell != nil; cell = cell.NextSibling {
		if cell.Type != html.ElementNode {
			continue
		}
		if cell.DataAtom == atom.Td || cell.DataAtom == atom.Th {
			xc := parseTableCell(cell, depth, p)
			xc.header = cell.DataAtom == atom.Th
			xc.css = p.cssBloque(cell)
			xc.colspan = atoiDefault(attr(cell, "colspan"), 1)
			xc.rowspan = atoiDefault(attr(cell, "rowspan"), 1)
			row = append(row, xc)
		}
	}
	return row
}

// parseTableCell reduce una celda de tabla a sus párrafos (al menos uno).
// La celda hereda del cuerpo lo que cambia el mapeo (notas, exportación a OOXML/ODF, anclas de
// comentario): una revisión o un comentario dentro de una tabla es igual de real.
func parseTableCell(cell *html.Node, depth int, p *rtBuilder) xcell {
	cb := &rtBuilder{depth: depth + 1, notes: p.notes, office: p.office, marks: p.marks, imagenes: p.imagenes, marco: p.marco}
	cb.children(cell, cb.heredado(cell, xrun{}))
	cb.flush()
	xc := xcell{}
	for _, blk := range cb.blocks {
		if blk.para != nil {
			xc.paras = append(xc.paras, *blk.para)
		}
	}
	if len(xc.paras) == 0 {
		xc.paras = []xpara{{marco: p.marco}}
	}
	return xc
}

// --- helpers de atributos/estilo HTML ---

func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}

// pageFieldRuns reparte el texto de un campo de página («Página 1 de 5») en literales y
// campos: la primera cifra es PAGE y la segunda NUMPAGES. // ponytail: el texto lo pone la
// vista en su idioma y solo trae dos números; un campo con más cifras las deja literales.
func pageFieldRuns(text string, f xrun) []xrun {
	var out []xrun
	n := 0
	for text != "" {
		i := strings.IndexAny(text, "0123456789")
		if i < 0 {
			i = len(text)
		}
		if i > 0 {
			r := f
			r.text = text[:i]
			out = append(out, r)
			text = text[i:]
			continue
		}
		j := 0
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		r := f
		r.text = text[:j]
		if n < 2 {
			r.field = []string{"PAGE", "NUMPAGES"}[n]
		}
		n++
		out = append(out, r)
		text = text[j:]
	}
	return out
}

// markOf marca los tramos de un <mark data-comment-id> con el id de su comentario (solo en
// una exportación a OOXML/ODF) y apunta el ancla.
func (b *rtBuilder) markOf(n *html.Node, f xrun) xrun {
	if id := attr(n, "data-comment-id"); b.office && id != "" {
		if b.marks == nil {
			b.marks = map[string]bool{}
		}
		b.marks[id] = true
		f.mark = id
	}
	return f
}

// trackOf es la revisión de un <ins>/<del data-track> en una exportación a OOXML/ODF; en
// las demás (PDF) sigue siendo subrayado y tachado.
func (b *rtBuilder) trackOf(n *html.Node) *xtrack {
	if !b.office || !hasAttr(n, "data-track") {
		return nil
	}
	ts, _ := strconv.ParseInt(attr(n, "data-ts"), 10, 64)
	return &xtrack{kind: n.Data, author: attr(n, "data-author"), ts: ts}
}

// region saca del cuerpo el encabezado o el pie (div[data-doc-header|footer]) y los deja en
// b.header y b.footer como UN párrafo en línea; las líneas de más se unen con saltos.
func (b *rtBuilder) region(n *html.Node, f xrun) bool {
	var dst **xpara
	switch {
	case hasAttr(n, "data-doc-header"):
		dst = &b.header
		b.firstDiff = hasAttr(n, "data-first-page-different")
	case hasAttr(n, "data-doc-footer"):
		dst = &b.footer
	case hasAttr(n, "data-sytue-page"):
		b.pagina = leerPagina(n)
		return true
	case hasAttr(n, "data-marco"):
		// Un marco de texto: sus párrafos llevan su framePr (framePrDocx); el div no es un párrafo.
		b.flush()
		fuera := b.marco
		b.marco = attr(n, "style")
		b.children(n, f)
		b.flush()
		b.marco = fuera
		return true
	default:
		return false
	}
	b.flush()
	sub := &rtBuilder{office: true, depth: b.depth}
	sub.children(n, f)
	sub.flush()
	p := &xpara{}
	for _, blk := range sub.blocks {
		if blk.para == nil {
			continue
		}
		if len(p.runs) > 0 {
			p.runs = append(p.runs, xrun{brk: true})
		}
		p.runs = append(p.runs, blk.para.runs...)
	}
	*dst = p
	return true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// tamanoCSS es el tamaño en px que el editor de Diapos pone a un tramo o a su párrafo, que
// heredan sus tramos como en CSS; `heredado` si el elemento no dice uno.
func tamanoCSS(n *html.Node, heredado float64) float64 {
	if m := reCSSFontSizePx.FindStringSubmatch(attr(n, "style")); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 && v < 1000 {
			return v
		}
	}
	return heredado
}

// styleAlign lee text-align del atributo style (o del atributo align).
func styleAlign(n *html.Node) string {
	css := attr(n, "style")
	if m := reTextAlign.FindStringSubmatch(css); m != nil {
		switch strings.ToLower(strings.TrimSpace(m[1])) {
		case "center", "right", "justify", "left":
			return strings.ToLower(strings.TrimSpace(m[1]))
		}
	}
	switch strings.ToLower(attr(n, "align")) {
	case "center", "right", "justify":
		return strings.ToLower(attr(n, "align"))
	}
	return ""
}

var (
	reTextAlign = regexp.MustCompile(`(?i)text-align\s*:\s*([a-z]+)`)
	reCSSColor  = regexp.MustCompile(`(?i)(?:^|;)\s*color\s*:\s*([^;]+)`)
	// reCSSFontSizePx es un font-size en px: el que escribe el editor de Diapos en un tramo.
	reCSSFontSizePx = regexp.MustCompile(`(?i)(?:^|;)\s*font-size\s*:\s*(\d+(?:\.\d+)?)px`)
)

// styleColor lee un color de texto (#hex, rgb() o atributo color de <font>).
func styleColor(n *html.Node) string {
	if m := reCSSColor.FindStringSubmatch(attr(n, "style")); m != nil {
		if c := normalizeColor(strings.TrimSpace(m[1])); c != "" {
			return c
		}
	}
	if c := normalizeColor(attr(n, "color")); c != "" {
		return c
	}
	return ""
}

var reRGB = regexp.MustCompile(`(?i)rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)`)

// normalizeColor convierte "#abc", "#aabbcc" o "rgb(r,g,b)" a "RRGGBB" en mayúsculas.
func normalizeColor(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) == 6 && isHex(hex) {
			return strings.ToUpper(hex)
		}
		return ""
	}
	if m := reRGB.FindStringSubmatch(s); m != nil {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		bl, _ := strconv.Atoi(m[3])
		if r <= 255 && g <= 255 && bl <= 255 {
			return strings.ToUpper(hexByte(r) + hexByte(g) + hexByte(bl))
		}
	}
	return ""
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func hexByte(v int) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[(v>>4)&0xF], digits[v&0xF]})
}

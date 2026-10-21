// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package rico es el MODELO del folio de Docus: lee el HTML que guarda el editor de la
// web, lo describe como bloques con su estilo para que la vista los pinte, y lo edita SIN
// PERDER NADA. No sabe de Fyne: la vista es `docus.Folio`.
//
// Viene del puerto a Gio (`internal/rico`, archivado el 2026-09-22), donde se escribió y
// se probó contra el HTML de la web; lo que era de Gio —su vista— se quedó allí.
//
// En la web «el HTML ES el modelo»: lo que se guarda es el innerHTML de un
// contenteditable. Aquí el modelo es ese mismo árbol —el de golang.org/x/net/html, el
// analizador con el que el servicio lo exporta a docx y PDF—, no una estructura propia a
// la que traducirlo: lo que no se traduce no se puede perder. De ahí salen las dos reglas:
//
//   - Editar toca SOLO el bloque editado. Todo lo demás vuelve al servidor con los
//     mismos nodos con los que llegó, y un documento sin tocar se escribe byte a byte
//     igual (HTML sigue el algoritmo de serialización de los navegadores).
//   - Un documento se puede editar solo si se entiende ENTERO (Editable): cada etiqueta,
//     cada atributo y cada propiedad de estilo están en la lista de lo que este paquete
//     sabe pintar y no puede romper, y además leerlo y volverlo a escribir da el mismo
//     árbol. Si no, se lee —con lo que no se sabe pintar dicho, no escondido— y no se toca.
//     La excepción es un puñado de propiedades de presentación que trae un .docx importado
//     (bordes y anchos de celda, espaciado entre letras…): se dejan sin pintar, ninguna
//     edición las toca y la web las sigue pintando ([Documento.Ignorado]).
package rico

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Documento es un documento de texto enriquecido.
type Documento struct {
	raiz    *html.Node // un <body> de mentira: sus hijos son el documento
	motivos []string   // por qué no se edita; vacío es que se entiende entero
	// ignorados es lo que se deja sin pintar y no impide editar (ver [Documento.Ignorado]).
	ignorados []string
	// rastreo es la [Sentinela] del frente: el control de cambios está encendido. No
	// entra en el árbol, como en la web (`readTrackFlag`), para que no se borre al teclear.
	rastreo bool
	hoja    *Hoja // la del contenedor; nil es la del folio
}

// profundidadMax acota los recorridos recursivos: el contenido llega de fuera y miles de
// <div> anidados tumbarían el proceso. Lo que pase de ahí no se recorre, y se dice.
const profundidadMax = 200

func cuerpo() *html.Node {
	return &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
}

func leer(fuente string) (*html.Node, error) {
	nodos, err := html.ParseFragment(strings.NewReader(fuente), cuerpo())
	if err != nil {
		return nil, err
	}
	raiz := cuerpo()
	for _, n := range nodos {
		raiz.AppendChild(n)
	}
	ordenDeLaFuente(raiz, fuente)
	return raiz, nil
}

// ordenDeLaFuente devuelve a los atributos el orden en que venían escritos. El analizador
// ORDENA alfabéticamente los de los elementos de formato (<a>, <font>, <b>…) para buscar
// duplicados más deprisa; en el DOM el orden no significa nada, pero en los bytes sí, y
// un `<a href target rel>` pegado de una web volvería al servidor como `<a href rel
// target>` sin que nadie lo hubiera tocado.
//
// El tokenizador sí conserva el orden. Se casa cada etiqueta de apertura con su elemento,
// en orden de documento, y solo se aplica si casan TODAS: si el analizador ha puesto o
// movido algo (un <tbody>, contenido SVG con otras mayúsculas), se deja como está.
func ordenDeLaFuente(raiz *html.Node, fuente string) {
	var elementos []*html.Node
	var recorrer func(n *html.Node, prof int)
	recorrer = func(n *html.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.Type == html.ElementNode {
				elementos = append(elementos, h)
				recorrer(h, prof+1)
			}
		}
	}
	recorrer(raiz, 0)
	ordenes := make([][]html.Attribute, 0, len(elementos))
	for z := html.NewTokenizer(strings.NewReader(fuente)); ; {
		tipo := z.Next()
		if tipo == html.ErrorToken {
			break
		}
		if tipo != html.StartTagToken && tipo != html.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		if i := len(ordenes); i >= len(elementos) || elementos[i].Data != tok.Data || !mismosAtributos(elementos[i].Attr, tok.Attr) {
			return
		}
		ordenes = append(ordenes, tok.Attr)
	}
	if len(ordenes) != len(elementos) {
		return
	}
	for i, n := range elementos {
		n.Attr = ordenes[i]
	}
}

func mismosAtributos(a, b []html.Attribute) bool {
	comparar := func(x, y html.Attribute) int {
		return strings.Compare(x.Namespace+"\x00"+x.Key+"\x00"+x.Val, y.Namespace+"\x00"+y.Key+"\x00"+y.Val)
	}
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, comparar)
	slices.SortFunc(b, comparar)
	return slices.Equal(a, b)
}

// ParseCon lee el HTML de un documento que vive en un contenedor con su propia hoja de
// estilo (ver [Hoja]). Con `h` nil es [Parse].
func ParseCon(fuente string, h *Hoja) *Documento {
	d := Parse(fuente)
	d.hoja = h
	return d
}

// Hoja es la hoja con la que se leyó, o nil si es la del folio.
func (d *Documento) Hoja() *Hoja { return d.hoja }

// Parse lee el HTML de un documento. No falla: lo que no se puede leer como HTML se
// enseña como texto y no se edita.
func Parse(fuente string) *Documento {
	fuente, rastreo := strings.CutPrefix(fuente, Sentinela)
	raiz, err := leer(fuente)
	if err != nil {
		raiz = cuerpo()
		raiz.AppendChild(&html.Node{Type: html.TextNode, Data: fuente})
		return &Documento{raiz: raiz, motivos: []string{"html"}, rastreo: rastreo}
	}
	d := &Documento{raiz: raiz, rastreo: rastreo}
	motivos := map[string]bool{}
	entender(raiz, 0, motivos)
	for m := range motivos {
		if ig, ok := strings.CutPrefix(m, ignorado); ok {
			d.ignorados = append(d.ignorados, ig)
		} else {
			d.motivos = append(d.motivos, m)
		}
	}
	slices.Sort(d.motivos)
	slices.Sort(d.ignorados)
	// La prueba de ida y vuelta, hecha con ESTE documento y no con uno de ejemplo: lo que
	// se escribiría tiene que leerse como el mismo árbol. Cubre lo que la lista de arriba
	// no puede prever (un analizador que recoloca nodos, una entidad que cambia).
	if len(d.motivos) == 0 && !d.fiel() {
		d.motivos = []string{"html"}
	}
	return d
}

// fiel dice si escribir el árbol y volverlo a leer da el mismo árbol.
func (d *Documento) fiel() bool {
	otra, err := leer(d.cuerpoHTML())
	return err == nil && igual(d.raiz, otra, 0)
}

// Editable dice si el documento se entiende entero y por tanto se puede editar.
func (d *Documento) Editable() bool { return len(d.motivos) == 0 }

// NoEntendido lista, ordenado, lo que impide editarlo: `<table>`, `p[class]`,
// `style: margin-left`, `<!-- -->`… Son nombres de HTML: quien los enseña los traduce.
func (d *Documento) NoEntendido() []string { return slices.Clone(d.motivos) }

// Ignorado lista, ordenado, lo que el documento lleva y aquí no se pinta pero tampoco impide
// editarlo: el formato de presentación que trae un .docx importado (los bordes, el relleno y el
// ancho de una celda, el espaciado entre letras…). Vive en el atributo `style` de un nodo que
// la edición no reescribe, así que vuelve al servidor tal cual y la web lo sigue pintando.
func (d *Documento) Ignorado() []string { return slices.Clone(d.ignorados) }

// ignorado marca en `motivos` lo que no se pinta y no impide editar.
const ignorado = "~"

// ignorables son las propiedades de estilo que se dejan sin pintar sin dejar el documento en solo
// lectura: son de presentación, van en el `style` del propio elemento y ninguna operación de
// edición de este paquete las reescribe ni depende de ellas. Las que sí cambian el texto que se
// edita (white-space, que conserva los tabuladores) no están: un documento con ellas se lee.
var ignorables = map[string]bool{
	"width": true, "padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true,
	"padding-left": true, "border": true, "border-top": true, "border-right": true, "border-bottom": true,
	"border-left": true, "letter-spacing": true, "font-variant": true, "text-transform": true,
	"vertical-align": true, "display": true, "margin-left": true, "margin-right": true, "float": true,
}

// Guardable dice si lo que hay ahora se puede mandar al servidor: tras editar, lo que se
// escribiría tiene que seguir leyéndose como el mismo árbol. Es el último cerrojo: si una
// edición dejara algo que un navegador recolocaría al abrirlo, no sale de aquí.
func (d *Documento) Guardable() bool { return d.Editable() && d.fiel() }

func igual(a, b *html.Node, prof int) bool {
	if a.Type != b.Type || a.Data != b.Data || a.Namespace != b.Namespace || !mismosAtributos(a.Attr, b.Attr) || prof > profundidadMax {
		return false
	}
	ha, hb := a.FirstChild, b.FirstChild
	for ; ha != nil && hb != nil; ha, hb = ha.NextSibling, hb.NextSibling {
		if !igual(ha, hb, prof+1) {
			return false
		}
	}
	return ha == nil && hb == nil
}

// HTML escribe el documento como lo escribiría `innerHTML`: el algoritmo de serialización
// de fragmentos de HTML (WHATWG). No se usa html.Render porque escapa también comillas y
// apóstrofos del texto (`l&#39;équipe`), y cada guardado desde aquí reescribiría medio
// documento francés sin haber cambiado nada.
func (d *Documento) HTML() string {
	if d.rastreo {
		return Sentinela + d.cuerpoHTML()
	}
	return d.cuerpoHTML()
}

// cuerpoHTML es HTML sin la sentinela: lo que hay en el árbol.
func (d *Documento) cuerpoHTML() string {
	var b strings.Builder
	for n := d.raiz.FirstChild; n != nil; n = n.NextSibling {
		escribir(&b, n)
	}
	return b.String()
}

var (
	escapaTexto    = strings.NewReplacer("&", "&amp;", "\u00a0", "&nbsp;", "<", "&lt;", ">", "&gt;")
	escapaAtributo = strings.NewReplacer("&", "&amp;", "\u00a0", "&nbsp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	// sinCierre son los elementos vacíos: no llevan etiqueta de cierre ni hijos.
	sinCierre = map[string]bool{
		"area": true, "base": true, "basefont": true, "bgsound": true, "br": true, "col": true, "embed": true,
		"frame": true, "hr": true, "img": true, "input": true, "keygen": true, "link": true, "meta": true,
		"param": true, "source": true, "track": true, "wbr": true,
	}
	// textoCrudo son los elementos cuyo texto se escribe sin escapar.
	textoCrudo = map[string]bool{
		"style": true, "script": true, "xmp": true, "iframe": true, "noembed": true, "noframes": true, "plaintext": true,
	}
)

// ponytail: recursivo y sin tope. Guardar solo llama a esto con documentos editables, que
// `entender` ya ha acotado a profundidadMax; con uno más hondo (solo lectura) la pila de
// Go crece lo que haga falta. Ponerle tope si algún día se escribe lo que no se entiende.
func escribir(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if p := n.Parent; p != nil && p.Type == html.ElementNode && p.Namespace == "" && textoCrudo[p.Data] {
			b.WriteString(n.Data)
		} else {
			escapaTexto.WriteString(b, n.Data) //nolint:errcheck // un strings.Builder no falla
		}
	case html.CommentNode:
		b.WriteString("<!--" + n.Data + "-->")
	case html.ElementNode:
		b.WriteString("<" + n.Data)
		for _, a := range n.Attr {
			nombre := a.Key
			if a.Namespace != "" {
				nombre = a.Namespace + ":" + a.Key
			}
			b.WriteString(" " + nombre + `="`)
			escapaAtributo.WriteString(b, a.Val) //nolint:errcheck // ídem
			b.WriteString(`"`)
		}
		b.WriteString(">")
		if n.Namespace == "" && sinCierre[n.Data] {
			return
		}
		for h := n.FirstChild; h != nil; h = h.NextSibling {
			escribir(b, h)
		}
		b.WriteString("</" + n.Data + ">")
	default: // doctype y demás: un fragmento no los lleva
	}
}

// Lo que se entiende. Una etiqueta entra aquí cuando este paquete sabe pintarla Y sus
// operaciones de edición no pueden estropearla; no antes.
var (
	bloquesEntendidos = map[atom.Atom]bool{
		atom.P: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
		atom.Blockquote: true, atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Div: true, atom.Hr: true,
		atom.Table: true, atom.Tbody: true, atom.Thead: true, atom.Tfoot: true, atom.Tr: true, atom.Td: true, atom.Th: true,
		// El índice y las notas al pie de «Referencias»; sin su marca no se entienden.
		atom.Nav: true, atom.Section: true,
	}
	enLineaEntendidos = map[atom.Atom]bool{
		atom.B: true, atom.Strong: true, atom.I: true, atom.Em: true, atom.U: true, atom.S: true, atom.Strike: true,
		atom.A: true, atom.Span: true, atom.Font: true, atom.Br: true, atom.Sup: true, atom.Sub: true, atom.Img: true,
		atom.Mark: true,
		// El control de cambios (`trackchanges.ts`); sin `data-track` no se entienden.
		atom.Ins: true, atom.Del: true,
	}
	atributosEntendidos = map[atom.Atom][]string{
		atom.A: {"href", "target", "rel", "data-xref"}, atom.Font: {"color"},
		atom.Td: {"colspan", "rowspan"}, atom.Th: {"colspan", "rowspan"},
		// El ancla de un comentario (`wrapSelectionForComment`). Un <mark> suelto, sin
		// ella, es el amarillo del navegador, que no se pinta: no se entiende.
		atom.Mark: {"data-comment-id"},
		// El marco de texto de un .docx importado (framePr o un cuadro anclado): sus párrafos se
		// editan en el flujo; su `float`, su ancho y sus márgenes son de presentación.
		atom.Div: {"data-marco"},
		// Lo que escribe «Referencias»: los ids de los títulos que enlaza el índice, sus
		// entradas con su nivel, el marcador de una nota al pie y la nota.
		atom.H1: {"id"}, atom.H2: {"id"}, atom.H3: {"id"}, atom.Nav: {"data-toc"},
		atom.Li: {"data-level", "id", "value", "data-index-term", "data-index-sub"}, atom.Sup: {"data-footnote-ref", "data-endnote-ref", "data-note-class", "id"},
		atom.Section: {"data-footnotes", "data-endnotes", "data-index", "data-bibliography"},
		// Los de `trackAttrs`; el `style` solo con el color del autor (ver `entenderAtributo`).
		atom.Ins: {"data-track", "data-author", "data-author-id", "data-ts"},
		atom.Del: {"data-track", "data-author", "data-author-id", "data-ts"},
		// Los de `imageDom.ts` de la web. El `style` de una imagen se deriva de sus data-*
		// y no se lee (ver `entenderElemento`): el nodo vuelve tal cual, con todo.
		atom.Img: {
			"src", "alt", "title", "width", "height", "contenteditable", "data-sytue-image", "data-wrap",
			"data-align", "data-w", "data-h", "data-crop", "data-x", "data-y",
		},
	}
)

// esSaltoPagina dice si n es el `<div data-page-break>` de la web. Su estilo es fijo
// (`PAGE_BREAK_HTML`) y vuelve tal cual: no se lee.
func esSaltoPagina(n *html.Node) bool {
	return n.DataAtom == atom.Div && tieneAtributo(n, "data-page-break")
}

func dentroDeCelda(n *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if esCelda(p) {
			return true
		}
	}
	return false
}

// entender apunta en `motivos` todo lo que hay bajo n que no se entiende.
func entender(n *html.Node, prof int, motivos map[string]bool) {
	if prof > profundidadMax {
		motivos["anidamiento"] = true
		return
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		switch h.Type {
		case html.TextNode:
		case html.ElementNode:
			if esFigura(h) { // atómica: su SVG vuelve tal cual y no se lee
				entenderFigura(h, motivos)
				continue
			}
			entenderElemento(h, motivos)
			entender(h, prof+1, motivos)
		default: // comentarios (la sentinela del control de cambios no llega aquí), doctype…
			motivos["<!-- -->"] = true
		}
	}
}

// entenderPropio entiende los elementos que la web escribe con su propia forma —los datos
// embebidos, las capas de la página y las columnas— y dice si lo era.
func entenderPropio(n *html.Node, motivos map[string]bool) bool {
	switch {
	case (esFuentes(n) || esOrigen(n)) && n.Parent != nil && n.Parent.DataAtom == atom.Body:
		// las fuentes de la bibliografía y el origen de la combinación: datos
	case capaDecorativa(n):
		entenderCapa(n, motivos)
	case esColumnas(n):
		entenderColumnas(n, motivos)
	default:
		return false
	}
	return true
}

func entenderElemento(n *html.Node, motivos map[string]bool) {
	if entenderPropio(n, motivos) {
		return
	}
	bloque := bloquesEntendidos[n.DataAtom]
	if n.Namespace != "" || !bloque && !enLineaEntendidos[n.DataAtom] {
		motivos["<"+n.Data+">"] = true
		return
	}
	if n.DataAtom == atom.Table && dentroDeCelda(n) {
		motivos["<table> en <td>"] = true
	}
	if regionDe(n) != "" {
		entenderRegion(n, motivos)
	}
	if n.DataAtom == atom.Section && !tieneAtributo(n, "data-footnotes") && !esSeccionFinal(n) && !esSeccionIndice(n) && !esSeccionBib(n) {
		motivos["<section>"] = true // solo las dos secciones de notas
	}
	for atomo, marca := range map[atom.Atom]string{
		atom.Mark: "data-comment-id", atom.Nav: "data-toc", atom.Ins: "data-track", atom.Del: "data-track",
	} {
		if n.DataAtom == atomo && !tieneAtributo(n, marca) {
			motivos["<"+n.Data+">"] = true
		}
	}
	for _, a := range n.Attr {
		entenderAtributo(n, a, bloque, motivos)
	}
}

// atributoFijo dice si a es de los que la web escribe siempre igual y vuelven tal cual sin
// leerse: el `style` de una imagen (se deriva de sus data-*), del salto de página y de una
// región, y las marcas del salto, de las regiones y del número de página.
func atributoFijo(n *html.Node, a html.Attribute) bool {
	if a.Key == "style" && estiloFijo(n, a.Val) {
		return true
	}
	for _, f := range atributosFijos {
		if f.es(n) && slices.Contains(f.claves, a.Key) {
			return true
		}
	}
	return false
}

// atributosFijos son, por cada elemento que la web marca, sus atributos fijos.
var atributosFijos = []struct {
	es     func(*html.Node) bool
	claves []string
}{
	{func(n *html.Node) bool { return n.DataAtom == atom.Img }, []string{"style"}},
	{esSaltoPagina, []string{"style", "data-page-break", "contenteditable"}},
	{func(n *html.Node) bool { return regionDe(n) == Encabezado }, []string{"style", "data-doc-header", "data-first-page-different"}},
	{func(n *html.Node) bool { return regionDe(n) == Pie }, []string{"style", "data-doc-footer"}},
	{esCampoPagina, []string{"data-page-field"}},
	{esVuelta, []string{"data-endnote-back", "contenteditable", "title"}},
	{esEntradaIndice, []string{"data-index-entry", "data-index-sub", "id"}},
	{esLeyenda, []string{"data-caption", "id"}},
	{esCita, []string{"data-citation", "contenteditable"}},
	{esCampoCombinacion, []string{"data-mergefield", "contenteditable"}},
	{esEntradaBib, []string{"data-bib-entry"}},
	{esEtiquetaLeyenda, []string{"data-caption-label"}},
}

// estiloFijo dice si v es el `style` que la web escribe siempre igual en n: el del número
// de página y los de las listas de las notas al final y del índice.
func estiloFijo(n *html.Node, v string) bool {
	switch {
	case esCampoPagina(n):
		return v == EstiloCampoPagina
	case esEntradaBib(n):
		return v == estiloEntradaBib
	case esLeyenda(n):
		return v == estiloLeyenda // se lee (letra, color y alineación); los márgenes, en `emitir`
	case n.DataAtom == atom.Ol && esSeccionFinal(n.Parent):
		return v == estiloListaNotasFinal
	case n.DataAtom == atom.Ul && enIndice(n):
		return v == estiloListaIndice || v == estiloSubIndice
	}
	return false
}

func entenderAtributo(n *html.Node, a html.Attribute, bloque bool, motivos map[string]bool) {
	if a.Namespace == "" && atributoFijo(n, a) {
		return
	}
	switch {
	case a.Namespace == "" && a.Key == "style" && esPista(n):
		if !rePista.MatchString(a.Val) {
			motivos["style: --sytue-track"] = true
		}
	case a.Namespace == "" && a.Key == "style" && n.DataAtom != atom.Br && n.DataAtom != atom.Hr:
		prueba := estiloBase // con tamaño: `1.5em` de nada no es un tamaño
		for _, prop := range prueba.declarar(a.Val, bloque) {
			if ignorables[prop] {
				motivos[ignorado+"style: "+prop] = true
			} else {
				motivos["style: "+prop] = true
			}
		}
	case a.Namespace == "" && a.Key == "color" && n.DataAtom == atom.Font:
		if _, ok := colorCSS(a.Val); !ok {
			motivos["font[color]"] = true
		}
	case a.Namespace != "" || !slices.Contains(atributosEntendidos[n.DataAtom], a.Key):
		motivos[n.Data+"["+a.Key+"]"] = true
	}
}

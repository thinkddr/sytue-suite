// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Marca es un formato de carácter que se puede poner y quitar: los seis botones de la
// fuente que la web manda con `styleWithCSS` apagado, así que escriben etiqueta y no
// `<span style>` (`SEMANTIC_INLINE` de packages/office/src/editor/canvas/exec.ts).
type Marca uint8

// Las seis, en el orden de la cinta.
const (
	Negrita Marca = iota
	Cursiva
	Subrayado
	Tachado
	Superindice
	Subindice
)

// Edicion es UN bloque abierto para editarlo: las runas que ve la persona ('\n' es un
// <br>, ObjetoRuna una imagen) y, por cada una, los elementos que la envuelven. La cadena
// de un objeto acaba en el PROPIO objeto, que vuelve al árbol tal cual. Cada cambio se
// escribe en el árbol en el acto, así que Documento.HTML dice siempre lo que hay en pantalla.
type Edicion struct {
	runas   []rune
	cadenas [][]*html.Node
	base    Estilo
	Lugar
	// pendiente es el formato pedido con el cursor quieto («negrita» y a escribir): lo
	// lleva lo próximo que se teclee en pendienteEn.
	pendiente    []*html.Node
	pendienteEn  int
	hayPendiente bool
}

// Editar abre un bloque de texto. Una Regla no se abre.
func (b Bloque) Editar() *Edicion {
	e := &Edicion{base: b.Base, Lugar: b.Lugar}
	for _, t := range b.Tramos {
		if t.Salto {
			e.runas, e.cadenas = append(e.runas, '\n'), append(e.cadenas, t.cadena)
		}
		if t.Objeto != "" {
			e.runas, e.cadenas = append(e.runas, ObjetoRuna), append(e.cadenas, append(slices.Clone(t.cadena), t.nodo))
		}
		for _, r := range t.Texto {
			e.runas, e.cadenas = append(e.runas, r), append(e.cadenas, t.cadena)
		}
	}
	return e
}

// Texto es lo que se pone en el campo de edición.
func (e *Edicion) Texto() string { return string(e.runas) }

// Largo es el número de runas.
func (e *Edicion) Largo() int { return len(e.runas) }

// hereda es la cadena que llevaría lo que se escriba en p: lo pendiente si lo hay, y si no
// lo de la izquierda (o, al principio del bloque, lo de la derecha).
func (e *Edicion) hereda(p int) []*html.Node {
	switch {
	case e.hayPendiente && e.pendienteEn == p:
		return e.pendiente
	case p > 0 && p <= len(e.cadenas):
		return e.sinObjeto(p - 1)
	case len(e.cadenas) > 0:
		return e.sinObjeto(0)
	}
	return nil
}

// objeto es el objeto que ocupa la runa i —lo último de su cadena—, o nil si es texto. Se
// sabe por la RUNA y no por el nodo: el marcador vacío del índice es un `<span>` como el
// que envuelve un término marcado, y solo la runa dice cuál es cuál.
func (e *Edicion) objeto(i int) *html.Node {
	if c := e.cadenas[i]; e.runas[i] == ObjetoRuna && len(c) > 0 {
		return c[len(c)-1]
	}
	return nil
}

// sinObjeto es la cadena de lo que envuelve a la runa i sin el objeto: lo que hereda el
// texto de al lado, que no puede ir DENTRO de una imagen.
func (e *Edicion) sinObjeto(i int) []*html.Node {
	if e.objeto(i) != nil {
		return slices.Clip(e.cadenas[i][:len(e.cadenas[i])-1])
	}
	return e.cadenas[i]
}

func (est Estilo) lleva(m Marca) bool {
	switch m {
	case Cursiva:
		return est.Cursiva
	case Subrayado:
		return est.Subrayado
	case Tachado:
		return est.Tachado
	case Superindice:
		return est.Vertical > 0
	case Subindice:
		return est.Vertical < 0
	case Negrita:
	}
	return est.Negrita()
}

func estiloDe(base Estilo, cadena []*html.Node) Estilo {
	for _, n := range cadena {
		base = aplicar(base, n)
	}
	return base
}

// limpiar quita lo que un analizador de HTML cambiaría por otra cosa al volver a leer.
func limpiar(texto string) []rune {
	return []rune(strings.Map(func(r rune) rune {
		if r == '\r' || r == 0 || r == ObjetoRuna {
			return -1 // un objeto no se escribe: se inserta
		}
		return r
	}, texto))
}

// Cambiar pone el texto nuevo entero y deduce qué ha cambiado: lo que queda entre el
// principio y el final que no han cambiado. Es para quien solo sabe «ha cambiado»; el
// folio sabe dónde está el cursor y usa [Edicion.Sustituir].
// ponytail: al escribir una letra igual a su vecina justo en una frontera de formato,
// puede heredar el del otro lado. El texto es siempre el correcto.
func (e *Edicion) Cambiar(texto string) {
	nuevo := limpiar(texto)
	p := 0
	for p < len(nuevo) && p < len(e.runas) && nuevo[p] == e.runas[p] {
		p++
	}
	s := 0
	for s < len(nuevo)-p && s < len(e.runas)-p && nuevo[len(nuevo)-1-s] == e.runas[len(e.runas)-1-s] {
		s++
	}
	e.sustituir(p, len(e.runas)-s, nuevo[p:len(nuevo)-s])
}

// Sustituir cambia las runas [desde, hasta) por `texto`. Lo escrito hereda el formato
// como en cualquier editor: si SUSTITUYE a algo (escribir sobre una selección), el de lo
// que sustituye; si solo se añade, el de lo que tiene a su izquierda —o lo pendiente, si
// se pidió una marca con el cursor quieto—.
func (e *Edicion) Sustituir(desde, hasta int, texto string) {
	desde = max(0, min(desde, len(e.runas)))
	hasta = max(desde, min(hasta, len(e.runas)))
	e.sustituir(desde, hasta, limpiar(texto))
}

func (e *Edicion) sustituir(desde, hasta int, nuevo []rune) {
	hereda := e.hereda(desde)
	if desde < hasta && (!e.hayPendiente || e.pendienteEn != desde) {
		hereda = e.sinObjeto(desde)
	}
	cadenas := slices.Clone(e.cadenas[:desde])
	for range nuevo {
		cadenas = append(cadenas, hereda)
	}
	e.cadenas = append(cadenas, e.cadenas[hasta:]...)
	e.runas = slices.Concat(e.runas[:desde], nuevo, e.runas[hasta:])
	e.hayPendiente = false
	e.reescribir()
}

// Tiene dice si TODO el tramo [desde, hasta) lleva la marca; con el tramo vacío, si la
// lleva lo que se escribiría ahí.
func (e *Edicion) Tiene(desde, hasta int, m Marca) bool {
	if desde >= hasta {
		return estiloDe(e.base, e.hereda(desde)).lleva(m)
	}
	for i := desde; i < hasta && i < len(e.runas); i++ {
		if e.runas[i] != '\n' && !estiloDe(e.base, e.cadenas[i]).lleva(m) {
			return false
		}
	}
	return true
}

// Alternar pone la marca en [desde, hasta), o la quita si ya la lleva todo el tramo. Con
// el tramo vacío la deja pendiente para lo próximo que se escriba ahí.
func (e *Edicion) Alternar(desde, hasta int, m Marca) {
	e.Marcar(desde, hasta, m, !e.Tiene(max(desde, 0), min(hasta, len(e.runas)), m))
}

// Marcar pone (o quita) la marca en [desde, hasta) sin preguntar: es lo que necesita una
// selección que cruza varios bloques, donde la decisión de poner o quitar se toma una vez
// para todos —si la lleva TODO, se quita; si no, se pone—.
func (e *Edicion) Marcar(desde, hasta int, m Marca, poner bool) {
	nuevo := envoltorio(m, poner)
	cambiar := func(c []*html.Node) []*html.Node {
		// Superíndice y subíndice se excluyen, como en un procesador de textos: poner
		// uno quita el otro.
		if contraria, ok := contrarias[m]; ok && poner {
			c = slices.DeleteFunc(slices.Clone(c), func(n *html.Node) bool { return soloMarca(n, contraria) })
		}
		return conMarca(c, e.base, m, poner, nuevo)
	}
	e.aplicar(desde, hasta, cambiar)
}

var contrarias = map[Marca]Marca{Superindice: Subindice, Subindice: Superindice}

// aplicar cambia la cadena de cada runa de [desde, hasta) y reescribe. Con el tramo vacío
// no toca el árbol: deja el cambio pendiente para lo que se escriba ahí.
func (e *Edicion) aplicar(desde, hasta int, cambiar func([]*html.Node) []*html.Node) {
	desde, hasta = max(desde, 0), min(hasta, len(e.runas))
	if desde >= hasta {
		e.pendiente, e.pendienteEn, e.hayPendiente = cambiar(e.hereda(desde)), desde, true
		return
	}
	for i := desde; i < hasta; i++ {
		// El objeto sigue siendo lo último de su cadena: el formato va por fuera de él.
		if obj := e.objeto(i); obj != nil {
			e.cadenas[i] = append(slices.Clip(cambiar(e.sinObjeto(i))), obj)
			continue
		}
		e.cadenas[i] = cambiar(e.cadenas[i])
	}
	e.reescribir()
}

// PonerPropiedad pone `prop: valor` en [desde, hasta) con un `<span style>`, que es lo
// que escribe Chrome con `styleWithCSS` para la fuente, el tamaño, el color y el resalte.
// Antes quita los elementos que SOLO fijaban esa propiedad, para no apilar un span por
// cada vez que se cambia de color. Con `valor` vacío la quita y no pone nada.
func (e *Edicion) PonerPropiedad(desde, hasta int, prop, valor string) {
	var nuevo *html.Node
	if valor != "" {
		nuevo = elemento("span", html.Attribute{Key: "style", Val: prop + ": " + valor + ";"})
	}
	e.aplicar(desde, hasta, func(c []*html.Node) []*html.Node {
		out := slices.DeleteFunc(slices.Clone(c), func(n *html.Node) bool { return soloPropiedad(n, prop) })
		if nuevo != nil {
			out = append(out, nuevo)
		}
		return out
	})
}

// QuitarFormato es el `removeFormat` de la web: lo que se ve como formato de carácter se
// va y los enlaces se quedan (Chrome tampoco los quita).
func (e *Edicion) QuitarFormato(desde, hasta int) {
	e.aplicar(desde, hasta, func(c []*html.Node) []*html.Node {
		return slices.DeleteFunc(slices.Clone(c), func(n *html.Node) bool { return n.DataAtom != atom.A })
	})
}

// Formato es el estilo con el que se ve lo que hay en [desde, hasta) —el de su primera
// runa— o, con el tramo vacío, el de lo que se escribiría ahí. Es lo que la cinta refleja
// en sus selectores (fuente, tamaño).
func (e *Edicion) Formato(desde, hasta int) Estilo {
	if desde < hasta && desde >= 0 && desde < len(e.cadenas) {
		return estiloDe(e.base, e.cadenas[desde])
	}
	return estiloDe(e.base, e.hereda(desde))
}

// PonerBloque pone en el ELEMENTO del bloque una propiedad de estilo de párrafo
// (`text-align`, `line-height`, `margin-left`), o la quita con `valor` vacío. Solo en un
// bloque con elemento propio: una línea suelta entre bloques no tiene dónde llevarla.
func (e *Edicion) PonerBloque(prop, valor string) bool {
	if !e.cubre() || e.EnRegion() || !partible[e.cont.DataAtom] && !esCelda(e.cont) {
		return false
	}
	ponerDeclaracion(e.cont, prop, valor)
	return true
}

// Bloque dice qué elemento es el bloque abierto («p», «h1», «li»…), o "" en una línea
// suelta.
func (e *Edicion) Bloque() string {
	if !e.cubre() {
		return ""
	}
	return e.cont.Data
}

// Lista dice en qué lista está el bloque: «ul», «ol» o "".
func (e *Edicion) Lista() string {
	if e.cubre() && e.cont.DataAtom == atom.Li && deLista(e.cont.Parent) {
		return e.cont.Parent.Data
	}
	return ""
}

func elemento(etiqueta string, attr ...html.Attribute) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: etiqueta, DataAtom: atom.Lookup([]byte(etiqueta)), Attr: attr}
}

// envoltorio es el elemento que pone (o niega) una marca. Negar hace falta cuando la
// marca viene de fuera del tramo —un título es negrita entero— y es lo mismo que escribe
// un navegador: `<span style="font-weight: normal;">`. El subrayado y el tachado NO se
// pueden negar en CSS (la decoración se propaga, no se hereda) y por eso no tienen
// negación: se quitan quitando lo que los pone, o no se quitan. Chrome hace igual.
func envoltorio(m Marca, poner bool) *html.Node {
	if poner {
		return elemento(map[Marca]string{
			Negrita: "b", Cursiva: "i", Subrayado: "u", Tachado: "strike", Superindice: "sup", Subindice: "sub",
		}[m])
	}
	switch m {
	case Negrita:
		return elemento("span", html.Attribute{Key: "style", Val: "font-weight: normal;"})
	case Cursiva:
		return elemento("span", html.Attribute{Key: "style", Val: "font-style: normal;"})
	case Subrayado, Tachado, Superindice, Subindice:
	}
	return nil
}

// etiquetasMarca son las etiquetas SIN atributos que no hacen otra cosa que poner la marca.
var etiquetasMarca = map[Marca][]atom.Atom{
	Negrita: {atom.B, atom.Strong}, Cursiva: {atom.I, atom.Em}, Subrayado: {atom.U},
	Tachado: {atom.S, atom.Strike}, Superindice: {atom.Sup}, Subindice: {atom.Sub},
}

// soloMarca dice si lo ÚNICO que hace el elemento es fijar esa marca: quitarlo no se
// lleva nada más por delante (un <span> con negrita Y color no es solo negrita).
func soloMarca(n *html.Node, m Marca) bool {
	if len(n.Attr) == 0 {
		return slices.Contains(etiquetasMarca[m], n.DataAtom)
	}
	decl, ok := unaDeclaracion(n)
	if !ok {
		return false
	}
	switch m {
	case Negrita:
		return decl[0] == "font-weight"
	case Cursiva:
		return decl[0] == "font-style"
	case Subrayado:
		return (decl[0] == "text-decoration-line" || decl[0] == "text-decoration") && decl[1] == "underline"
	case Tachado:
		return (decl[0] == "text-decoration-line" || decl[0] == "text-decoration") && decl[1] == "line-through"
	case Superindice, Subindice:
	}
	return false
}

// soloPropiedad dice si lo único que hace el elemento es fijar esa propiedad de CSS: un
// `<span style="color: …;">`, o un `<font color>` si la propiedad es el color.
func soloPropiedad(n *html.Node, prop string) bool {
	if n.DataAtom == atom.Font && prop == "color" && len(n.Attr) == 1 && n.Attr[0].Key == "color" {
		return true
	}
	decl, ok := unaDeclaracion(n)
	return ok && decl[0] == prop
}

// unaDeclaracion devuelve la ÚNICA declaración de un `<span style>` sin más atributos:
// propiedad y valor, en minúsculas.
func unaDeclaracion(n *html.Node) ([2]string, bool) {
	if n.DataAtom != atom.Span || len(n.Attr) != 1 || n.Attr[0].Key != "style" || n.Attr[0].Namespace != "" {
		return [2]string{}, false
	}
	decls := declaraciones(n.Attr[0].Val)
	if len(decls) != 1 {
		return [2]string{}, false
	}
	return [2]string{strings.ToLower(decls[0][0]), strings.ToLower(decls[0][1])}, true
}

// declaraciones parte un atributo `style` en sus pares propiedad-valor, en orden y sin
// los vacíos. ponytail: parte por `;` a secas, como `Estilo.declarar`.
func declaraciones(css string) [][2]string {
	var out [][2]string
	for _, d := range strings.Split(css, ";") {
		prop, valor, _ := strings.Cut(d, ":")
		if prop, valor = strings.TrimSpace(prop), strings.TrimSpace(valor); prop != "" {
			out = append(out, [2]string{prop, valor})
		}
	}
	return out
}

// ponerDeclaracion pone, cambia o quita (valor vacío) una propiedad del `style` de n, con
// la forma en que la escribe un navegador: `prop: valor;` separadas por un espacio. Las
// demás declaraciones se quedan donde estaban.
func ponerDeclaracion(n *html.Node, prop, valor string) {
	var decls [][2]string
	i := slices.IndexFunc(n.Attr, func(a html.Attribute) bool { return a.Namespace == "" && a.Key == "style" })
	if i >= 0 {
		decls = declaraciones(n.Attr[i].Val)
	}
	j := slices.IndexFunc(decls, func(d [2]string) bool { return strings.EqualFold(d[0], prop) })
	switch {
	case valor == "" && j >= 0:
		decls = slices.Delete(decls, j, j+1)
	case valor == "":
	case j >= 0:
		decls[j][1] = valor
	default:
		decls = append(decls, [2]string{prop, valor})
	}
	partes := make([]string, len(decls))
	for k, d := range decls {
		partes[k] = d[0] + ": " + d[1] + ";"
	}
	switch {
	case len(partes) == 0 && i >= 0:
		n.Attr = slices.Delete(n.Attr, i, i+1)
	case len(partes) == 0:
	case i >= 0:
		n.Attr[i].Val = strings.Join(partes, " ")
	default:
		n.Attr = append(n.Attr, html.Attribute{Key: "style", Val: strings.Join(partes, " ")})
	}
}

// conMarca devuelve la cadena con la marca puesta o quitada. Primero quita los elementos
// que solo están para lo contrario; si no basta, añade `nuevo` por dentro. No escribe en
// `cadena`: las runas vecinas comparten ese array.
func conMarca(cadena []*html.Node, base Estilo, m Marca, poner bool, nuevo *html.Node) []*html.Node {
	lleva := func(c []*html.Node) bool { return estiloDe(base, c).lleva(m) }
	if lleva(cadena) == poner {
		return cadena
	}
	out := slices.DeleteFunc(slices.Clone(cadena), func(n *html.Node) bool { return soloMarca(n, m) })
	if lleva(out) == poner || nuevo == nil {
		return out
	}
	return append(out, nuevo)
}

// cubre dice si el bloque es TODO lo que hay en su contenedor: un <p> y su texto, no una
// línea suelta entre otros bloques.
func (l Lugar) cubre() bool {
	return l.cont != nil && l.cont.DataAtom != atom.Body && l.cont.FirstChild == l.primero && l.cont.LastChild == l.ultimo
}

// reescribir sustituye en el árbol los nodos del bloque por los de runas y cadenas.
func (e *Edicion) reescribir() {
	var despues *html.Node
	if e.primero != nil {
		despues = e.ultimo.NextSibling
		for n := e.primero; n != despues; {
			sig := n.NextSibling
			e.cont.RemoveChild(n)
			n = sig
		}
	}
	nuevos := construir(e.runas, e.cadenas)
	if len(nuevos) == 0 && e.cont.FirstChild == nil && e.cont.DataAtom != atom.Body {
		nuevos = []*html.Node{elemento("br")} // `<p><br></p>`: el párrafo vacío de la web, que así conserva su alto
	}
	e.primero, e.ultimo = nil, nil
	for _, n := range nuevos {
		e.cont.InsertBefore(n, despues)
	}
	if len(nuevos) > 0 {
		e.primero, e.ultimo = nuevos[0], nuevos[len(nuevos)-1]
	}
}

// construir levanta los nodos en línea de un texto: abre cada elemento de la cadena una
// vez por racha de runas que lo comparten, en el mismo orden en que estaba.
func construir(runas []rune, cadenas [][]*html.Node) []*html.Node {
	runas = duros(runas)
	var raices, abiertos, clones []*html.Node
	var texto strings.Builder
	colgar := func(n *html.Node) {
		if len(clones) == 0 {
			raices = append(raices, n)
			return
		}
		clones[len(clones)-1].AppendChild(n)
	}
	soltar := func() {
		if texto.Len() > 0 {
			colgar(&html.Node{Type: html.TextNode, Data: texto.String()})
			texto.Reset()
		}
	}
	for i, r := range runas {
		cadena := cadenas[i]
		var obj *html.Node
		if r == ObjetoRuna && len(cadena) > 0 { // un objeto es lo último de su cadena
			obj, cadena = cadena[len(cadena)-1], cadena[:len(cadena)-1]
		}
		k := 0
		for k < len(abiertos) && k < len(cadena) && abiertos[k] == cadena[k] {
			k++
		}
		if k != len(abiertos) || k != len(cadena) {
			soltar()
			abiertos, clones = abiertos[:k], clones[:k]
			for _, n := range cadena[k:] {
				c := elemento(n.Data, slices.Clone(n.Attr)...)
				colgar(c)
				abiertos, clones = append(abiertos, n), append(clones, c)
			}
		}
		switch {
		case obj != nil:
			soltar()
			colgar(clonar(obj))
			continue
		case r == '\n':
			soltar()
			colgar(elemento("br"))
			continue
		}
		texto.WriteRune(r)
	}
	soltar()
	return raices
}

// clonar copia un nodo con todo lo que lleva dentro.
func clonar(n *html.Node) *html.Node {
	c := &html.Node{Type: n.Type, Data: n.Data, DataAtom: n.DataAtom, Namespace: n.Namespace, Attr: slices.Clone(n.Attr)}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		c.AppendChild(clonar(h))
	}
	return c
}

// duros cambia por espacio duro los espacios que un navegador no pintaría: al borde de la
// línea y el segundo de dos seguidos. Es lo que hace un contenteditable al teclearlos.
func duros(runas []rune) []rune {
	out := slices.Clone(runas)
	for i, r := range out {
		if r != ' ' {
			continue
		}
		if i == 0 || i == len(out)-1 || out[i-1] == '\n' || out[i+1] == '\n' || out[i-1] == ' ' {
			out[i] = '\u00a0'
		}
	}
	return out
}

// Partir es Intro en la posición pos: el bloque se queda con lo de antes y nace otro
// detrás con lo de después. Devuelve el lugar del bloque nuevo para llevar allí el cursor;
// en una línea suelta (sin elemento propio que repetir) Intro es un salto de línea y
// devuelve false.
func (e *Edicion) Partir(pos int) (Lugar, bool) {
	pos = max(0, min(pos, len(e.runas)))
	if e.EnRegion() {
		return e.Lugar, false // la región es una línea: Intro no hace nada
	}
	e.celdaAParrafo()
	if !e.cubre() || !partible[e.cont.DataAtom] {
		texto := string(e.runas[:pos]) + "\n" + string(e.runas[pos:])
		e.hayPendiente = false
		e.Cambiar(texto)
		return e.Lugar, false
	}
	viejo := e.cont
	// Intro en un elemento de lista vacío sale de la lista, como en cualquier editor.
	if viejo.DataAtom == atom.Li && len(e.runas) == 0 && viejo.NextSibling == nil && deLista(viejo.Parent) {
		return sacarDeLista(viejo), true
	}
	// El de detrás no hereda lo que identifica al de delante: el ancla de un título ni el
	// número de una nota al pie.
	nuevo := elemento(viejo.Data, slices.DeleteFunc(slices.Clone(viejo.Attr), func(a html.Attribute) bool {
		return a.Namespace == "" && (a.Key == "id" || a.Key == "value")
	})...)
	if e.Tipo() == Titulo && pos == len(e.runas) {
		nuevo = elemento("p") // tras un título se sigue en texto corriente
	}
	resto := &Edicion{runas: slices.Clone(e.runas[pos:]), cadenas: slices.Clone(e.cadenas[pos:]), base: e.base, Lugar: Lugar{cont: nuevo}}
	e.runas, e.cadenas = e.runas[:pos], e.cadenas[:pos]
	e.reescribir()
	viejo.Parent.InsertBefore(nuevo, viejo.NextSibling)
	resto.reescribir()
	return resto.Lugar, true
}

// celdaAParrafo mete en un <p> el texto suelto de una celda antes de una operación de
// párrafo (Intro, hacerlo título o lista): la web edita con `defaultParagraphSeparator`
// en «p», y el navegador parte en párrafos el texto de la celda.
// ponytail: la forma exacta que deja Chromium con Intro en una celda no se ha medido; esta
// es válida, no pierde nada y la web la lee igual.
func (e *Edicion) celdaAParrafo() {
	if !e.cubre() || !esCelda(e.cont) {
		return
	}
	p := elemento("p")
	for h := e.cont.FirstChild; h != nil; h = e.cont.FirstChild {
		e.cont.RemoveChild(h)
		p.AppendChild(h)
	}
	if p.FirstChild == nil {
		p.AppendChild(elemento("br"))
	}
	e.cont.AppendChild(p)
	e.cont, e.primero, e.ultimo, e.ordinal = p, p.FirstChild, p.LastChild, 0
}

// Tipo dice si el bloque abierto es un título.
func (e *Edicion) Tipo() Tipo {
	switch e.cont.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return Titulo
	case atom.Blockquote:
		return Cita
	default:
		return Parrafo
	}
}

// partible son los contenedores que Intro repite.
var partible = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Li: true, atom.Blockquote: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
}

func deLista(n *html.Node) bool {
	return n != nil && (n.DataAtom == atom.Ul || n.DataAtom == atom.Ol)
}

// sacarDeLista convierte un <li> en un <p> FUERA de su lista, partiéndola en dos si estaba
// en medio. Solo con elementos que son una sola línea (sin listas dentro).
func sacarDeLista(li *html.Node) Lugar {
	lista := li.Parent
	p := elemento("p")
	for h := li.FirstChild; h != nil; h = li.FirstChild {
		li.RemoveChild(h)
		p.AppendChild(h)
	}
	if li.NextSibling != nil && li.PrevSibling != nil { // en medio: lo de detrás, a una lista gemela
		gemela := elemento(lista.Data, slices.Clone(lista.Attr)...)
		for h := li.NextSibling; h != nil; h = li.NextSibling {
			lista.RemoveChild(h)
			gemela.AppendChild(h)
		}
		lista.Parent.InsertBefore(gemela, lista.NextSibling)
	}
	if li.PrevSibling == nil && li.NextSibling != nil {
		lista.Parent.InsertBefore(p, lista)
	} else {
		lista.Parent.InsertBefore(p, lista.NextSibling)
	}
	lista.RemoveChild(li)
	if lista.FirstChild == nil {
		lista.Parent.RemoveChild(lista)
	}
	if p.FirstChild == nil {
		p.AppendChild(elemento("br"))
	}
	return Lugar{cont: p, primero: p.FirstChild, ultimo: p.LastChild}
}

// CambiarTipo convierte el bloque en párrafo («p»), título («h1»…«h3») o cita
// («blockquote»). Solo cambia el nombre del elemento: el texto y su formato no se tocan.
// No aplica a elementos de lista ni a líneas sueltas: devuelve false.
func (e *Edicion) CambiarTipo(etiqueta string) bool {
	if e.EnRegion() {
		return false
	}
	if slices.Contains([]string{"h1", "h2", "h3", "blockquote"}, etiqueta) {
		e.celdaAParrafo()
	}
	if !e.cubre() || !partible[e.cont.DataAtom] || e.cont.DataAtom == atom.Li || !slices.Contains([]string{"p", "h1", "h2", "h3", "blockquote"}, etiqueta) {
		return false
	}
	e.cont.Data, e.cont.DataAtom = etiqueta, atom.Lookup([]byte(etiqueta))
	if etiqueta != "h1" && etiqueta != "h2" && etiqueta != "h3" {
		// El ancla del índice es de los títulos: un párrafo con id no se entendería.
		e.cont.Attr = slices.DeleteFunc(e.cont.Attr, func(a html.Attribute) bool { return a.Namespace == "" && a.Key == "id" })
	}
	return true
}

// AlternarLista mete el bloque en una lista —de viñetas, o numerada— o lo saca de ella.
// En un elemento de una lista de la OTRA clase, cambia la clase de la lista entera.
// ponytail: un navegador cambiaría solo ese elemento, partiendo la lista; aquí se cambia
// entera, que es lo que casi siempre se quiere y no deja listas de un solo elemento.
func (e *Edicion) AlternarLista(ordenada bool) (Lugar, bool) {
	if e.EnRegion() {
		return e.Lugar, false
	}
	e.celdaAParrafo()
	if !e.cubre() || !partible[e.cont.DataAtom] {
		return e.Lugar, false
	}
	etiqueta := map[bool]string{false: "ul", true: "ol"}[ordenada]
	if li := e.cont; li.DataAtom == atom.Li {
		if !deLista(li.Parent) {
			return e.Lugar, false
		}
		if li.Parent.Data != etiqueta {
			li.Parent.Data, li.Parent.DataAtom = etiqueta, atom.Lookup([]byte(etiqueta))
			return e.Lugar, true
		}
		return sacarDeLista(li), true
	}
	viejo := e.cont
	li := elemento("li")
	for h := viejo.FirstChild; h != nil; h = viejo.FirstChild {
		viejo.RemoveChild(h)
		li.AppendChild(h)
	}
	if previa := viejo.PrevSibling; previa != nil && previa.Type == html.ElementNode && previa.Data == etiqueta && previa.Namespace == "" {
		previa.AppendChild(li) // sigue la lista que tiene encima
	} else {
		lista := elemento(etiqueta)
		lista.AppendChild(li)
		viejo.Parent.InsertBefore(lista, viejo)
	}
	viejo.Parent.RemoveChild(viejo)
	return Lugar{cont: li, primero: li.FirstChild, ultimo: li.LastChild}, true
}

// Fundir es Retroceso al principio del bloque i. Un título, una cita o un elemento de
// lista vuelven a ser texto corriente; un párrafo se une al bloque de texto que tiene
// encima (y si lo que tiene encima es una regla, la quita). Devuelve dónde queda el
// cursor: el lugar del bloque y la posición dentro de él.
func (d *Documento) Fundir(bloques []Bloque, i int) (Lugar, int, bool) {
	if i < 0 || i >= len(bloques) || !bloques[i].Tipo.EsTexto() {
		return Lugar{}, 0, false
	}
	e := bloques[i].Editar()
	if e.cubre() {
		switch {
		case e.cont.DataAtom == atom.Li && deLista(e.cont.Parent):
			return sacarDeLista(e.cont), 0, true
		case e.Tipo() != Parrafo:
			e.CambiarTipo("p")
			return e.Lugar, 0, true
		}
	}
	if i == 0 || bloques[i-1].Celda != bloques[i].Celda || bloques[i-1].Region != bloques[i].Region {
		return Lugar{}, 0, false // al principio de una celda no se sale de ella, como en Word; ni de una región
	}
	switch previo := bloques[i-1]; previo.Tipo {
	case Regla, SaltoPagina, Figura:
		previo.cont.RemoveChild(previo.primero)
		return e.Lugar, 0, true
	default:
		arriba := previo.Editar()
		en := len(arriba.runas)
		arriba.runas, arriba.cadenas = append(arriba.runas, e.runas...), append(arriba.cadenas, e.cadenas...)
		e.runas, e.cadenas = nil, nil
		e.quitar()
		arriba.reescribir()
		return arriba.Lugar, en, true
	}
}

// Unir es Suprimir al final del bloque i-1 o borrar una selección que empieza en él: el
// bloque i se une al de arriba SEA DEL TIPO QUE SEA —a diferencia de Fundir, que antes
// convierte en texto corriente el título, la cita o el elemento de lista—. Devuelve dónde
// queda el cursor. No une dos celdas, ni una celda con lo de fuera: con una regla, la quita.
func (d *Documento) Unir(bloques []Bloque, i int) (Lugar, int, bool) {
	if i <= 0 || i >= len(bloques) || bloques[i].Celda != bloques[i-1].Celda || bloques[i].Region != bloques[i-1].Region {
		return Lugar{}, 0, false
	}
	previo := bloques[i-1]
	switch {
	case !bloques[i].Tipo.EsTexto():
		bloques[i].Editar().quitar()
		return previo.Lugar, len(previo.Editar().runas), true
	case !previo.Tipo.EsTexto():
		previo.Editar().quitar()
		return bloques[i].Lugar, 0, true
	}
	e := bloques[i].Editar()
	arriba := previo.Editar()
	en := len(arriba.runas)
	arriba.runas, arriba.cadenas = append(arriba.runas, e.runas...), append(arriba.cadenas, e.cadenas...)
	e.runas, e.cadenas = nil, nil
	e.quitar()
	arriba.reescribir()
	return arriba.Lugar, en, true
}

// Quitar saca el bloque i del documento entero, con los contenedores que se queden vacíos.
func (d *Documento) Quitar(bloques []Bloque, i int) {
	if i >= 0 && i < len(bloques) {
		bloques[i].Editar().quitar()
	}
}

// Asegurar deja el documento con al menos un párrafo donde escribir —el `<p><br></p>` con
// el que la web abre uno vacío— y dice si lo ha tenido que poner. No cuenta como cambio:
// si no se escribe nada, no hay nada que guardar.
func (d *Documento) Asegurar() bool {
	if len(d.Bloques()) > 0 {
		return false
	}
	p := elemento("p")
	p.AppendChild(elemento("br"))
	d.raiz.AppendChild(p)
	return true
}

// quitar saca el bloque del árbol, y con él los contenedores que se queden vacíos.
func (e *Edicion) quitar() {
	if !e.cubre() {
		if e.primero != nil {
			despues := e.ultimo.NextSibling
			for n := e.primero; n != despues; {
				sig := n.NextSibling
				e.cont.RemoveChild(n)
				n = sig
			}
		}
		return
	}
	// Una celda no se quita nunca: se vacía, y queda el `<br>` con el que la web la crea.
	// La tabla solo cambia de forma con las órdenes de tabla.
	for n := e.cont; n != nil && n.DataAtom != atom.Body && n.Parent != nil; {
		if esCelda(n) {
			for h := n.FirstChild; h != nil; h = n.FirstChild {
				n.RemoveChild(h)
			}
			n.AppendChild(elemento("br"))
			break
		}
		padre := n.Parent
		padre.RemoveChild(n)
		if padre.FirstChild != nil {
			break
		}
		n = padre
	}
}

// Anclar envuelve [desde, hasta) en el `<mark data-comment-id>` de un comentario: el
// `wrapSelectionForComment` de la web. Va por dentro del formato, así que un tramo con
// formatos distintos queda en varias marcas con el MISMO id, como las que deja el
// navegador al envolver una selección que cruza elementos.
func (e *Edicion) Anclar(desde, hasta int, id string) {
	marca := elemento("mark", html.Attribute{Key: "data-comment-id", Val: id})
	e.aplicar(desde, hasta, func(c []*html.Node) []*html.Node {
		return append(slices.Clip(c), marca)
	})
}

// Ancla es dónde está en el bloque lo anclado a ese comentario: de la primera a la
// última runa marcadas.
func (b Bloque) Ancla(id string) (desde, hasta int, ok bool) {
	pos := 0
	for _, t := range b.Tramos {
		n := len([]rune(t.Texto))
		if t.Salto || t.Objeto != "" {
			n++
		}
		if slices.ContainsFunc(t.cadena, func(m *html.Node) bool { return esAncla(m, id) }) {
			if !ok {
				desde, ok = pos, true
			}
			hasta = pos + n
		}
		pos += n
	}
	return desde, hasta, ok
}

func esAncla(n *html.Node, id string) bool {
	return n.DataAtom == atom.Mark && atributo(n, "data-comment-id") == id
}

// QuitarAncla es `removeCommentMark`: los `<mark>` de ese comentario se van y su
// contenido se queda donde estaba. Dice si había alguno.
func (d *Documento) QuitarAncla(id string) bool {
	var marcas []*html.Node
	var recorrer func(n *html.Node, prof int)
	recorrer = func(n *html.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if esAncla(h, id) {
				marcas = append(marcas, h)
			}
			recorrer(h, prof+1)
		}
	}
	recorrer(d.raiz, 0)
	for _, m := range marcas {
		padre := m.Parent
		for h := m.FirstChild; h != nil; h = m.FirstChild {
			m.RemoveChild(h)
			padre.InsertBefore(h, m)
		}
		padre.RemoveChild(m)
		fundirTextos(padre)
	}
	return len(marcas) > 0
}

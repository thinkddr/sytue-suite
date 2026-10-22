// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Tipo es la clase de un bloque.
type Tipo uint8

// Parrafo es cualquier bloque de texto que no es título ni cita: un <p>, un <div>, el
// texto de un elemento de lista (que se distingue por Marca y Sangria), una línea suelta,
// el texto de una celda.
const (
	Parrafo Tipo = iota
	Titulo
	Cita
	Regla       // <hr>
	SaltoPagina // el `<div data-page-break>` de «Insertar → Salto de página»
	Figura      // una forma o un gráfico (`figure[data-shape]`, `figure[data-chart]`)
)

// Bloque es una línea del documento: lo que se pinta —y se edita— de una vez. Es una
// VISTA del árbol, no una copia: tras cualquier edición se vuelve a pedir con Bloques.
type Bloque struct {
	Tipo    Tipo
	Nivel   int    // de un Titulo: 1 a 6
	Sangria int    // cuántas listas lo contienen
	Marca   string // «•» o «3.» si es lo primero de un elemento de lista
	// Base es el estilo del bloque: alineación, interlineado y lo que heredan sus tramos.
	Base   Estilo
	Tramos []Tramo
	// Recuadro es el del índice (`nav[data-toc]`) que envuelve al bloque; nil fuera.
	Recuadro *Recuadro
	// Region es [Encabezado] o [Pie] si el bloque es una de las dos regiones, que van
	// fuera de la corriente, en el margen; "" en el cuerpo. Oculta es el encabezado con
	// «Primera página distinta», que en la web no se ve (`display: none`).
	Region string
	Oculta bool
	// Columnas es el envoltorio `div.sytue-cols` en el que va el bloque; nil fuera. Sus
	// bloques van seguidos y comparten el puntero.
	Columnas *Columnas
	// Celda es la celda de tabla en la que está el bloque; nil fuera de una tabla. Los
	// bloques de una tabla van seguidos, celda a celda en orden de documento, y los de una
	// misma celda comparten el puntero: así se agrupan.
	Celda *Celda
	// Ilustracion es la forma o el gráfico de un bloque Figura.
	Ilustracion *Ilustracion
	// Arriba y Abajo son los márgenes, en px de CSS.
	Arriba, Abajo float32
	Lugar
}

// Columnas es un `div.sytue-cols`: cuántas columnas (`column-count`, con `column-gap: 2em`).
type Columnas struct {
	N    int
	nodo *html.Node
}

// Recuadro es el borde discontinuo con el que la web enmarca el índice
// (`.sytue-page nav[data-toc] { border: 1px dashed; padding: 8px 12px }`). Sus bloques
// van seguidos y comparten el puntero.
type Recuadro struct{ nodo *html.Node }

// Tabla es una <table>: el tamaño de su rejilla y sus celdas, en orden de documento.
type Tabla struct {
	Filas, Cols int
	Celdas      []*Celda
	nodo        *html.Node
}

// Celda es un <td> o un <th> con su sitio en la rejilla, con los colspan y rowspan de las
// celdas de antes ya contados (`tableGrid` de packages/office/src/editor/canvas/exec.ts).
type Celda struct {
	Tabla       *Tabla
	Fila, Col   int
	Filas, Cols int // rowspan y colspan, al menos 1
	Cabecera    bool
	Fondo       color.NRGBA // `background-color` de la celda; A == 0, ninguno
	// Vertical es el `vertical-align` de la celda: "top" (el de `editor.css`), "middle"
	// o "bottom".
	Vertical string
	nodo     *html.Node
}

// Lugar es dónde vive el bloque en el árbol: los hijos de `cont` que van de `primero` a
// `ultimo`. Casi siempre son TODOS (un <p> y su texto); no lo son en una línea suelta
// entre bloques (`texto<p>…</p>`) ni en `<li>texto<ul>…</ul></li>`. primero nil: vacío.
type Lugar struct {
	cont            *html.Node
	primero, ultimo *html.Node
	ordinal         int // cuántos bloques de texto hay antes en el mismo contenedor
}

// Tramo es un trozo de texto con un solo estilo.
type Tramo struct {
	Texto  string
	Estilo Estilo
	Salto  bool   // <br>
	Objeto string // algo que no es texto (`img`, `svg`…): ocupa UNA runa, ObjetoRuna
	Imagen *Imagen
	// CampoPagina es el número de página (`span[data-page-field]`): un objeto cuyo texto
	// pone la vista en Rotulo, porque depende de la página que se esté viendo.
	CampoPagina bool
	Rotulo      string
	// Campo es el de una ficha de combinación de correspondencia: la vista previa le pone el
	// valor de un registro en Rotulo.
	Campo string
	nodo  *html.Node // el del objeto
	// cadena son los elementos en línea que envuelven al texto, de fuera adentro. Son los
	// NODOS del documento, no una traducción: al reescribir un bloque editado vuelven tal
	// cual, con atributos que este paquete no necesita entender para no perderlos.
	cadena []*html.Node
}

// ObjetoRuna es la runa que ocupa un objeto (una imagen) en el texto de un bloque: el
// OBJECT REPLACEMENT CHARACTER de Unicode, que es lo que el cursor salta de una vez.
const ObjetoRuna = '\uFFFC'

// Texto es el texto del bloque, con los saltos de línea como '\n' y cada objeto como
// ObjetoRuna: las posiciones son las de [Edicion].
func (b Bloque) Texto() string {
	var s strings.Builder
	for _, t := range b.Tramos {
		if t.Salto {
			s.WriteByte('\n')
		}
		if t.Objeto != "" {
			s.WriteRune(ObjetoRuna)
		}
		s.WriteString(t.Texto)
	}
	return s.String()
}

// Bloques recorre el documento y lo devuelve como líneas, en orden.
func (d *Documento) Bloques() []Bloque {
	c := &caminante{}
	c.contenedor(d.raiz, d.base(), Parrafo, 0, 0)
	return c.bloques
}

// base es el estilo del contenedor: el de su hoja, o el del folio.
func (d *Documento) base() Estilo {
	if d.hoja == nil {
		return estiloBase
	}
	e := d.hoja.Base
	e.hoja = d.hoja
	return e
}

// Texto es el documento entero como texto: para contar palabras o resumirlo.
func (d *Documento) Texto() string {
	bloques := d.Bloques()
	lineas := make([]string, 0, len(bloques))
	for _, b := range bloques {
		lineas = append(lineas, b.Texto())
	}
	return strings.Join(lineas, "\n")
}

var (
	// deBloque son las etiquetas que parten la línea (display: block, list-item, table…).
	deBloque = map[atom.Atom]bool{
		atom.Address: true, atom.Article: true, atom.Aside: true, atom.Blockquote: true, atom.Center: true,
		atom.Dd: true, atom.Details: true, atom.Dialog: true, atom.Div: true, atom.Dl: true, atom.Dt: true,
		atom.Fieldset: true, atom.Figcaption: true, atom.Figure: true, atom.Footer: true, atom.Form: true,
		atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
		atom.Header: true, atom.Hr: true, atom.Li: true, atom.Main: true, atom.Nav: true, atom.Ol: true,
		atom.P: true, atom.Pre: true, atom.Section: true, atom.Summary: true, atom.Table: true, atom.Ul: true,
		atom.Caption: true, atom.Thead: true, atom.Tbody: true, atom.Tfoot: true, atom.Tr: true, atom.Td: true, atom.Th: true,
	}
	// invisibles llevan datos, no texto del documento (el origen de una combinación de
	// correspondencia va en un <script type=json>): no se pintan, y tampoco se entienden.
	invisibles = map[atom.Atom]bool{atom.Script: true, atom.Style: true, atom.Template: true, atom.Head: true, atom.Title: true}
	// objetos son lo que ocupa sitio sin ser texto. No se saben pintar: se nombran.
	objetos = map[atom.Atom]bool{
		atom.Img: true, atom.Svg: true, atom.Math: true, atom.Iframe: true, atom.Video: true, atom.Audio: true,
		atom.Canvas: true, atom.Object: true, atom.Embed: true, atom.Input: true, atom.Select: true,
		atom.Textarea: true, atom.Button: true,
	}
)

type lista struct {
	ordenada bool
	n        int
	// sinMarca es la de las notas al final y las del índice (`list-style:none`): sin
	// número ni viñeta, y sin sangría salvo las subentradas del índice (`sangra`).
	sinMarca, sangra bool
	vineta           string // la de `list-style-type` con una cadena; "" es «•»
}

type caminante struct {
	bloques  []Bloque
	listas   []*lista
	marca    string    // la del <li> en curso, hasta que la gasta su primer bloque
	celda    *Celda    // la celda en curso
	recuadro *Recuadro // el índice en curso
	region   string    // la región en curso
	columnas *Columnas // las columnas en curso
	oculta   bool
}

// enLinea dice si el nodo va DENTRO de una línea. Un elemento en línea que envuelve
// bloques (`<span><p>…`, que un pegado puede dejar) cuenta como contenedor.
func enLinea(n *html.Node, prof int) bool {
	if n.Type != html.ElementNode {
		return true
	}
	if deBloque[n.DataAtom] || prof > profundidadMax {
		return false
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if !enLinea(h, prof+1) {
			return false
		}
	}
	return true
}

// contenedor recorre los hijos de n: los que van en línea, seguidos, forman un bloque de
// texto; los demás son bloques por su cuenta.
func (c *caminante) contenedor(n *html.Node, e Estilo, tipo Tipo, nivel, prof int) {
	if prof > profundidadMax {
		return
	}
	var primero, ultimo *html.Node
	ordinal := 0
	cerrar := func() {
		if primero != nil {
			c.texto(Lugar{n, primero, ultimo, ordinal}, e, tipo, nivel)
			ordinal++
		}
		primero, ultimo = nil, nil
	}
	for h := n.FirstChild; h != nil; h = h.NextSibling {
		if enLinea(h, prof) {
			if primero == nil {
				primero = h
			}
			ultimo = h
			continue
		}
		cerrar()
		c.bloque(h, e, tipo, nivel, prof+1)
	}
	if n.FirstChild == nil && n.DataAtom != atom.Body && !deLista(n) {
		c.emitir(Bloque{Tipo: tipo, Nivel: nivel, Base: e, Lugar: Lugar{cont: n}}) // <p></p>: una línea vacía
	}
	cerrar()
}

// texto emite el bloque de una racha de nodos en línea. Una racha de solo blancos entre
// bloques (el salto de línea entre </p> y <p>) no es una línea del documento.
func (c *caminante) texto(l Lugar, e Estilo, tipo Tipo, nivel int) {
	a := aplanador{base: e}
	for n := l.primero; n != nil; n = n.NextSibling {
		a.nodo(n, e, nil, 0)
		if n == l.ultimo {
			break
		}
	}
	a.cerrar()
	if len(a.tramos) == 0 && !l.cubre() {
		return
	}
	c.emitir(Bloque{Tipo: tipo, Nivel: nivel, Base: e, Tramos: a.tramos, Lugar: l})
}

func (c *caminante) emitir(b Bloque) {
	for _, l := range c.listas {
		if !l.sinMarca || l.sangra {
			b.Sangria++
		}
	}
	b.Marca, c.marca = c.marca, ""
	b.Celda, b.Recuadro, b.Region, b.Oculta, b.Columnas = c.celda, c.recuadro, c.region, c.oculta, c.columnas
	if h := b.Base.hoja; h != nil {
		// Con el reset de Tailwind no hay más márgenes que los de la hoja, ni marcas.
		b.Marca = ""
		switch {
		case b.Tipo == SaltoPagina:
			b.Arriba, b.Abajo = 1.5*b.Base.Tamano, 1.5*b.Base.Tamano // su `margin: 1.5em 0`, en línea
		case b.cont != nil && b.cont.DataAtom == atom.P:
			b.Abajo = h.Margen * b.Base.Tamano
		}
		c.bloques = append(c.bloques, b)
		return
	}
	switch {
	case b.Region != "": // fuera de la corriente: no empuja nada
	case b.Tipo == Titulo:
		t := titulos[b.Nivel]
		b.Arriba, b.Abajo = t.arriba*t.tamano, t.abajo*t.tamano
	case b.Tipo == Regla:
		b.Arriba, b.Abajo = 16, 16
	case b.Tipo == SaltoPagina:
		b.Arriba, b.Abajo = 24, 24 // `margin: 1.5em 0`
	case b.Tipo == Figura:
		b.Arriba, b.Abajo = 16, 16 // `margin: 1em auto`
	case b.cont != nil && esEntradaBib(b.cont):
		b.Arriba, b.Abajo = 0.35*b.Base.Tamano, 0.35*b.Base.Tamano // `margin:.35em 0`
	case b.cont != nil && esLeyenda(b.cont):
		b.Arriba, b.Abajo = 0.25*b.Base.Tamano, 0.75*b.Base.Tamano // `margin:.25em 0 .75em`
	case b.cont != nil && (b.cont.DataAtom == atom.Li || esCelda(b.cont)):
		// los elementos de una lista van seguidos; el margen es de la lista entera. El
		// texto suelto de una celda no es un <p>: no lleva margen
	default:
		b.Abajo = 8 // `.sytue-page p { margin: 0 0 0.5em }`
	}
	b.margenesPropios()
	c.bloques = append(c.bloques, b)
}

// margenesPropios pone el espaciado propio de un párrafo importado, que manda sobre el del
// folio. Una región va fuera de la corriente y no empuja nada.
func (b *Bloque) margenesPropios() {
	if b.Region != "" {
		return
	}
	if b.Base.conArriba {
		b.Arriba = b.Base.arriba
	}
	if b.Base.conAbajo {
		b.Abajo = b.Base.abajo
	}
}

//nolint:gocyclo // una rama por etiqueta de bloque
func (c *caminante) bloque(n *html.Node, e Estilo, tipo Tipo, nivel, prof int) {
	if n.Type != html.ElementNode || invisibles[n.DataAtom] || capaDecorativa(n) {
		return
	}
	if esColumnas(n) && c.columnas == nil {
		c.columnas = &Columnas{N: columnasDe(n), nodo: n}
		c.contenedor(n, aplicar(e, n), tipo, nivel, prof)
		c.columnas = nil
		return
	}
	if esFigura(n) {
		c.emitir(Bloque{Tipo: Figura, Ilustracion: figuraDe(n), Base: e, Lugar: Lugar{cont: n.Parent, primero: n, ultimo: n}})
		return
	}
	dentro := aplicar(e, n)
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		c.contenedor(n, dentro, Titulo, int(n.Data[1]-'0'), prof)
	case atom.Blockquote:
		c.contenedor(n, dentro, Cita, 0, prof)
	case atom.Nav:
		if c.recuadro == nil && tieneAtributo(n, "data-toc") {
			c.recuadro = &Recuadro{nodo: n}
			c.contenedor(n, dentro, tipo, nivel, prof)
			c.recuadro = nil
			return
		}
		c.contenedor(n, dentro, tipo, nivel, prof)
	case atom.Ul, atom.Ol:
		c.lista(n, dentro, tipo, nivel, prof)
	case atom.Li:
		c.marca = c.marcaDe(n)
		c.contenedor(n, dentro, tipo, nivel, prof)
		c.marca = ""
	case atom.Hr:
		c.emitir(Bloque{Tipo: Regla, Base: e, Lugar: Lugar{cont: n.Parent, primero: n, ultimo: n}})
	case atom.Div:
		if r := regionDe(n); r != "" && c.region == "" {
			c.region, c.oculta = r, r == Encabezado && tieneAtributo(n, "data-first-page-different")
			c.contenedor(n, dentro, tipo, nivel, prof)
			c.region, c.oculta = "", false
			return
		}
		if tieneAtributo(n, "data-page-break") {
			c.emitir(Bloque{Tipo: SaltoPagina, Base: e, Lugar: Lugar{cont: n.Parent, primero: n, ultimo: n}})
			return
		}
		c.contenedor(n, dentro, tipo, nivel, prof)
	case atom.Table:
		if c.celda != nil {
			// Una tabla dentro de otra no se edita (ver `entender`): su texto se lee en la
			// celda de fuera, un bloque detrás de otro.
			c.contenedor(n, dentro, tipo, nivel, prof)
			return
		}
		c.tabla(n, dentro, prof)
	default:
		c.contenedor(n, dentro, tipo, nivel, prof)
	}
}

// lista recorre una <ul> o una <ol>. Las de las notas al final y las del índice no llevan
// viñeta ni número; de ellas, solo las subentradas del índice van sangradas.
func (c *caminante) lista(n *html.Node, dentro Estilo, tipo Tipo, nivel, prof int) {
	estilo := atributo(n, "style")
	ol, ul := n.DataAtom == atom.Ol, n.DataAtom == atom.Ul
	l := &lista{ordenada: ol}
	l.sinMarca = ol && esSeccionFinal(n.Parent) && estilo == estiloListaNotasFinal ||
		ul && enIndice(n) && (estilo == estiloListaIndice || estilo == estiloSubIndice)
	l.sangra = ul && estilo == estiloSubIndice && enIndice(n)
	if ul {
		l.vineta = vinetaCSS(estilo)
	}
	if inicio, err := strconv.Atoi(atributo(n, "start")); err == nil {
		l.n = inicio - 1
	}
	c.listas = append(c.listas, l)
	c.contenedor(n, dentro, tipo, nivel, prof)
	c.listas = c.listas[:len(c.listas)-1]
	if len(c.bloques) > 0 && len(c.listas) == 0 {
		c.bloques[len(c.bloques)-1].Abajo = 8 // `ul, ol { margin: 0 0 0.5em }`
		switch {
		case dentro.hoja != nil:
			c.bloques[len(c.bloques)-1].Abajo = dentro.hoja.Margen * dentro.Tamano
		case l.sinMarca:
			c.bloques[len(c.bloques)-1].Abajo = 4 // `margin:.25em 0`
		}
	}
}

// marcaDe es la marca de un <li>: la viñeta, o su número si la lista es numerada, y
// ninguna en las listas sin marca.
func (c *caminante) marcaDe(li *html.Node) string {
	if len(c.listas) == 0 {
		return "•"
	}
	l := c.listas[len(c.listas)-1]
	l.n++
	if valor, err := strconv.Atoi(atributo(li, "value")); err == nil {
		l.n = valor
	}
	switch {
	case l.sinMarca:
		return ""
	case l.ordenada:
		return strconv.Itoa(l.n) + "."
	case l.vineta != "":
		return l.vineta
	}
	return "•"
}

// vinetaCSS es la viñeta de un `list-style-type` con una cadena (`'- '`, la de una lista
// importada de Pages), sin las comillas ni el espacio de detrás; "" si no lleva.
func vinetaCSS(estilo string) string {
	for _, decl := range strings.Split(estilo, ";") {
		prop, valor, _ := strings.Cut(decl, ":")
		if strings.TrimSpace(prop) != "list-style-type" {
			continue
		}
		valor = strings.TrimSpace(valor)
		if len(valor) >= 2 && (valor[0] == '\'' || valor[0] == '"') && valor[len(valor)-1] == valor[0] {
			return strings.TrimSpace(valor[1 : len(valor)-1])
		}
	}
	return ""
}

// tabla recorre una <table>: sus celdas en la rejilla y, celda a celda, sus bloques.
func (c *caminante) tabla(n *html.Node, e Estilo, prof int) {
	t := leerRejilla(n, prof)
	for _, cel := range t.Celdas {
		// El fondo y la alineación vertical son de la CAJA de la celda: no se heredan.
		dentro := aplicar(e, cel.nodo)
		cel.Fondo, cel.Vertical = dentro.Fondo, dentro.valign
		dentro.Fondo, dentro.valign = e.Fondo, ""
		if cel.Vertical == "" || cel.Vertical == "baseline" {
			cel.Vertical = "top"
		}
		if cel.Cabecera {
			dentro.Peso = 700
		}
		c.celda = cel
		c.contenedor(cel.nodo, dentro, Parrafo, 0, prof+3)
	}
	c.celda = nil
}

// leerRejilla saca las celdas de una tabla con su sitio en la rejilla.
// ponytail: un rowspan="0" (hasta el final de su grupo de filas) cuenta como 1; la web no
// lo escribe.
func leerRejilla(n *html.Node, prof int) *Tabla {
	var filas []*html.Node
	var recorrer func(n *html.Node, prof int)
	recorrer = func(n *html.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			switch h.DataAtom {
			case atom.Thead, atom.Tbody, atom.Tfoot:
				recorrer(h, prof+1)
			case atom.Tr:
				filas = append(filas, h)
			default: // caption, colgroup: no traen celdas
			}
		}
	}
	recorrer(n, prof)
	t := &Tabla{nodo: n, Filas: len(filas)}
	ocupado := make([][]bool, len(filas))
	for f, tr := range filas {
		col := 0
		for td := tr.FirstChild; td != nil; td = td.NextSibling {
			if !esCelda(td) {
				continue
			}
			for col < len(ocupado[f]) && ocupado[f][col] {
				col++
			}
			cel := &Celda{
				Tabla: t, Fila: f, Col: col, Cabecera: td.DataAtom == atom.Th, nodo: td,
				Cols: tramoCelda(td, "colspan", 1000), Filas: min(tramoCelda(td, "rowspan", 65534), len(filas)-f),
			}
			ocupar(ocupado, cel)
			t.Celdas = append(t.Celdas, cel)
			t.Cols = max(t.Cols, col+cel.Cols)
			col += cel.Cols
		}
	}
	return t
}

// ocupar marca en la rejilla las casillas que abarca la celda.
func ocupar(ocupado [][]bool, cel *Celda) {
	for df := range cel.Filas {
		fila := &ocupado[cel.Fila+df]
		for len(*fila) < cel.Col+cel.Cols {
			*fila = append(*fila, false)
		}
		for dc := range cel.Cols {
			(*fila)[cel.Col+dc] = true
		}
	}
}

func esCelda(n *html.Node) bool {
	return n.Type == html.ElementNode && (n.DataAtom == atom.Td || n.DataAtom == atom.Th) && n.Namespace == ""
}

// tramoCelda lee un colspan o un rowspan como el navegador: un entero de 1 a `tope`, y 1 si
// no se lee.
func tramoCelda(n *html.Node, clave string, tope int) int {
	v, err := strconv.Atoi(strings.TrimSpace(atributo(n, clave)))
	if err != nil || v < 1 {
		return 1
	}
	return min(v, tope)
}

// Indice busca el bloque que ocupa ese lugar del árbol; -1 si ya no existe. Es como se
// reencuentra el bloque en el que se escribía después de una edición que cambia la
// lista de bloques (partir, fundir, hacer lista).
func Indice(bloques []Bloque, l Lugar) int { return indice(bloques, l) }

// indice es Indice.
func indice(bloques []Bloque, l Lugar) int {
	for i, b := range bloques {
		if b.cont == l.cont && b.primero == l.primero {
			return i
		}
	}
	return -1
}

// aplanador convierte nodos en línea en tramos, juntando blancos como hace un navegador:
// cualquier racha de espacios, tabuladores y saltos es UN espacio, y ninguno al principio
// ni al final de la línea. El espacio duro (U+00A0) no es un blanco: se queda.
type aplanador struct {
	base        Estilo
	tramos      []Tramo
	conTexto    bool // la línea en curso ya tiene algo delante
	trasEspacio bool // lo último puesto es un espacio: los blancos que sigan sobran
}

func (a *aplanador) nodo(n *html.Node, e Estilo, cadena []*html.Node, prof int) {
	switch {
	case n.Type == html.TextNode:
		a.texto(n.Data, e, cadena)
	case n.Type != html.ElementNode || invisibles[n.DataAtom] || prof > profundidadMax:
	case n.DataAtom == atom.Br:
		a.recortar()
		a.tramos = append(a.tramos, Tramo{Salto: true, Estilo: e, cadena: cadena})
		a.conTexto, a.trasEspacio = false, false
	case objetoAlLeer(n):
		t := Tramo{Objeto: n.Data, Estilo: e, cadena: cadena, nodo: n, CampoPagina: esCampoPagina(n)}
		if esVuelta(n) || esCita(n) || esCampoCombinacion(n) {
			// El enlace de vuelta de una nota al final no se edita: es UNA runa, que se
			// pinta con su texto y el estilo de enlace.
			t.Rotulo, t.Estilo = textoDe(n), aplicar(e, n)
		}
		if esCampoCombinacion(n) {
			t.Campo = atributo(n, "data-mergefield")
		}
		if n.DataAtom == atom.Img && n.Namespace == "" {
			t.Imagen = leerImagen(n)
		}
		a.tramos = append(a.tramos, t)
		a.conTexto, a.trasEspacio = true, false
	default:
		dentro := aplicar(e, n)
		envuelto := make([]*html.Node, len(cadena)+1) // copia: los hermanos de n comparten `cadena`
		copy(envuelto, cadena)
		envuelto[len(cadena)] = n
		for h := n.FirstChild; h != nil; h = h.NextSibling {
			a.nodo(h, dentro, envuelto, prof+1)
		}
	}
}

func (a *aplanador) texto(s string, e Estilo, cadena []*html.Node) {
	var b strings.Builder
	if a.base.pre {
		// `white-space: pre-wrap`: los blancos se quedan como están, tabuladores incluidos.
		if s != "" {
			a.conTexto, a.trasEspacio = true, false
			a.tramos = append(a.tramos, Tramo{Texto: s, Estilo: e, cadena: cadena})
		}
		return
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			// El espacio se queda en el tramo DONDE ESTABA: al reescribir un bloque editado,
			// `a <b>b</b>` no debe volver como `a<b> b</b>`.
			if a.conTexto && !a.trasEspacio {
				b.WriteByte(' ')
				a.trasEspacio = true
			}
			continue
		}
		a.conTexto, a.trasEspacio = true, false
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		a.tramos = append(a.tramos, Tramo{Texto: b.String(), Estilo: e, cadena: cadena})
	}
}

// recortar quita el espacio con el que acaba la línea: un navegador no lo pinta.
func (a *aplanador) recortar() {
	n := len(a.tramos)
	if !a.trasEspacio || n == 0 {
		return
	}
	if a.tramos[n-1].Texto = strings.TrimSuffix(a.tramos[n-1].Texto, " "); a.tramos[n-1].Texto == "" {
		a.tramos = a.tramos[:n-1]
	}
	a.trasEspacio = false
}

// cerrar quita además el <br> final: en un navegador el último salto de un bloque no abre
// otra línea (es el relleno de un párrafo vacío, `<p><br></p>`).
func (a *aplanador) cerrar() {
	a.recortar()
	if n := len(a.tramos); n > 0 && a.tramos[n-1].Salto {
		a.tramos = a.tramos[:n-1]
		a.recortar()
	}
}

// Imagen es un <img> con lo que la maqueta necesita de él. El modelo es el de la web
// (`packages/office/src/editor/images.ts`): los data-* mandan y el `style` se deriva de ellos,
// así que se leen los data-*; una imagen que la web no ha tocado aún (la de `insertImage`)
// no los tiene, y mide lo que diga su `style`, sus atributos o, si nada, el fichero.
type Imagen struct {
	Src, Alt string
	// Ajuste es `data-wrap`: "inline" (el de por defecto), "square", "tight", "topBottom",
	// "behind" o "front". Alinear es `data-align`: "left", "center" o "right".
	Ajuste, Alinear string
	Ancho, Alto     float32 // px de CSS; 0 si no se dicen
	X, Y            float32 // `data-x`, `data-y`: dónde va una imagen detrás o delante del texto
	// Gestionada es `data-sytue-image`: la web la ha tocado y va `vertical-align: bottom`;
	// una sin tocar se apoya en la línea base, como cualquier <img>.
	Gestionada bool
	// Recorte es `data-crop`: lo recortado de cada lado (arriba, derecha, abajo, izquierda)
	// como fracción de su lado, ya saneado (`clampCrop`).
	Recorte [4]float32
	nodo    *html.Node
}

// Flota dice si la imagen va fuera de la línea: en su propio renglón (arriba y abajo),
// flotando con el texto alrededor (cuadrado, estrecho) o fuera de la corriente (detrás,
// delante).
func (im *Imagen) Flota() bool { return im.Ajuste != "inline" }

// Rodeada dice si flota con el texto alrededor: cuadrado o estrecho (`isFloatWrap`).
func (im *Imagen) Rodeada() bool { return im.Ajuste == "square" || im.Ajuste == "tight" }

// Absoluta dice si va detrás o delante del texto, en X e Y del folio, sin ocupar sitio.
func (im *Imagen) Absoluta() bool { return im.Ajuste == "behind" || im.Ajuste == "front" }

func leerImagen(n *html.Node) *Imagen {
	im := &Imagen{Src: atributo(n, "src"), Alt: atributo(n, "alt"), Ajuste: "inline", Alinear: "left", nodo: n}
	if v := atributo(n, "data-wrap"); slices.Contains([]string{"square", "tight", "topBottom", "behind", "front"}, v) {
		im.Ajuste = v
	}
	if v := atributo(n, "data-align"); v == "center" || v == "right" {
		im.Alinear = v
	}
	im.Gestionada = tieneAtributo(n, "data-sytue-image")
	num := func(clave string) float32 {
		v, err := strconv.ParseFloat(strings.TrimSpace(atributo(n, clave)), 32)
		if err != nil || v < 0 || v > 20000 {
			return 0
		}
		return float32(v)
	}
	im.Ancho, im.Alto, im.X, im.Y = num("data-w"), num("data-h"), num("data-x"), num("data-y")
	im.Recorte = leerRecorte(atributo(n, "data-crop"))
	for _, d := range declaraciones(atributo(n, "style")) {
		px, ok := pxCSS(strings.ToLower(d[1]))
		switch {
		case !ok || px < 0:
		case strings.EqualFold(d[0], "width") && im.Ancho == 0:
			im.Ancho = px
		case strings.EqualFold(d[0], "height") && im.Alto == 0:
			im.Alto = px
		}
	}
	if im.Ancho == 0 {
		im.Ancho = num("width")
	}
	if im.Alto == 0 {
		im.Alto = num("height")
	}
	return im
}

// tieneAtributo dice si n lleva ese atributo, aunque sea vacío (`data-sytue-image=""`).
func tieneAtributo(n *html.Node, clave string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == clave {
			return true
		}
	}
	return false
}

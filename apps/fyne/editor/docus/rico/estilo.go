// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"image/color"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Alineacion es `text-align`.
type Alineacion uint8

// Las cuatro de CSS, en el orden de `FormatState.align`.
const (
	Izquierda Alineacion = iota
	Centro
	Derecha
	Justificada
)

// Estilo es cómo se ve un tramo de texto: lo que en CSS se hereda, ya resuelto.
type Estilo struct {
	Peso         int // el `font-weight` de CSS: 400 normal, 600, 700
	Cursiva      bool
	Subrayado    bool
	Tachado      bool
	Color        color.NRGBA
	Fondo        color.NRGBA // A == 0: sin resalte
	Tamano       float32     // px de CSS
	Enlace       string      // href, o "" si no es un enlace
	Alineacion   Alineacion
	Interlineado float32 // múltiplo del tamaño, como `line-height: 1.5`
	// Vertical es 1 en un superíndice y -1 en un subíndice.
	Vertical int8
	// Familia es el `font-family` tal cual se escribió, con sus mayúsculas: es lo que
	// enseña el selector de la cinta. Se pinta con Figtree (ver `propiedad`).
	Familia string
	// MargenIzq, MargenDer y Sangria1 son `margin-left`, `margin-right` y `text-indent`
	// del bloque, en px: lo que escriben la sangría y los marcadores de la regla.
	MargenIzq, MargenDer, Sangria1 float32
	// valign es el `vertical-align` de una celda. No se hereda: lo recoge la celda.
	valign string
	// arriba y abajo son el `margin-top` y el `margin-bottom` del propio bloque, en px (los de un
	// .docx importado: el espaciado antes y después del párrafo), si conArriba y conAbajo; si no,
	// los del folio. No se heredan.
	arriba, abajo       float32
	conArriba, conAbajo bool
	// pre es `white-space: pre-wrap`: los blancos y los tabuladores se conservan.
	pre bool
	// Tabulador es el `tab-size` de un bloque con `white-space: pre-wrap`, en px: el de un
	// .docx importado, que conserva sus tabuladores. 0 es que el bloque junta los blancos.
	Tabulador float32
	// Paradas es el `--sytue-tabs` de un párrafo importado de un .docx: sus paradas de
	// tabulación propias, tal cual (ver [LeerParadas]). "" es que solo valen las de Tabulador.
	Paradas string
	// hoja es la hoja del contenedor (ver [Hoja]); nil es la del folio. Va en el estilo
	// para que la herede todo lo de dentro sin pasarla aparte por cada recorrido.
	hoja *Hoja
}

// Hoja es la hoja de estilo del CONTENEDOR del documento: lo que la web le pone con CSS al
// elemento que aloja el HTML. Sin hoja es la del folio de Docus (`.sytue-page` de
// `editor.css`); una caja de texto de Diapos trae la suya (`BOX_CSS` de SlidesCanvas.tsx).
//
// Una hoja propia NO lleva las decoraciones de `editor.css`: ahí manda el reset de
// Tailwind (Preflight), así que un enlace hereda color y no se subraya, una cita no va en
// cursiva, los márgenes son cero salvo los que declare la hoja y las listas no llevan
// viñeta ni número (`list-style: none`).
type Hoja struct {
	// Base es el estilo del contenedor: tamaño, color, peso, interlineado y alineación.
	Base Estilo
	// Titulos son el tamaño y el peso de h1, h2 y h3 (en 1, 2 y 3). De h4 a h6 el reset
	// los deja como el texto de alrededor.
	Titulos [4]EstiloTitulo
	// Margen es el `margin-bottom` de un <p> y de una lista, en em de su propio tamaño.
	Margen float32
}

// EstiloTitulo es cómo pinta una hoja un nivel de título.
type EstiloTitulo struct {
	Tamano float32
	Peso   int
}

// DeHoja dice si el estilo es el de un contenedor con su propia hoja (ver [Hoja]), donde
// tablas y reglas van con el reset de Tailwind y no con `editor.css`.
func (e Estilo) DeHoja() bool { return e.hoja != nil }

// Negrita dice si el tramo se ve en negrita.
func (e Estilo) Negrita() bool { return e.Peso >= 600 }

// El folio de la web (`.sytue-page` en packages/office/src/editor/editor.css): papel blanco y
// tinta negra SIEMPRE, también en tema oscuro; 16 px con interlineado 1,5. No son tokens
// de la interfaz y no deben serlo: el papel no sigue al tema, y una tinta que lo siguiera
// saldría casi blanca sobre él.
var (
	// Papel es `--editor-page-bg`.
	Papel = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// Tinta es el `color: #0a0a0a` de `.sytue-page`.
	Tinta       = color.NRGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff}
	tintaCita   = color.NRGBA{R: 0x47, G: 0x55, B: 0x69, A: 0xff} // blockquote: #475569
	TintaLinea  = color.NRGBA{R: 15, G: 23, B: 42, A: 41}         // --page-rule: rgba(15,23,42,.16)
	tintaEnlace = tokens.Color("primary-700", tokens.Light)       // el enlace sobre papel, en los dos temas
	// FondoAncla es el de lo comentado: `color-mix(in srgb, var(--sytue-primary) 16%,
	// transparent)` de `mark[data-comment-id]`, el primario del papel (el claro).
	FondoAncla = conAlfa(tokens.Color("primary", tokens.Light), 0.16)
	estiloBase = Estilo{Peso: 400, Color: Tinta, Tamano: 16, Interlineado: 1.5}
)

// titulos son `.sytue-page h1…h3`. De h4 a h6 la web no dice nada y el reset de Tailwind
// los deja como texto corriente: aquí también.
var titulos = [...]struct {
	tamano        float32
	peso          int
	arriba, abajo float32 // márgenes, en em del propio título
}{1: {32, 700, 0.67, 0.35}, 2: {24, 600, 0.83, 0.35}, 3: {18.72, 600, 1, 0.35}, 4: {16, 400, 0, 0}, 5: {16, 400, 0, 0}, 6: {16, 400, 0, 0}}

// aplicar devuelve el estilo de lo que hay DENTRO del elemento n.
//
//nolint:gocyclo // una rama por etiqueta: la ramificación ES la tabla
func aplicar(e Estilo, n *html.Node) Estilo {
	e.conArriba, e.conAbajo = false, false // los márgenes son de cada elemento, no se heredan
	switch n.DataAtom {
	case atom.B, atom.Strong:
		e.Peso = 700
	case atom.I, atom.Em:
		e.Cursiva = true
	case atom.U:
		e.Subrayado = true
	case atom.S, atom.Strike:
		e.Tachado = true
	case atom.Ins, atom.Del:
		if esPista(n) {
			return estiloPista(e, n) // su `style` es solo el color del autor
		}
		e.Subrayado, e.Tachado = e.Subrayado || n.DataAtom == atom.Ins, e.Tachado || n.DataAtom == atom.Del
	case atom.Sup:
		// El `font-size: 75%` del reset de Tailwind (Preflight), que es el de la web; la
		// vista lo sube `top: -0.5em`.
		e.Tamano *= 0.75
		e.Vertical = 1
	case atom.Sub:
		e.Tamano *= 0.75
		e.Vertical = -1
	case atom.A:
		if href := atributo(n, "href"); href != "" {
			e.Enlace = href
			if e.hoja == nil {
				e.Subrayado, e.Color = true, tintaEnlace
			}
		}
	case atom.Blockquote:
		if e.hoja == nil {
			e.Cursiva, e.Color = true, tintaCita
		}
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		nivel := n.Data[1] - '0'
		switch {
		case e.hoja == nil:
			e.Tamano, e.Peso = titulos[nivel].tamano, titulos[nivel].peso
		case nivel <= 3:
			e.Tamano, e.Peso = e.hoja.Titulos[nivel].Tamano, e.hoja.Titulos[nivel].Peso
		}
	case atom.Font:
		if c, ok := colorCSS(atributo(n, "color")); ok {
			e.Color = c
		}
	case atom.P:
		if esEntradaBib(n) && atributo(n, "style") == estiloEntradaBib {
			e.MargenIzq = 2 * e.Tamano // `padding-left:2em`; el `text-indent`, abajo
		}
	case atom.Div:
		if regionDe(n) != "" {
			e.Color = tintaRegion // el `font-size:12px` sale de su `style`, abajo
		}
	case atom.Mark:
		if tieneAtributo(n, "data-comment-id") {
			e.Fondo = FondoAncla // `color: inherit`: la tinta no cambia
		}
	default: // lo demás no cambia cómo se ve el texto por sí mismo; su `style`, abajo
	}
	switch strings.ToLower(atributo(n, "align")) {
	case "center":
		e.Alineacion = Centro
	case "right":
		e.Alineacion = Derecha
	case "justify":
		e.Alineacion = Justificada
	}
	if css := atributo(n, "style"); css != "" {
		e.declarar(css, true)
	}
	return e
}

func conAlfa(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(a*255 + 0.5)
	return c
}

func atributo(n *html.Node, clave string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == clave {
			return a.Val
		}
	}
	return ""
}

// declarar aplica un atributo `style` y devuelve las propiedades que NO entiende: las que
// no conoce y las que conoce con un valor que no sabe leer. `bloque` admite las de párrafo.
func (e *Estilo) declarar(css string, bloque bool) []string {
	var sin []string
	// ponytail: parte por `;` a secas. Un `;` dentro de comillas o de url() daría una
	// propiedad rara, que cae en «no entendida»: falla hacia el lado seguro.
	for _, decl := range strings.Split(css, ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		prop, valor = strings.ToLower(strings.TrimSpace(prop)), strings.TrimSpace(valor)
		if prop == "" && !ok {
			continue
		}
		if prop == "font-family" {
			// Con sus mayúsculas y sin comillas: «Times New Roman», que es lo que enseña el
			// selector, no «"times new roman"». De una lista con las de respaldo (la de un .docx
			// importado: «'Times New Roman','Liberation Serif',serif»), la primera.
			primera, _, _ := strings.Cut(valor, ",")
			e.Familia = strings.Trim(strings.TrimSpace(primera), `"'`)
			continue
		}
		if !e.propiedad(prop, strings.ToLower(valor), bloque) {
			sin = append(sin, prop)
		}
	}
	return sin
}

//nolint:gocyclo // una rama por propiedad de CSS: la ramificación ES la tabla
func (e *Estilo) propiedad(prop, valor string, bloque bool) bool {
	switch prop {
	case "font-weight":
		return e.peso(valor)
	case "font-style":
		switch valor {
		case "italic", "oblique":
			e.Cursiva = true
		case "normal":
			e.Cursiva = false
		default:
			return false
		}
	case "text-decoration", "text-decoration-line":
		for _, linea := range strings.Fields(valor) {
			switch linea {
			case "underline":
				e.Subrayado = true
			case "line-through":
				e.Tachado = true
			case "none": // no quita la del padre: en CSS la decoración no se hereda, se propaga
			default:
				return false
			}
		}
	case "color":
		c, ok := colorCSS(valor)
		if !ok || c.A == 0 {
			return false
		}
		e.Color = c
	case "background-color":
		c, ok := colorCSS(valor)
		if !ok {
			return false
		}
		e.Fondo = c
	case "font-size":
		t, ok := tamanoCSS(valor, e.Tamano)
		if !ok {
			return false
		}
		e.Tamano = t
	case "margin-left", "margin-right", "text-indent":
		return bloque && e.margen(prop, valor)
	case "list-style-type":
		// Solo la viñeta como cadena (`'- '`), que pinta la lista (vinetaCSS).
		return bloque && vinetaCSS(prop+":"+valor) != ""
	case "white-space", "tab-size", "--sytue-tabs":
		return bloque && e.blancos(prop, valor)
	case "margin-top", "margin-bottom":
		return bloque && e.margenVertical(prop, valor)
	case "text-align":
		al, ok := map[string]Alineacion{"left": Izquierda, "start": Izquierda, "center": Centro, "right": Derecha, "end": Derecha, "justify": Justificada}[valor]
		if !ok || !bloque {
			return false
		}
		e.Alineacion = al
	case "line-height":
		if valor == "normal" {
			valor = "1.2"
		}
		valor = e.enMultiplo(valor)
		n, err := strconv.ParseFloat(valor, 32) // solo el múltiplo sin unidad, que es lo que escribe la web
		if err != nil || n < 0 || n > 1000 || !bloque {
			return false
		}
		e.Interlineado = min(max(float32(n), 0.5), 5)
	case "vertical-align":
		// Solo en bloque, donde es el de una celda (en un <p> no hace nada). En línea
		// movería el texto, y eso no se pinta.
		if !bloque || !slices.Contains([]string{"top", "middle", "bottom", "baseline"}, valor) {
			return false
		}
		e.valign = valor
	default:
		return false
	}
	return true
}

// blancos lee `white-space`, `tab-size` y `--sytue-tabs`: pre-wrap conserva los blancos y los
// tabuladores de un párrafo importado; la parada, de tab-size (o la de CSS: ocho espacios, unos
// 2 em), salvo las propias.
func (e *Estilo) blancos(prop, valor string) bool {
	if prop == "--sytue-tabs" {
		return e.paradas(valor)
	}
	if prop == "tab-size" {
		n, ok := pxCSS(valor)
		if ok && n > 0 {
			e.Tabulador = n
		}
		return ok && n > 0
	}
	switch valor {
	case "pre-wrap":
		e.pre = true
		if e.Tabulador == 0 {
			e.Tabulador = 2 * e.Tamano
		}
	case "normal":
		e.pre = false
	default:
		return false
	}
	return true
}

// margenVertical lee el espaciado antes y después de un párrafo importado, en px; la vista lo
// pone en Bloque.Arriba y Bloque.Abajo.
func (e *Estilo) margenVertical(prop, valor string) bool {
	n, ok := pxCSS(valor)
	if !ok || n < 0 {
		return false
	}
	if prop == "margin-top" {
		e.arriba, e.conArriba = n, true
	} else {
		e.abajo, e.conAbajo = n, true
	}
	return true
}

// enMultiplo pasa a múltiplo del tamaño un interlineado con unidad (pt, px, em o %);
// lo demás lo devuelve tal cual.
func (e *Estilo) enMultiplo(valor string) string {
	if e.Tamano <= 0 {
		return valor
	}
	for sufijo, factor := range map[string]float64{"pt": 4.0 / 3 / float64(e.Tamano), "px": 1 / float64(e.Tamano), "em": 1, "%": 0.01} {
		if num, ok := strings.CutSuffix(valor, sufijo); ok && (sufijo != "em" || !strings.HasSuffix(valor, "rem")) {
			if n, err := strconv.ParseFloat(num, 32); err == nil {
				return strconv.FormatFloat(n*factor, 'f', 3, 32)
			}
			return valor
		}
	}
	return valor
}

// peso lee `font-weight`: los nombres y el número de 1 a 1000.
func (e *Estilo) peso(valor string) bool {
	switch valor {
	case "bold", "bolder":
		e.Peso = 700
	case "normal", "lighter":
		e.Peso = 400
	default:
		n, err := strconv.Atoi(valor)
		if err != nil || n < 1 || n > 1000 {
			return false
		}
		e.Peso = n
	}
	return true
}

// margen lee las tres propiedades de sangría del párrafo, solo en px, que es lo único
// que escriben la sangría y la regla de la web. Los márgenes no pueden ser negativos; la
// sangría de primera línea sí (la francesa).
func (e *Estilo) margen(prop, valor string) bool {
	n, ok := pxCSS(valor)
	if em, esEm := strings.CutSuffix(valor, "em"); !ok && esEm && !strings.HasSuffix(em, "r") {
		// La sangría del índice va en em (`margin-left:1.5em`): del tamaño del bloque.
		if v, err := strconv.ParseFloat(em, 32); err == nil && v >= -100 && v <= 100 {
			n, ok = float32(v)*e.Tamano, true
		}
	}
	if !ok || (prop != "text-indent" && n < 0) {
		return false
	}
	switch prop {
	case "margin-left":
		e.MargenIzq = n
	case "margin-right":
		e.MargenDer = n
	default:
		e.Sangria1 = n
	}
	return true
}

// Parada es una parada de tabulación propia: dónde, en px desde el borde izquierdo del texto
// del bloque (negativa, a la izquierda de la sangría: solo la alcanza una primera línea con
// sangría francesa); cómo se alinea lo que la sigue (left, center, right o decimal) y con qué se
// rellena el hueco (dot, hyphen, underscore, heavy, middledot o "").
type Parada struct {
	Pos           float32
	Alin, Relleno string
}

// paradas lee `--sytue-tabs` de un bloque.
func (e *Estilo) paradas(valor string) bool {
	if LeerParadas(valor) == nil {
		return false
	}
	e.Paradas = valor
	return true
}

// LeerParadas lee `--sytue-tabs`: «120px left, 600px right dot», en orden. nil si no se entiende.
func LeerParadas(v string) []Parada {
	var out []Parada
	for _, s := range strings.Split(v, ",") {
		c := strings.Fields(s)
		if len(c) < 2 || len(c) > 3 {
			return nil
		}
		pos, ok := pxCSS(c[0])
		if !ok || !slices.Contains([]string{"left", "center", "right", "decimal"}, c[1]) {
			return nil
		}
		p := Parada{Pos: pos, Alin: c[1]}
		if len(c) == 3 {
			if !slices.Contains([]string{"dot", "hyphen", "underscore", "heavy", "middledot"}, c[2]) {
				return nil
			}
			p.Relleno = c[2]
		}
		out = append(out, p)
	}
	return out
}

// pxCSS lee un `12px` o un `0`.
func pxCSS(valor string) (float32, bool) {
	if valor == "0" {
		return 0, true
	}
	num, ok := strings.CutSuffix(valor, "px")
	n, err := strconv.ParseFloat(num, 32)
	if !ok || err != nil || n < -2000 || n > 2000 {
		return 0, false
	}
	return float32(n), true
}

var reRGB = regexp.MustCompile(`^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*([0-9.]+)\s*)?\)$`)

// colorCSS lee `#rgb`, `#rrggbb`, `rgb()`, `rgba()` y `transparent`.
// ponytail: sin nombres de color (`red`). Ni la web ni los importadores los escriben; uno
// que llegue de un pegado deja el documento en solo lectura, que es el lado seguro.
func colorCSS(s string) (color.NRGBA, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "transparent" {
		return color.NRGBA{}, true
	}
	if hex, ok := strings.CutPrefix(s, "#"); ok {
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		if len(hex) != 6 || err != nil {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}, true //nolint:gosec // tres bytes de un hex de seis cifras
	}
	m := reRGB.FindStringSubmatch(s)
	if m == nil {
		return color.NRGBA{}, false
	}
	var canal [3]uint8
	for i := range canal {
		n, _ := strconv.Atoi(m[i+1])
		if n > 255 {
			return color.NRGBA{}, false
		}
		canal[i] = uint8(n) //nolint:gosec // acotado a 255 en la línea de arriba
	}
	alfa := 1.0
	if m[4] != "" {
		var err error
		if alfa, err = strconv.ParseFloat(m[4], 64); err != nil || alfa < 0 || alfa > 1 {
			return color.NRGBA{}, false
		}
	}
	return color.NRGBA{R: canal[0], G: canal[1], B: canal[2], A: uint8(alfa*255 + 0.5)}, true //nolint:gosec // alfa está entre 0 y 1
}

// tamanosCSS son las tallas con nombre, que es lo que escribe `execCommand('fontSize')`
// con estilos en CSS: de `x-small` (1) a `xxx-large` (7).
var tamanosCSS = map[string]float32{
	"xx-small": 9, "x-small": 10, "small": 13, "medium": 16, "large": 18, "x-large": 24, "xx-large": 32, "xxx-large": 48,
	"-webkit-xxx-large": 48, // el peldaño 7 como lo escribe Chrome
}

func tamanoCSS(valor string, actual float32) (float32, bool) {
	if t, ok := tamanosCSS[valor]; ok {
		return t, true
	}
	for sufijo, factor := range map[string]float32{"px": 1, "pt": 4.0 / 3, "rem": 16, "em": actual, "%": actual / 100} {
		if num, ok := strings.CutSuffix(valor, sufijo); ok && (sufijo != "em" || !strings.HasSuffix(valor, "rem")) {
			n, err := strconv.ParseFloat(num, 32)
			if err != nil || n <= 0 {
				return 0, false
			}
			if t := float32(n) * factor; t >= 4 && t <= 400 {
				return t, true
			}
			return 0, false
		}
	}
	return 0, false
}

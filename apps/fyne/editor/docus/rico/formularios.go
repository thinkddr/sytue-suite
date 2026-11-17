// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"image/color"
	"regexp"
	"slices"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los CAMPOS DE FORMULARIO rellenables de «Insertar › Formulario» (`editor/formfields.ts`),
// con el mismo HTML: fichas en línea `<span data-formfield="text|checkbox|select">` con su
// nombre en `data-name`. El VALOR vive en el propio campo, así que se guarda con el
// documento como cualquier otra edición:
//
//   - texto: su contenido (`<span data-formfield="text" data-name data-placeholder>Ana</span>`);
//   - casilla: `data-checked="1"` o `"0"`;
//   - desplegable: `data-value`, que es una de las `data-options` («a|b|c») o vacío.
//
// Aquí son piezas en línea ATÓMICAS (una runa, como la ficha de la combinación): se pintan
// con su valor y no se escriben por dentro. Su valor se cambia en el modo «Rellenar
// formulario» con [Documento.RellenarCampo], que valida como `coerceFieldValue`.
// [ExtraerRespuestas] es `extractFormAnswers`; los vectores de `testdata/formularios.json`
// los recorren las dos, para que «Ver respuestas» liste lo mismo con el mismo HTML.

// Los tres tipos de campo (`FormFieldType`).
const (
	CampoTexto       = "text"
	CampoCasilla     = "checkbox"
	CampoDesplegable = "select"
)

// CampoFormulario es lo que la vista necesita de un campo para pintarlo y rellenarlo.
type CampoFormulario struct {
	Tipo     string
	Nombre   string   // `data-name`: la clave de la respuesta
	Marcador string   // `data-placeholder`: lo que se ve con el campo vacío
	Opciones []string // las del desplegable, ya saneadas con [ParsearOpciones]
	// Valor es el texto del campo de texto, "1" o "0" en una casilla y la opción elegida
	// (o "") en un desplegable.
	Valor string
	nodo  *xhtml.Node
}

// Marcada dice si la casilla está marcada.
func (c *CampoFormulario) Marcada() bool { return c.Tipo == CampoCasilla && c.Valor == "1" }

// Los colores de editor.css para los campos (`.sytue-page [data-formfield]`): el papel no
// cambia con el tema, así que tampoco ellos.
var (
	// FondoCampo es `rgba(15, 23, 42, 0.03)`, el fondo tenue que los distingue del texto.
	FondoCampo = color.NRGBA{R: 15, G: 23, B: 42, A: 8}
	// BordeCampo es `var(--page-rule)`.
	BordeCampo = TintaLinea
	// TintaMarcador es la del marcador de posición (#6e7379, AA sobre el papel).
	TintaMarcador = tintaHecha
	// TintaFlecha es la del `▾` del desplegable (#475569).
	TintaFlecha = tintaCita
	// TintaMarca es la del `✓` de la casilla: `--sytue-primary-700`.
	TintaMarca = tintaEnlace
	// FondoRellenar y BordeRellenar son los del modo «Rellenar formulario»: el latón al 6 %
	// y al 35 % (`color-mix(in srgb, var(--sytue-primary) N%, transparent)`).
	FondoRellenar = conAlfa(tokens.Color("primary", tokens.Light), 0.06)
	BordeRellenar = conAlfa(tokens.Color("primary", tokens.Light), 0.35)
)

// tipoCampo es el tipo de un campo de formulario, o "" si n no lo es. La web lo lee sin
// distinguir mayúsculas (`extractFormAnswers`).
func tipoCampo(n *xhtml.Node) string {
	if n == nil || n.Type != xhtml.ElementNode || n.DataAtom != atom.Span || n.Namespace != "" || !tieneAtributo(n, "data-formfield") {
		return ""
	}
	switch t := strings.ToLower(atributo(n, "data-formfield")); t {
	case CampoTexto, CampoCasilla, CampoDesplegable:
		return t
	}
	return ""
}

// esCampoFormulario dice si n es un campo de formulario que se entiende. Uno con otro tipo
// (`data-formfield="radio"`) no lo es y deja el documento en solo lectura.
func esCampoFormulario(n *xhtml.Node) bool { return tipoCampo(n) != "" }

// campoDe lee el campo n.
func campoDe(n *xhtml.Node) *CampoFormulario {
	c := &CampoFormulario{Tipo: tipoCampo(n), Nombre: atributo(n, "data-name"), Marcador: atributo(n, "data-placeholder"), nodo: n}
	switch c.Tipo {
	case CampoCasilla:
		c.Valor = "0"
		if atributo(n, "data-checked") == "1" {
			c.Valor = "1"
		}
	case CampoDesplegable:
		c.Valor = atributo(n, "data-value")
		c.Opciones = ParsearOpciones(atributo(n, "data-options"))
	default:
		c.Valor = textoCampo(n)
	}
	return c
}

// ocultosCampo son las etiquetas cuyo contenido no es texto del campo (`HIDDEN`).
var ocultosCampo = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Template: true, atom.Noscript: true,
	atom.Iframe: true, atom.Object: true, atom.Embed: true,
}

// textoCampo es `fieldText`: el texto plano del campo, sin lo oculto y recortado. El `<br>`
// con el que un contenteditable deja un campo vacío no cuenta.
func textoCampo(n *xhtml.Node) string {
	var b strings.Builder
	var ir func(n *xhtml.Node, prof int)
	ir = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			switch {
			case h.Type == xhtml.TextNode:
				b.WriteString(h.Data)
			case h.Type == xhtml.ElementNode && !ocultosCampo[h.DataAtom]:
				ir(h, prof+1)
			}
		}
	}
	ir(n, 0)
	return strings.TrimSpace(b.String())
}

// ParsearOpciones es `parseOptions`: «a | b | c» → [a b c], recortadas, sin vacías ni repetidas.
func ParsearOpciones(crudo string) []string {
	out := []string{}
	for _, parte := range strings.Split(crudo, "|") {
		if o := strings.TrimSpace(parte); o != "" && !slices.Contains(out, o) {
			out = append(out, o)
		}
	}
	return out
}

// reAfirmativo es el de `coerceFieldValue` para una casilla.
var reAfirmativo = regexp.MustCompile(`(?i)^(1|true|s[íi]|x|on|yes)$`)

// ValorCampo es `coerceFieldValue`: el valor que se guarda en un campo de ese tipo, y false
// si no vale para él (un desplegable con algo que no está entre sus opciones).
func ValorCampo(tipo, crudo string, opciones []string) (string, bool) {
	switch tipo {
	case CampoCasilla:
		if reAfirmativo.MatchString(strings.TrimSpace(crudo)) {
			return "1", true
		}
		return "0", true
	case CampoDesplegable:
		v := strings.TrimSpace(crudo)
		if v == "" || slices.Contains(opciones, v) {
			return v, true
		}
		return "", false
	}
	return crudo, true
}

// RellenarCampo guarda en el campo el valor, validado con [ValorCampo], y dice si ha
// cambiado algo. El de texto se queda con el texto en una sola línea, sin formato: lo que
// tuviera dentro (un `<br>` de vacío, una negrita) se sustituye, como al teclear encima en
// la web con todo seleccionado.
func (d *Documento) RellenarCampo(c *CampoFormulario, crudo string) bool {
	if c == nil || c.nodo == nil || !d.contiene(c.nodo) {
		return false
	}
	if c.Tipo == CampoTexto {
		crudo = strings.Join(strings.Fields(crudo), " ")
	}
	v, ok := ValorCampo(c.Tipo, crudo, c.Opciones)
	if !ok {
		return false
	}
	n := c.nodo
	switch c.Tipo {
	case CampoCasilla:
		if atributo(n, "data-checked") == v {
			return false
		}
		ponerAtributo(n, "data-checked", v)
	case CampoDesplegable:
		if atributo(n, "data-value") == v && tieneAtributo(n, "data-value") {
			return false
		}
		ponerAtributo(n, "data-value", v)
	default:
		if textoCampo(n) == v && (v != "" || n.FirstChild == nil) {
			return false
		}
		for n.FirstChild != nil {
			n.RemoveChild(n.FirstChild)
		}
		if v != "" {
			n.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: v})
		}
	}
	c.Valor = v
	return true
}

// CamposFormulario son los campos del documento, en orden de documento.
func (d *Documento) CamposFormulario() []*CampoFormulario {
	var out []*CampoFormulario
	var ir func(n *xhtml.Node, prof int)
	ir = func(n *xhtml.Node, prof int) {
		for h := n.FirstChild; h != nil && prof <= profundidadMax; h = h.NextSibling {
			if h.Type != xhtml.ElementNode || ocultosCampo[h.DataAtom] {
				continue
			}
			if esCampoFormulario(h) {
				out = append(out, campoDe(h))
			}
			ir(h, prof+1)
		}
	}
	ir(d.raiz, 0)
	return out
}

// Respuesta es la de un campo (`FormAnswer`): la casilla responde «Sí» o «No», como la web.
type Respuesta struct {
	Nombre string `json:"name"`
	Tipo   string `json:"type"`
	Valor  string `json:"value"`
}

// respuestaDe es la respuesta de un campo, con el «Sí»/«No» de `extractFormAnswers`: es
// dato del documento (lo que se copia y se exporta), no texto de la interfaz.
func respuestaDe(c *CampoFormulario) Respuesta {
	r := Respuesta{Nombre: c.Nombre, Tipo: c.Tipo, Valor: c.Valor}
	if c.Tipo == CampoCasilla {
		r.Valor = map[bool]string{true: "Sí", false: "No"}[c.Marcada()]
	}
	return r
}

// Respuestas son las de los campos del documento, en orden (lo que lista «Ver respuestas»).
func (d *Documento) Respuestas() []Respuesta {
	cs := d.CamposFormulario()
	out := make([]Respuesta, len(cs))
	for i, c := range cs {
		out[i] = respuestaDe(c)
	}
	return out
}

// ExtraerRespuestas es `extractFormAnswers` sobre un HTML cualquiera: las respuestas de
// sus campos, en orden de documento. Los comentarios, los scripts, los estilos y las
// plantillas no cuentan; los valores son texto plano, ya sin entidades.
func ExtraerRespuestas(fuente string) []Respuesta {
	raiz, err := leer(fuente)
	if err != nil {
		return []Respuesta{}
	}
	out := (&Documento{raiz: raiz}).Respuestas()
	if out == nil {
		return []Respuesta{}
	}
	return out
}

// RespuestasATexto es `answersToText`: «Nombre: valor», una por línea, para copiar.
// `sinNombre` es lo que va en lugar del nombre de un campo que no lo tiene.
func RespuestasATexto(rs []Respuesta, sinNombre string) string {
	lineas := make([]string, len(rs))
	for i, r := range rs {
		nombre := r.Nombre
		if nombre == "" {
			nombre = sinNombre
		}
		lineas[i] = nombre + ": " + r.Valor
	}
	return strings.Join(lineas, "\n")
}

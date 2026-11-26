// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// SANEADO DE TEXTO ENRIQUECIDO
//
// El editor del navegador es un `contenteditable`, así que lo que llega es HTML.
// Ese HTML lo pinta después el navegador de OTRA persona (un cuaderno de
// organización, un enlace público), de modo que saneado en cliente no es saneado:
// se comparte en el servicio y en los dispositivos que procesan datos E2E.
//
// La política es una ALLOWLIST cerrada —etiquetas conocidas, atributos contados—
// y todo lo demás desaparece. Un `<script>` no se escapa: se descarta con su
// contenido, que es lo que evita que reaparezca al copiar y pegar la nota.
//
// Se tokeniza con golang.org/x/net/html, que ya está en el módulo (lo usa el
// unfurl de Rooms): un saneador de allowlist son cuarenta líneas sobre él y no
// justifica traer una dependencia más (CONVENTIONS §3, cadena de suministro).

// allowedTags son las etiquetas que sobreviven al saneado. Es el vocabulario de
// un editor de notas: dar formato, listas, encabezados, cita, código y enlaces.
var allowedTags = map[string]bool{
	"p": true, "br": true, "div": true, "span": true,
	"b": true, "strong": true, "i": true, "em": true, "u": true,
	"s": true, "strike": true, "del": true, "mark": true,
	"code": true, "pre": true, "blockquote": true, "hr": true,
	"ul": true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"a": true,
	// Tablas: una nota de reunión o un comparativo son una tabla, y sin estas seis
	// etiquetas el saneador tiraba la rejilla y dejaba las celdas como texto
	// corrido. El exportador a texto plano ya las contemplaba (`tr` en cortaLinea).
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
	// Tareas: casillas de verificación interactivas en listas de tareas.
	"input": true,
	// Los bloques del editor (ver `bloques` más abajo): la sección que se pliega, la
	// fecha, y subíndice y superíndice del «más formato» de la barra.
	"details": true, "summary": true, "time": true, "sub": true, "sup": true,
	// Una imagen DENTRO del texto, solo de un adjunto de la nota o pequeña y en línea
	// (ver `imagenPermitida`): ni una dirección de fuera ni una del propio servicio.
	"img": true,
}

// LOS BLOQUES DEL EDITOR, compartidos con la web y con Fyne: el HTML que se guarda
// y que las tres partes leen igual. Cada atributo vale SOLO en su etiqueta y SOLO con
// los valores de su lista; cualquier otro se cae, no se corrige.
//
//	panel       <div data-panel="info|nota|exito|aviso|error">…</div>
//	expandir    <details data-expandir [open]><summary>Título</summary>…</details>
//	estado      <span data-estado="gris|azul|verde|amarillo|rojo|morado">TEXTO</span>
//	decisiones  <ul data-tipo="decisiones"><li>…</li></ul>
//	fecha       <time datetime="AAAA-MM-DD">texto</time>
//	código      <pre [data-lineas] [data-ajuste]><code data-lenguaje="go|javascript|…">…</code></pre>
//	tabla       <table [data-numerada]>, <thead> de <th>, <th scope="row">, data-fondo en td/th,
//	            colspan="2…20" y rowspan="2…100" en td/th (celdas combinadas)
//	columnas    <div data-columnas="2|3"><div data-columna>…</div>…</div>
//	imagen      <img src="adjunto:ID" alt="…" [width] [height]> (ver `imagenPermitida`)
//	fichero     <a href="adjunto:ID">nombre</a>
//
// La lista de la web es `BLOQUES_DE_NOTAS` en
// `apps/frontend/packages/ui/src/browser/portapapeles.ts`, y la prueba
// `bloques-saneado.test.ts` de Notes compara las dos: si cambias una, cambia la otra.
var (
	coloresDeBloque = []string{"gris", "azul", "verde", "amarillo", "rojo", "morado"}
	// La lista del editor de Fyne, con su misma grafía: la nota se escribe en las dos.
	lenguajes = []string{
		"texto", "bash", "c", "cpp", "csharp", "css", "go", "html", "java", "javascript",
		"json", "kotlin", "markdown", "php", "python", "ruby", "rust", "sql", "swift",
		"typescript", "xml", "yaml",
	}
	// bloquesEnum: etiqueta → atributo → valores que valen.
	bloquesEnum = map[string]map[string][]string{
		"div":  {"data-panel": {"info", "nota", "exito", "aviso", "error"}, "data-columnas": {"2", "3"}},
		"span": {"data-estado": coloresDeBloque},
		"ul":   {"data-tipo": {"decisiones"}},
		"code": {"data-lenguaje": lenguajes},
		"th":   {"scope": {"row"}, "data-fondo": coloresDeBloque},
		"td":   {"data-fondo": coloresDeBloque},
	}
	// bloquesSinValor: etiqueta → atributos que se ponen sin valor (presentes o no).
	bloquesSinValor = map[string][]string{
		"details": {"data-expandir", "open"},
		"div":     {"data-columna"},
		"pre":     {"data-lineas", "data-ajuste"},
		"table":   {"data-numerada"},
	}
	// bloquesNumericos: etiqueta → atributo → el número más alto que vale. Valen de 2 al
	// tope, en decimal y sin ceros delante (un 1 es no ponerlo): así la vuelta sale igual
	// byte a byte, y una celda no puede pedir un millón de columnas.
	bloquesNumericos = map[string]map[string]int{
		"td": {"colspan": 20, "rowspan": 100},
		"th": {"colspan": 20, "rowspan": 100},
	}
)

// LAS IMÁGENES Y LOS FICHEROS DEL TEXTO. Se guardan como los demás adjuntos de la nota
// (en `data.attachments`, que topa el tamaño de `data`) y el cuerpo los nombra por su id
// con el esquema `adjunto:`. Es lo único que vale en un `src`, además de una imagen
// pequeña en línea (`data:image/png|jpeg|gif|webp;base64,…`, hasta MaxImagenEnLinea):
//
//   - una dirección de fuera (`https://…`), porque la nota la pinta el navegador de OTRA
//     persona (un cuaderno compartido, un enlace público) y pedir la imagen le dice a
//     ese servidor quién la ha abierto y cuándo;
//   - una del propio servicio (`/api/…`), porque el navegador la pide CON la sesión de
//     quien la ve: un `<img src="/api/…">` es una petición GET hecha en su nombre;
//   - un `data:` de otro tipo (`image/svg+xml`, `text/html`) o sin base64, ni uno mayor
//     que el tope: un SVG puede traer guiones, y la nota no es sitio para megas en línea.
//
// De `<img>` solo quedan `src`, `alt` (texto, hasta MaxAltImagen letras) y `width` y
// `height` (enteros de 1 a MaxLadoImagen). Una imagen sin `src` válido no sale.

// Los topes de una imagen del texto.
const (
	MaxImagenEnLinea = 64 << 10
	MaxAltImagen     = 300
	MaxLadoImagen    = 4096
)

var (
	// reAdjunto es `adjunto:` con un id de adjunto: el de la web y Fyne
	// (`Date.now()-aleatorio`), y nada que pueda salirse de una URL.
	reAdjunto = regexp.MustCompile(`^adjunto:[A-Za-z0-9_-]{1,64}$`)
	// reImagenEnLinea es una imagen en línea de las cuatro que pinta cualquiera.
	reImagenEnLinea = regexp.MustCompile(`^data:image/(?:png|jpeg|gif|webp);base64,[A-Za-z0-9+/]+={0,2}$`)
)

// EsAdjunto dice si `v` nombra un adjunto de la nota (`adjunto:ID`).
func EsAdjunto(v string) bool { return reAdjunto.MatchString(v) }

// imagenPermitida dice si un `src` vale en una nota.
func imagenPermitida(v string) bool {
	return EsAdjunto(v) || len(v) <= MaxImagenEnLinea && reImagenEnLinea.MatchString(v)
}

// numeroDeBloque devuelve el atributo numérico ya serializado si vale, o "".
func numeroDeBloque(tag, clave, valor string) (string, bool) {
	tope, ok := bloquesNumericos[tag][clave]
	if !ok {
		return "", false
	}
	n, err := strconv.Atoi(valor)
	if err != nil || n < 2 || n > tope || strconv.Itoa(n) != valor {
		return "", true
	}
	return ` ` + clave + `="` + valor + `"`, true
}

// atributoDeBloque devuelve el atributo ya serializado si vale para esa etiqueta, o "".
func atributoDeBloque(tag, clave, valor string) string {
	if n, es := numeroDeBloque(tag, clave, valor); es {
		return n
	}
	if valores, ok := bloquesEnum[tag][clave]; ok {
		if slices.Contains(valores, valor) {
			return ` ` + clave + `="` + valor + `"`
		}
		return ""
	}
	// Uno sin valor solo vale vacío (o con su propio nombre, que es lo mismo en HTML).
	if slices.Contains(bloquesSinValor[tag], clave) && (valor == "" || valor == clave) {
		return ` ` + clave
	}
	if tag == "time" && clave == "datetime" && fechaISO(valor) {
		return ` datetime="` + valor + `"`
	}
	return ""
}

// fechaISO dice si `v` es un día de verdad escrito AAAA-MM-DD: el formato del
// `<time>` del editor, sin hora ni zona.
func fechaISO(v string) bool {
	if len(v) != 10 {
		return false
	}
	t, err := time.Parse("2006-01-02", v)
	return err == nil && t.Format("2006-01-02") == v
}

// voidTags no llevan cierre.
var voidTags = map[string]bool{"br": true, "hr": true, "input": true, "img": true}

// droppedSubtrees son las etiquetas cuyo CONTENIDO también se tira. Escapar el
// texto de un <script> lo dejaría visible como código en la nota; borrarlo entero
// es lo que espera quien pega desde otra página.
var droppedSubtrees = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true,
	"embed": true, "template": true, "noscript": true, "svg": true, "math": true,
}

// saneador lleva el estado del recorrido: lo emitido, la pila de etiquetas
// abiertas y la profundidad dentro de un subárbol descartado.
//
// Existe para que los dos casos con miga —abrir y cerrar— salgan del `switch` y
// se lean por separado. Estaban en línea y el bucle acumulaba una complejidad
// cognitiva de 49, casi el doble del límite: la parte de cerrar, que es la que
// tiene la búsqueda hacia atrás en la pila, quedaba enterrada bajo la de abrir.
type saneador struct {
	out      strings.Builder
	abiertas []string
	saltando int
	// ficheros deja también los enlaces a ficheros de este equipo
	// (SanitizeHTMLConFicheros).
	ficheros bool
}

// abre atiende una etiqueta de apertura (o autocerrada).
func (s *saneador) abre(z *html.Tokenizer, autocerrada bool) {
	name, hasAttr := z.TagName()
	tag := string(name)
	if droppedSubtrees[tag] {
		// Una autocerrada no abre subárbol; una normal sí.
		if !autocerrada {
			s.saltando++
		}
		return
	}
	if s.saltando > 0 || !allowedTags[tag] {
		return
	}
	attrs := atributosPermitidos(z, tag, hasAttr, s.ficheros)
	if tag == "img" && !strings.HasPrefix(attrs, ` src="`) {
		return // una imagen sin `src` que valga no se pinta: fuera entera
	}
	s.out.WriteString("<" + tag)
	s.out.WriteString(attrs)
	s.out.WriteString(">")
	if !voidTags[tag] && !autocerrada {
		s.abiertas = append(s.abiertas, tag)
	}
}

// cierra atiende una etiqueta de cierre.
func (s *saneador) cierra(z *html.Tokenizer) {
	name, _ := z.TagName()
	tag := string(name)
	if droppedSubtrees[tag] {
		if s.saltando > 0 {
			s.saltando--
		}
		return
	}
	if s.saltando > 0 || !allowedTags[tag] || voidTags[tag] {
		return
	}
	// Cierra hasta la apertura correspondiente. Un cierre HUÉRFANO —</p> sin
	// <p>— se descarta: emitirlo dejaría el HTML desbalanceado y el navegador
	// lo «arregla» tragándose lo que viene detrás.
	idx := -1
	for i := len(s.abiertas) - 1; i >= 0; i-- {
		if s.abiertas[i] == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	for i := len(s.abiertas) - 1; i >= idx; i-- {
		s.out.WriteString("</" + s.abiertas[i] + ">")
	}
	s.abiertas = s.abiertas[:idx]
}

// termina cierra lo que quedara abierto y devuelve el resultado.
func (s *saneador) termina() string {
	for i := len(s.abiertas) - 1; i >= 0; i-- {
		s.out.WriteString("</" + s.abiertas[i] + ">")
	}
	return strings.TrimSpace(s.out.String())
}

// SanitizeHTML devuelve el HTML de entrada reducido a la allowlist. Nunca
// devuelve error: lo que no está permitido, no sale.
func SanitizeHTML(in string) string { return sanear(in, false) }

// SanitizeHTMLConFicheros es SanitizeHTML que deja además los enlaces a ficheros de
// este equipo (`file:///…`, EnlaceAFichero). Solo para la edición local (Sytue Office,
// N-07), donde la nota no la pinta el navegador de nadie más: en el servicio y en la
// edición conectada, un `file:` sigue cayéndose como cualquier otro esquema.
func SanitizeHTMLConFicheros(in string) string { return sanear(in, true) }

func sanear(in string, ficheros bool) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	s := saneador{ficheros: ficheros}
	z := html.NewTokenizer(strings.NewReader(in))
	for {
		switch tt := z.Next(); tt {
		case html.ErrorToken:
			return s.termina()
		case html.TextToken:
			if s.saltando == 0 {
				s.out.WriteString(html.EscapeString(string(z.Text())))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			s.abre(z, tt == html.SelfClosingTagToken)
		case html.EndTagToken:
			s.cierra(z)
		default:
			// Comentarios y doctype: fuera sin más.
		}
	}
}

// atributosPermitidos devuelve los atributos que sobreviven para esa etiqueta, ya
// serializados y escapados. La lista es corta a propósito: `href` en enlaces (solo
// http/https/mailto, y `file:` con ficheros) y `style` reducido a color de texto y de
// fondo.
func atributosPermitidos(z *html.Tokenizer, tag string, hasAttr, ficheros bool) string {
	if !hasAttr {
		return ""
	}
	if tag == "img" {
		return atributosDeImagen(z)
	}
	var b strings.Builder
	// Un atributo repetido sale una vez, el primero, que es el que lee el navegador.
	vistos := map[string]bool{}
	for {
		k, v, more := z.TagAttr()
		clave, valor := string(k), string(v)
		repetido := vistos[clave]
		vistos[clave] = true
		switch {
		case repetido:
		case clave == "href" && tag == "a":
			b.WriteString(enlacePermitido(valor, ficheros))
		case clave == "type" && tag == "input":
			if strings.EqualFold(valor, "checkbox") {
				b.WriteString(` type="checkbox"`)
			}
		case (clave == "checked" || clave == "disabled") && tag == "input":
			b.WriteString(` ` + html.EscapeString(clave))
		case clave == "class":
			if clasePermitida(tag, valor) {
				b.WriteString(` class="` + html.EscapeString(valor) + `"`)
			}
		case clave == "style":
			if s := estiloPermitido(valor); s != "" {
				b.WriteString(` style="` + html.EscapeString(s) + `"`)
			}
		default:
			// Los valores de los bloques salen de listas cerradas, de una fecha
			// comprobada y de enteros: no hay nada que escapar.
			b.WriteString(atributoDeBloque(tag, clave, valor))
		}
		if !more {
			return b.String()
		}
	}
}

// EnlaceAFichero dice si raw es un enlace a un fichero de ESTE equipo: `file:` sin
// máquina (o `localhost`), sin usuario, consulta ni fragmento y con una ruta absoluta
// (`file:///home/ana/x.pdf`, `file:///C:/Users/ana/x.pdf`). `url.Parse` ya rechaza los
// caracteres de control.
func EnlaceAFichero(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "file") || u.Opaque != "" || u.User != nil {
		return false
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return false
	}
	return strings.HasPrefix(u.Path, "/") && len(u.Path) > 1 && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// enlacePermitido es el `href` de un enlace si vale —una dirección web, un correo, un
// adjunto de la nota o, con ficheros, un fichero de este equipo—, ya serializado, o "".
func enlacePermitido(valor string, ficheros bool) string {
	if ficheros && EnlaceAFichero(valor) {
		// Un fichero de este equipo lo abre su app, no una pestaña: sin `target`.
		return ` href="` + html.EscapeString(valor) + `"`
	}
	if !ValidWebURL(valor) && !strings.HasPrefix(valor, "mailto:") && !EsAdjunto(valor) {
		return ""
	}
	// Un enlace de una nota ajena abre en otra pestaña SIN darle acceso a esta:
	// `noopener` cierra window.opener, `noreferrer` no filtra de dónde viene.
	return ` href="` + html.EscapeString(valor) + `" target="_blank" rel="noopener noreferrer"`
}

// atributosDeImagen son los de un `<img>`: `src` el primero (si no vale, ninguno), y
// detrás `alt`, `width` y `height`, en ese orden y escapados. Todo lo demás —`onerror`,
// `srcset`, `style`, `loading`— se cae.
func atributosDeImagen(z *html.Tokenizer) string {
	valores := map[string]string{}
	for {
		k, v, more := z.TagAttr()
		if _, visto := valores[string(k)]; !visto {
			valores[string(k)] = string(v)
		}
		if !more {
			break
		}
	}
	src, ok := valores["src"]
	if !ok || !imagenPermitida(src) {
		return ""
	}
	b := ` src="` + html.EscapeString(src) + `"`
	if alt, ok := valores["alt"]; ok && utf8.RuneCountInString(alt) <= MaxAltImagen {
		b += ` alt="` + html.EscapeString(alt) + `"`
	}
	for _, lado := range []string{"width", "height"} {
		if n, err := strconv.Atoi(valores[lado]); err == nil && n >= 1 && n <= MaxLadoImagen && strconv.Itoa(n) == valores[lado] {
			b += ` ` + lado + `="` + valores[lado] + `"`
		}
	}
	return b
}

// clasePermitida dice si una clase sobrevive: la fila y la casilla de una tarea, y
// `plegada` en un `<li>`, que es la lista plegada: se guarda con la nota y
// la lectura esconde su sublista.
func clasePermitida(tag, valor string) bool {
	return valor == "task-row" || valor == "task-checkbox" || (valor == "plegada" && tag == "li")
}

// estiloPermitido reduce un `style` a las declaraciones que un editor de
// notas necesita —color y fondo, solo con colores #RRGGBB, alineación del
// bloque, solo con las cuatro palabras, y cursor interactivo—. Todo lo demás (`url(...)`, `position`,
// `expression`) se cae por no estar en la lista, no por filtrarse.
func estiloPermitido(raw string) string {
	var partes []string
	for _, decl := range strings.Split(raw, ";") {
		prop, valor, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		valor = strings.TrimSpace(valor)
		if (prop == "color" || prop == "background-color") && ValidHexColor(valor) {
			partes = append(partes, prop+":"+valor)
		}
		if prop == "text-align" && alineaciones[strings.ToLower(valor)] {
			partes = append(partes, prop+":"+strings.ToLower(valor))
		}
		if prop == "cursor" && valor == "pointer" {
			partes = append(partes, "cursor:pointer")
		}
	}
	return strings.Join(partes, ";")
}

// alineaciones son los únicos valores que admite `text-align`. La barra del editor
// ofrece justo estos cuatro; cualquier otra cosa en esa propiedad no viene de ella.
var alineaciones = map[string]bool{"left": true, "center": true, "right": true, "justify": true}

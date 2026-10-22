// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Formato de exportación.
//
// EL NATIVO ES HTML, y es el que sale cuando no se pide ninguno. Es el único que
// hace las dos cosas a la vez: se abre en cualquier navegador y se ve el cuaderno
// COMO ES —con su disposición, sus colores y sus enlaces—, y lleva dentro el
// volcado íntegro en un `script` que ningún navegador pinta, así que del mismo
// fichero se puede volver. Es también lo que exporta BasKet Note Pads, la
// inspiración de este producto.
//
// El resto son salidas hacia fuera: Markdown y texto para leerlo en otro
// programa, PDF para imprimirlo, `.onex` para dárselo a una máquina.
//
// Las extensiones que la gente escribe no son tres formatos: `.md`, `.MD` y
// `.markdown` son el MISMO Markdown con otro nombre, y tratarlas como cosas
// distintas obligaría a mantener tres generadores que se separan. Lo que cambia
// es la extensión del fichero, no el contenido.
type FormatoExport string

const (
	FormatoMarkdown FormatoExport = "md"
	// FormatoMarkdownLargo es el mismo Markdown con la extensión completa. Existe
	// porque hay quien la escribe así y quien tiene el editor asociado a ella.
	FormatoMarkdownLargo FormatoExport = "markdown"
	FormatoTextoPlano    FormatoExport = "txt"
	// FormatoHTML es EL FORMATO NATIVO: el volcado completo, y el único que se lee
	// y se restaura con el mismo fichero.
	//
	// Markdown y texto plano se llevan el contenido y pierden la posición en el
	// lienzo, el ancho y los estados. Y hay algo que no pueden llevarse de
	// ninguna manera: una nota CIFRADA, porque el servidor no tiene la clave y el
	// criptograma en un fichero de lectura solo son bytes que nadie abre.
	FormatoHTML FormatoExport = "html"
	// FormatoNativo es el MISMO volcado íntegro que lleva el HTML dentro, pero sin
	// la página alrededor: JSON pelado, para un script o una copia de seguridad.
	// Es el único formato que se puede volver a IMPORTAR aquí.
	FormatoNativo FormatoExport = "onex"
	// FormatoPDF es el extremo opuesto: el que se imprime o se archiva, del que no
	// se vuelve. No lleva volcado embebido —un PDF no tiene dónde— y por eso no
	// sustituye a ninguno de los otros.
	FormatoPDF FormatoExport = "pdf"
)

// FormatoDe interpreta lo que pida quien exporta, sin distinguir mayúsculas: la
// diferencia entre `.MD` y `.md` es de teclado, no de formato.
func FormatoDe(v string) (FormatoExport, bool) {
	switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(v, "."))) {
	// El vacío cae al NATIVO, no a Markdown: quien no elige formato quiere su
	// cuaderno, y el único que se lo lleva entero es el HTML.
	case "", "html", "htm":
		return FormatoHTML, true
	case "md":
		return FormatoMarkdown, true
	case "markdown":
		return FormatoMarkdownLargo, true
	case "txt", "text":
		return FormatoTextoPlano, true
	case "onex":
		return FormatoNativo, true
	case "pdf":
		return FormatoPDF, true
	}
	return "", false
}

// Extension y TipoMIME describen el fichero que se entrega.
func (f FormatoExport) Extension() string {
	switch f {
	case FormatoMarkdown:
		return ".md"
	case FormatoMarkdownLargo:
		return ".markdown"
	case FormatoTextoPlano:
		return ".txt"
	case FormatoNativo:
		return ".onex"
	case FormatoPDF:
		return ".pdf"
	case FormatoHTML:
	}
	// El respaldo es el NATIVO: si algún día se añade un formato y se olvida un
	// caso aquí, el fichero saldrá con la extensión del que lo lleva todo, no con
	// la de uno que pierde la mitad del cuaderno.
	return ".html"
}

func (f FormatoExport) TipoMIME() string {
	switch f {
	case FormatoMarkdown, FormatoMarkdownLargo:
		return "text/markdown; charset=utf-8"
	case FormatoTextoPlano:
		return "text/plain; charset=utf-8"
	case FormatoPDF:
		return "application/pdf"
	case FormatoNativo:
		// Tipo propio y no `application/json` a secas: así el navegador y el
		// escritorio pueden asociarlo a Notepads en vez de abrirlo con lo que abra
		// cualquier JSON. El sufijo `+json` dice que por dentro SÍ es JSON, que es
		// lo que permite leerlo con herramientas normales.
		return "application/vnd.sytue.notepads+json"
	case FormatoHTML:
	}
	return "text/html; charset=utf-8"
}

// PuedeVerseEnLinea dice qué formatos se pueden servir `inline`, es decir, que el
// navegador los abra en una pestaña en vez de bajarlos.
//
// EL HTML SE QUEDA FUERA aunque sea el que mejor se vería. Lleva dentro contenido
// del cuaderno, y servirlo `inline` desde el origen de la API le daría a ese
// contenido el mismo origen que la sesión de quien lo abre. Se descarga y se abre
// desde el disco, donde no es nadie.
//
// El PDF sí, porque lo pinta el visor del navegador —un proceso aparte, sin
// acceso a la página— y porque es exactamente para lo que se pidió: mirarlo.
// Markdown, texto y `.onex` también: son texto y su tipo MIME no ejecuta nada.
func (f FormatoExport) PuedeVerseEnLinea() bool { return f != FormatoHTML }

// Exportar vuelca el cuaderno en el formato pedido.
//
// Devuelve BYTES y no texto porque desde que existe el PDF no todos los formatos
// son texto, y un `string` con un PDF dentro es una invitación a que alguien lo
// recorra por runas o lo pase por un normalizador.
func Exportar(nb Notebook, notas []Note, f FormatoExport) []byte {
	switch f {
	case FormatoPDF:
		return NotebookAPDF(nb, notas)
	case FormatoNativo:
		return []byte(NotebookANativo(nb, notas))
	case FormatoTextoPlano:
		return []byte(NotebookATextoPlano(nb, notas))
	case FormatoMarkdown, FormatoMarkdownLargo:
		return []byte(NotebookAMarkdown(nb, notas))
	case FormatoHTML:
	}
	return []byte(NotebookAHTML(nb, notas))
}

// LLEVARSE LAS NOTAS ES UN DERECHO, no una función más.
//
// Sytue Notepads era el único servicio de la suite con contenido de usuario sin
// forma de exportarlo: Contactos tiene vCard y CSV, Documentos doce formatos,
// Archivos la descarga y Correo el `.eml` original. Aquí no había nada, y sin eso
// el contenido solo se puede sacar a mano, nota a nota, desde la pantalla.
//
// Para una suite que se vende como soberana eso no es un hueco de comodidad: el
// artículo 20 del RGPD da derecho a recibir los datos «en un formato estructurado,
// de uso común y lectura mecánica». Los seis lo son; el HTML nativo cumple además
// lo que el artículo no exige pero es lo que la gente quiere: que al abrirlo se
// vea el cuaderno, sin instalar nada.

// NotebookAMarkdown vuelca un cuaderno y sus notas a Markdown.
//
// EL ORDEN ES EL DE LA PANTALLA —columna y luego posición— y no el de la base de
// datos. Quien exporta reconoce lo que ve; un volcado por fecha de creación sería
// el mismo contenido en un orden que no ha visto nunca.
func NotebookAMarkdown(nb Notebook, notas []Note) string {
	var b strings.Builder
	b.WriteString("# " + tituloDe(nb.Title, "Cuaderno sin título") + "\n\n")

	for _, n := range enOrdenDePantalla(notas) {
		b.WriteString("## " + tituloDe(n.Title, "Nota sin título") + "\n\n")
		b.WriteString(cuerpoDeNota(n))
		if etiquetas := etiquetasDe(n); etiquetas != "" {
			b.WriteString(etiquetas + "\n")
		}
		if enlaces := enlacesDe(n); enlaces != "" {
			b.WriteString(enlaces)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TituloDeNota es el rótulo con el que se guarda una nota exportada sola: su
// título, o «nota» si no tiene (una nota sin título es de lo más normal).
func TituloDeNota(n Note) string {
	return tituloDe(n.Title, "nota")
}

func tituloDe(v, porDefecto string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return porDefecto
}

// cuerpoDeNota devuelve el contenido de una nota listo para Markdown.
func cuerpoDeNota(n Note) string {
	// UNA NOTA CIFRADA NO SE EXPORTA, Y SE DICE. El criptograma lo abre la clave
	// del navegador de su dueño, que el servidor no tiene ni debe tener: ese es
	// justo el punto del cifrado extremo a extremo. Omitirla en silencio sería
	// peor que no exportar —quien recibe el fichero creería que ya está todo—, y
	// volcar el criptograma sería dar bytes que nadie va a poder abrir.
	if n.ContentCipher != "" {
		return "> Nota cifrada de extremo a extremo. Su contenido solo puede leerlo\n" +
			"> quien tiene la clave, así que no viaja en esta exportación.\n\n"
	}
	if c := SanitizeHTML(n.Content); c != "" {
		// El contenido es HTML, se sanea al salir (allowlist cerrada, ver html.go; lo
		// guardado antes de sanear todos los tipos puede no estarlo) y se vuelca sin
		// convertir: Markdown admite HTML en línea, así que se abre en
		// cualquier editor. Convertirlo a Markdown puro exigiría un conversor
		// —otra dependencia, o cien líneas propias— para un resultado que se ve
		// igual. Si algún día hace falta Markdown estricto, este es el sitio.
		return c + "\n\n"
	}
	return "_(vacía)_\n\n"
}

// etiquetasDe pinta los estados aplicados a la nota, que es lo que en la interfaz
// le da su color y su significado.
func etiquetasDe(n Note) string {
	if len(n.States) == 0 {
		return ""
	}
	// «Prioridad: Alta» y no «Alta» a secas: el estado solo significa algo junto a
	// su etiqueta, y en un volcado no hay color ni columna que lo desambigüe. Dos
	// etiquetas distintas pueden tener un estado que se llame igual.
	nombres := make([]string, 0, len(n.States))
	for _, s := range n.States {
		estado := strings.TrimSpace(s.Name)
		if estado == "" {
			continue
		}
		if tag := strings.TrimSpace(s.TagName); tag != "" {
			estado = tag + ": " + estado
		}
		nombres = append(nombres, estado)
	}
	if len(nombres) == 0 {
		return ""
	}
	return "**Etiquetas:** " + strings.Join(nombres, ", ") + "\n"
}

// enlacesDe lista lo que la nota apunta en el resto de la suite.
//
// Se exporta el DESTINO y no solo su rótulo: un enlace sin identificador es una
// palabra suelta, y lo que hace útil el volcado es poder volver a lo que
// señalaba.
func enlacesDe(n Note) string {
	if len(n.Links) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**Enlaces:**\n")
	for _, l := range n.Links {
		rotulo := strings.TrimSpace(l.Label)
		if rotulo == "" {
			rotulo = l.TargetID
		}
		if u := strings.TrimSpace(l.URL); u != "" {
			fmt.Fprintf(&b, "- [%s](%s)\n", rotulo, u)
			continue
		}
		fmt.Fprintf(&b, "- %s (%s: %s)\n", rotulo, l.TargetKind, l.TargetID)
	}
	return b.String()
}

// NombreDeFicheroExportado convierte el título del cuaderno en un nombre usable.
//
// Se limpia aquí y no se confía en el título: va dentro de una cabecera
// `Content-Disposition`, donde una comilla o un salto de línea no son un nombre
// feo sino una cabecera partida en dos.
func NombreDeFicheroExportado(titulo string, f FormatoExport) string {
	limpio := strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f: // controles, incluidos CR y LF
			return -1
		case r == '"' || r == '\\' || r == '/' || r == ':':
			return '-'
		}
		return r
	}, strings.TrimSpace(titulo))
	limpio = strings.TrimSpace(limpio)
	if limpio == "" {
		limpio = "cuaderno"
	}
	// Los sistemas de ficheros cortan por bytes, no por caracteres: se recorta con
	// margen para que un título largo en cirílico o con emoji no llegue al límite.
	if len(limpio) > 120 {
		limpio = strings.ToValidUTF8(limpio[:120], "")
	}
	return limpio + f.Extension()
}

// NotebookATextoPlano vuelca el cuaderno sin marcado: lo que se lee en un editor
// que no entiende Markdown, en un correo, o al pegarlo en cualquier sitio.
//
// NO es el Markdown con los asteriscos quitados. El contenido de una nota es
// HTML, así que hay que RECORRERLO: un `</p>` es un párrafo nuevo y un `<li>` es
// una línea, y con un simple borrado de etiquetas todo el cuaderno saldría como
// un solo párrafo corrido —que es exactamente lo que hace inútil un volcado—.
func NotebookATextoPlano(nb Notebook, notas []Note) string {
	var b strings.Builder
	b.WriteString(tituloDe(nb.Title, "Cuaderno sin título") + "\n")
	b.WriteString(strings.Repeat("=", len([]rune(tituloDe(nb.Title, "Cuaderno sin título")))) + "\n\n")

	for _, n := range enOrdenDePantalla(notas) {
		b.WriteString(tituloDe(n.Title, "Nota sin título") + "\n")
		b.WriteString(strings.Repeat("-", len([]rune(tituloDe(n.Title, "Nota sin título")))) + "\n\n")

		switch {
		case n.ContentCipher != "":
			b.WriteString("[Nota cifrada de extremo a extremo: su contenido solo puede leerlo\n" +
				"quien tiene la clave, así que no viaja en esta exportación.]\n\n")
		case strings.TrimSpace(n.Content) != "":
			b.WriteString(TextoDeHTML(n.Content) + "\n\n")
		default:
			b.WriteString("(vacía)\n\n")
		}

		if e := etiquetasDe(n); e != "" {
			b.WriteString(strings.ReplaceAll(e, "**", ""))
		}
		if l := enlacesDe(n); l != "" {
			// Los enlaces se escriben en Markdown; aquí se dejan como «rótulo (URL)»,
			// que es lo que se entiende sin un lector que resuelva corchetes.
			b.WriteString(markdownAEnlacesLlanos(strings.ReplaceAll(l, "**", "")))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TextoDeHTML reduce el HTML de una nota a texto legible, conservando los cortes
// de línea que da la estructura.
//
// Se tokeniza con el mismo `x/net/html` que ya usa el saneador: la alternativa
// —una expresión regular que borre `<...>`— junta palabras que estaban en
// párrafos distintos y deja las entidades (`&amp;`) sin resolver.
func TextoDeHTML(in string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(in))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(colapsarLineas(b.String()))
		case html.TextToken:
			b.WriteString(string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			if cortaLinea[string(name)] {
				b.WriteString("\n")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if separaCelda[string(name)] {
				b.WriteString("\t")
			}
			if cierraParrafo[string(name)] {
				b.WriteString("\n")
			}
		case html.CommentToken, html.DoctypeToken:
			// No aportan texto legible. Se nombran en vez de caer en un `default`
			// para que añadir un tipo de token nuevo sea un fallo de compilación y
			// no una omisión silenciosa.
		}
	}
}

// Etiquetas que empiezan una línea nueva al ABRIRSE y al CERRARSE. `br` y `hr`
// no tienen cierre, y `li` necesita el corte antes de su texto.
var cortaLinea = map[string]bool{"br": true, "hr": true, "li": true, "tr": true}

// Celdas: en texto plano se separan con tabulador. Sin separador, «12 34» de dos
// celdas se lee como un solo número.
var separaCelda = map[string]bool{"td": true, "th": true}

var cierraParrafo = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true, "pre": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
	// Una tabla entera es un bloque; sus filas ya cortan línea (cortaLinea).
	"table": true,
}

// colapsarLineas deja como mucho una línea en blanco seguida: el HTML de un
// editor trae `<p></p>` vacíos y anidamientos que, contados uno a uno, abrirían
// media pantalla de hueco entre dos frases.
func colapsarLineas(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// markdownAEnlacesLlanos convierte `- [rótulo](url)` en `- rótulo (url)`.
func markdownAEnlacesLlanos(s string) string {
	var b strings.Builder
	for _, linea := range strings.Split(s, "\n") {
		i, j := strings.Index(linea, "]("), strings.LastIndex(linea, ")")
		if k := strings.Index(linea, "["); k >= 0 && i > k && j > i {
			b.WriteString(linea[:k] + linea[k+1:i] + " (" + linea[i+2:j] + ")\n")
			continue
		}
		b.WriteString(linea + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// enOrdenDePantalla ordena por columna y posición, que es como se ven.
func enOrdenDePantalla(notas []Note) []Note {
	out := make([]Note, len(notas))
	copy(out, notas)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Col != out[j].Col {
			return out[i].Col < out[j].Col
		}
		return out[i].Position < out[j].Position
	})
	return out
}

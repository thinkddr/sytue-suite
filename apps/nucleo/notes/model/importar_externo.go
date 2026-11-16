// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"html"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// IMPORTAR DESDE OTROS PROGRAMAS: Markdown suelto, un ZIP de Markdown (una bóveda
// de Obsidian, la exportación de Bear, de Apple Notas o de Notion) y Evernote
// (.enex).
//
// La portabilidad que da el `.onex` es de salida; para quien llega a la suite la
// barrera es la contraria: años de notas en otro sitio. Sin esto había que
// copiarlas a mano una a una, y eso en la práctica significa no venirse.
//
// Todo se traduce a lo mismo que da `LeerNativoTransferencia` —un cuaderno y sus notas— para
// que la vista previa y la importación sigan siendo UN solo camino: el mismo
// `Normalize`, el mismo saneado, el mismo recuento de descartadas. Aquí solo se
// convierte; lo que entra lo decide el saneador, igual que con un `.onex`.
//
// Sin dependencias: la biblioteca estándar lee ZIP y XML, y el Markdown que sale
// de estos programas es el común (CommonMark con las extensiones de GFM y alguna
// de Obsidian), que se cubre con un conversor corto.

// Topes del ZIP. El cuerpo ya llega cortado a 32 MiB, pero un ZIP comprimido
// descomprime a mucho más —la «bomba ZIP»—: se mide lo DESCOMPRIMIDO, no el
// fichero, y se para en cuanto se pasa.
const (
	maxEntradasZip      = 5000
	maxDescomprimidoZip = 128 << 20
)

// LeerImportable reconoce el formato del fichero por su contenido —no por la
// extensión, que el navegador a veces no manda y que OneNote comparte con el
// `.onex`— y lo traduce a un cuaderno y sus notas.
//
// `nombre` es el nombre del fichero tal como lo eligió quien importa: da título
// al cuaderno (Evernote exporta «Recetas.enex» por cuaderno) y a la nota de un
// Markdown suelto. Puede venir vacío.
func LeerImportable(datos []byte, nombre string) (Notebook, []Note, error) {
	base := strings.TrimSpace(path.Base(strings.ReplaceAll(nombre, `\`, "/")))
	if base == "." || base == "/" {
		base = ""
	}
	ext := strings.ToLower(path.Ext(base))
	titulo := strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base)))

	switch {
	case bytes.HasPrefix(datos, []byte("PK\x03\x04")):
		return leerZipMarkdown(datos, tituloONombre(titulo, "Markdown"))
	case esENEX(datos):
		return leerENEX(datos, tituloONombre(titulo, "Evernote"))
	case esMarkdown(ext) && utf8.Valid(datos):
		t := tituloONombre(titulo, "Markdown")
		nota := notaDeMarkdown(string(datos), t, 0)
		return cuadernoImportado(t), []Note{nota}, nil
	}
	// El `.onex` pasa por la lectura estricta de transferencia (claves duplicadas,
	// campos desconocidos en un cuaderno cifrado, copia cifrada coherente): lo que
	// valida el dispositivo es exactamente lo que interpreta el servicio.
	return LeerNativoTransferencia(datos)
}

func tituloONombre(titulo, porDefecto string) string {
	if titulo == "" {
		return porDefecto
	}
	return recortar(titulo, maxNotebookTitle)
}

func recortar(s string, tope int) string {
	if utf8.RuneCountInString(s) <= tope {
		return s
	}
	return string([]rune(s)[:tope])
}

func esMarkdown(ext string) bool {
	return ext == ".md" || ext == ".markdown" || ext == ".txt"
}

func cuadernoImportado(titulo string) Notebook {
	return Notebook{Title: titulo, Layout: LayoutTree, Visibility: VisPersonal}
}

// --- ZIP de Markdown -------------------------------------------------------

func leerZipMarkdown(datos []byte, titulo string) (Notebook, []Note, error) {
	zr, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil || len(zr.File) > maxEntradasZip {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	ficheros := make([]*zip.File, 0, len(zr.File))
	for _, f := range zr.File {
		nombre := f.Name
		// Lo que no es una nota se deja fuera: carpetas, adjuntos, la
		// configuración de Obsidian (`.obsidian/`), la basura de macOS
		// (`__MACOSX/`, `._x.md`) y cualquier fichero oculto.
		if f.FileInfo().IsDir() || !esMarkdown(strings.ToLower(path.Ext(nombre))) {
			continue
		}
		oculto := false
		for _, parte := range strings.Split(nombre, "/") {
			if strings.HasPrefix(parte, ".") || parte == "__MACOSX" {
				oculto = true
				break
			}
		}
		if !oculto {
			ficheros = append(ficheros, f)
		}
	}
	if len(ficheros) == 0 {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	// El orden del ZIP es el de quien lo comprimió; el alfabético por ruta es el
	// que se ve en la carpeta, y deja juntas las notas de cada subcarpeta.
	sort.Slice(ficheros, func(i, j int) bool { return ficheros[i].Name < ficheros[j].Name })

	var total int64
	notas := make([]Note, 0, len(ficheros))
	for _, f := range ficheros {
		rc, err := f.Open()
		if err != nil {
			return Notebook{}, nil, ErrFormatoNoReconocido
		}
		// Se lee con tope aunque la cabecera diga el tamaño: la cabecera la
		// escribe quien fabrica el ZIP, y puede mentir.
		restante := maxDescomprimidoZip - total
		cuerpo, err := io.ReadAll(io.LimitReader(rc, restante+1))
		rc.Close()
		if err != nil {
			return Notebook{}, nil, ErrFormatoNoReconocido
		}
		total += int64(len(cuerpo))
		if total > maxDescomprimidoZip {
			return Notebook{}, nil, ErrFormatoNoReconocido
		}
		if !utf8.Valid(cuerpo) {
			// Un fichero en otra codificación entra igual, sin los bytes que no se
			// pueden leer, mejor que perder la nota entera.
			cuerpo = bytes.ToValidUTF8(cuerpo, []byte("\uFFFD"))
		}
		notas = append(notas, notaDeMarkdown(string(cuerpo), tituloDeEntrada(f.Name), len(notas)))
	}
	return cuadernoImportado(titulo), notas, nil
}

// reHashNotion es el identificador que Notion pega al nombre de cada página
// exportada («Ideas 0123…cdef.md»): no es parte del título.
var reHashNotion = regexp.MustCompile(`\s+[0-9a-f]{32}$`)

// tituloDeEntrada saca el título de una nota de la ruta de su fichero en el ZIP.
// Un TextBundle (la exportación de Bear con adjuntos) guarda cada nota como
// «Título.textbundle/text.md»: el título es la carpeta, no «text».
func tituloDeEntrada(nombre string) string {
	base := path.Base(nombre)
	t := strings.TrimSuffix(base, path.Ext(base))
	if dir := path.Base(path.Dir(nombre)); strings.EqualFold(t, "text") &&
		strings.EqualFold(path.Ext(dir), ".textbundle") {
		t = strings.TrimSuffix(dir, path.Ext(dir))
	}
	return strings.TrimSpace(reHashNotion.ReplaceAllString(t, ""))
}

// notaDeMarkdown convierte un fichero Markdown en una nota de texto.
//
// Bear, Apple Notas y Notion repiten el título como `# Título` en la primera
// línea, y el fichero ya se llama así: se quita para no tener el título dos veces.
func notaDeMarkdown(md, titulo string, pos int) Note {
	md = quitarFrontMatter(strings.ReplaceAll(strings.TrimPrefix(md, "\uFEFF"), "\r\n", "\n"))
	primera, resto, _ := strings.Cut(strings.TrimLeft(md, "\n"), "\n")
	if h, ok := strings.CutPrefix(strings.TrimSpace(primera), "# "); ok {
		if titulo == "" || strings.EqualFold(strings.TrimSpace(h), titulo) {
			titulo = strings.TrimSpace(h)
			md = resto
		}
	}
	return Note{
		Kind:     KindText,
		Title:    recortar(titulo, maxNoteTitle),
		Content:  MarkdownAHTML(md),
		Position: pos,
	}
}

// quitarFrontMatter quita la cabecera YAML (`---` … `---`) de Obsidian y Notion:
// son metadatos para el programa, y pintados quedan como un bloque de ruido.
func quitarFrontMatter(md string) string {
	if !strings.HasPrefix(md, "---\n") {
		return md
	}
	if i := strings.Index(md[4:], "\n---"); i >= 0 {
		resto := md[4+i+4:]
		if j := strings.IndexByte(resto, '\n'); j >= 0 {
			return resto[j+1:]
		}
		return ""
	}
	return md
}

// --- Evernote (.enex) ------------------------------------------------------

func esENEX(datos []byte) bool {
	cabeza := datos
	if len(cabeza) > 4096 {
		cabeza = cabeza[:4096]
	}
	return bytes.Contains(cabeza, []byte("<en-export"))
}

type enexExport struct {
	Notas []struct {
		Title   string   `xml:"title"`
		Content string   `xml:"content"`
		Tags    []string `xml:"tag"`
	} `xml:"note"`
}

var (
	// Evernote 10 escribe las listas de tareas como listas con propiedades CSS
	// propias; las versiones anteriores, como una `<en-todo/>` al principio de un
	// `<div>`. Las dos pasan a ser la fila-casilla del editor (`div.task-row`): una
	// casilla suelta no la modela Fyne y deja de ser una tarea.
	reEnListaTareas = regexp.MustCompile(`(?is)<ul\b[^>]*--en-todo\s*:\s*true[^>]*>(.*?)</ul>`)
	reEnLi          = regexp.MustCompile(`(?is)<li\b([^>]*)>(.*?)</li>`)
	reEnLiMarcada   = regexp.MustCompile(`(?i)--en-checked\s*:\s*true`)
	reEnDivTodo     = regexp.MustCompile(`(?is)<div\b[^>]*>\s*<en-todo\b([^>]*?)/?>(?:\s*</en-todo>)?(.*?)</div>`)
	reEnDiv         = regexp.MustCompile(`(?i)</?div\b[^>]*>`)
	reEnTodo        = regexp.MustCompile(`(?is)<en-todo\b([^>]*)/?>(\s*</en-todo>)?`)
	reEnChecked     = regexp.MustCompile(`(?i)checked\s*=\s*["']true["']`)
	reEnMedia       = regexp.MustCompile(`(?is)<en-media\b[^>]*/?>(\s*</en-media>)?`)
	reEnCrypt       = regexp.MustCompile(`(?is)<en-crypt\b.*?</en-crypt>`)
	reEnNote        = regexp.MustCompile(`(?is)</?en-note\b[^>]*>`)
	reXMLDecl       = regexp.MustCompile(`(?is)<\?xml.*?\?>|<!DOCTYPE[^>]*>`)
)

func leerENEX(datos []byte, titulo string) (Notebook, []Note, error) {
	var doc enexExport
	dec := xml.NewDecoder(bytes.NewReader(datos))
	// Evernote declara la DTD de su web; no se resuelve nada externo, y las
	// entidades HTML que traen algunas exportaciones se aceptan.
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	if err := dec.Decode(&doc); err != nil {
		return Notebook{}, nil, ErrFormatoNoReconocido
	}
	notas := make([]Note, 0, len(doc.Notas))
	for _, n := range doc.Notas {
		notas = append(notas, Note{
			Kind:     KindText,
			Title:    recortar(strings.TrimSpace(n.Title), maxNoteTitle),
			Content:  enmlAHTML(n.Content, n.Tags),
			Position: len(notas),
		})
	}
	return cuadernoImportado(titulo), notas, nil
}

// enmlAHTML pasa el ENML de Evernote —XHTML con etiquetas propias— al HTML de
// una nota. Las casillas pasan a ser las del editor (`task-checkbox`, que es lo
// que las hace marcables); los adjuntos (`en-media`) y el texto cifrado de
// Evernote no se pueden traer sin sus datos y se quitan. El resto lo decide el
// saneador, como siempre.
//
// Las etiquetas de Evernote se vuelven `#hashtags` al final: aquí las etiquetas
// libres son hashtags, y así se conservan sin inventar un sitio nuevo.
func enmlAHTML(enml string, tags []string) string {
	s := reXMLDecl.ReplaceAllString(enml, "")
	s = reEnCrypt.ReplaceAllString(s, "")
	s = reEnMedia.ReplaceAllString(s, "")
	s = reEnListaTareas.ReplaceAllStringFunc(s, func(m string) string {
		var b strings.Builder
		for _, li := range reEnLi.FindAllStringSubmatch(reEnListaTareas.FindStringSubmatch(m)[1], -1) {
			b.WriteString(filaDeTarea(reEnLiMarcada.MatchString(li[1]), li[2]))
		}
		return b.String()
	})
	s = reEnDivTodo.ReplaceAllStringFunc(s, func(m string) string {
		p := reEnDivTodo.FindStringSubmatch(m)
		return filaDeTarea(reEnChecked.MatchString(p[1]), p[2])
	})
	// Lo que quede (una casilla en mitad de un párrafo) no tiene fila propia:
	// se conserva como casilla suelta antes que perder si estaba hecha.
	s = reEnTodo.ReplaceAllStringFunc(s, func(m string) string {
		if reEnChecked.MatchString(m) {
			return `<input type="checkbox" class="task-checkbox" checked> `
		}
		return `<input type="checkbox" class="task-checkbox"> `
	})
	s = strings.TrimSpace(reEnNote.ReplaceAllString(s, ""))
	var hs []string
	for _, t := range tags {
		t = strings.Join(strings.Fields(t), "-")
		if t != "" {
			hs = append(hs, "#"+html.EscapeString(t))
		}
	}
	if len(hs) > 0 {
		s += "<p>" + strings.Join(hs, " ") + "</p>"
	}
	return s
}

// filaDeTarea es la fila-casilla del editor, la misma que sale del Markdown.
func filaDeTarea(hecha bool, texto string) string {
	marca := ""
	if hecha {
		marca = " checked"
	}
	return `<div class="task-row"><input type="checkbox" class="task-checkbox"` + marca +
		`> <span>` + strings.TrimSpace(reEnDiv.ReplaceAllString(texto, "")) + `</span></div>`
}

// --- Markdown → HTML -------------------------------------------------------

var (
	reTitulo   = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	reLista    = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+(.*)$`)
	reTarea    = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	reRaya     = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	reSepTabla = regexp.MustCompile(`^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$`)

	reImagen   = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	reEmbebido = regexp.MustCompile(`!\[\[([^\]|]+)(\|[^\]]*)?\]\]`)
	reWiki     = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	reEnlace   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)(?:\s+&#34;[^)]*&#34;)?\)`)
	reAuto     = regexp.MustCompile(`&lt;((?:https?://|mailto:)[^\s&]+)&gt;`)
	reNegrita  = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	reCursiva  = regexp.MustCompile(`\*([^*\s][^*]*)\*|(^|\W)_([^_\s][^_]*)_(\W|$)`)
	reTachado  = regexp.MustCompile(`~~([^~]+)~~`)
	reResalta  = regexp.MustCompile(`==([^=]+)==`)
)

// MarkdownAHTML convierte el Markdown común (títulos, párrafos, listas, tareas,
// citas, código, tablas, separadores y el formato en línea de GFM y Obsidian) al
// HTML de una nota. No pretende ser CommonMark completo: lo que no reconoce sale
// como texto, que es lo que hace falta para no perder nada.
//
// Un salto de línea dentro de un párrafo se respeta (`<br>`): Obsidian, Bear y
// Apple Notas lo pintan así, y unir líneas cambiaría notas escritas como listas
// sin viñetas.
func MarkdownAHTML(md string) string {
	lineas := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	var parrafo []string
	cerrarParrafo := func() {
		if len(parrafo) > 0 {
			b.WriteString("<p>" + strings.Join(parrafo, "<br>") + "</p>")
			parrafo = nil
		}
	}
	for i := 0; i < len(lineas); i++ {
		l := lineas[i]
		t := strings.TrimSpace(l)
		switch {
		case t == "":
			cerrarParrafo()
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			cerrarParrafo()
			i = bloqueDeCodigo(&b, lineas, i)
		case reTitulo.MatchString(t):
			cerrarParrafo()
			m := reTitulo.FindStringSubmatch(t)
			n := strconv.Itoa(len(m[1]))
			b.WriteString("<h" + n + ">" + enLinea(m[2]) + "</h" + n + ">")
		case reRaya.MatchString(t):
			cerrarParrafo()
			b.WriteString("<hr>")
		case strings.HasPrefix(t, ">"):
			cerrarParrafo()
			i = cita(&b, lineas, i) - 1
		case strings.Contains(t, "|") && i+1 < len(lineas) && strings.Contains(lineas[i+1], "|") &&
			reSepTabla.MatchString(lineas[i+1]):
			// El separador tiene que llevar `|`: «a | b» seguido de `---` es un
			// párrafo y una raya, no una tabla.
			cerrarParrafo()
			i = tabla(&b, lineas, i) - 1
		case reLista.MatchString(l):
			cerrarParrafo()
			i = lista(&b, lineas, i) - 1
		default:
			parrafo = append(parrafo, enLinea(t))
		}
	}
	cerrarParrafo()
	return b.String()
}

// bloqueDeCodigo pinta un bloque entre vallas y devuelve la línea de la valla
// que lo cierra (o el final, si nunca se cierra).
func bloqueDeCodigo(b *strings.Builder, lineas []string, i int) int {
	valla := strings.TrimSpace(lineas[i])[:3]
	var codigo []string
	for i++; i < len(lineas) && !strings.HasPrefix(strings.TrimSpace(lineas[i]), valla); i++ {
		codigo = append(codigo, html.EscapeString(lineas[i]))
	}
	b.WriteString("<pre><code>" + strings.Join(codigo, "\n") + "</code></pre>")
	return i
}

// cita pinta las líneas seguidas que empiezan por `>` y devuelve la primera que
// ya no. Lo de dentro es Markdown otra vez.
func cita(b *strings.Builder, lineas []string, i int) int {
	var dentro []string
	for ; i < len(lineas) && strings.HasPrefix(strings.TrimSpace(lineas[i]), ">"); i++ {
		c := strings.TrimPrefix(strings.TrimSpace(lineas[i]), ">")
		dentro = append(dentro, strings.TrimPrefix(c, " "))
	}
	b.WriteString("<blockquote>" + MarkdownAHTML(strings.Join(dentro, "\n")) + "</blockquote>")
	return i
}

// tabla pinta una tabla de GFM (cabecera, separador y filas) y devuelve la
// primera línea que ya no es suya.
func tabla(b *strings.Builder, lineas []string, i int) int {
	b.WriteString("<table><thead><tr>")
	for _, c := range celdas(lineas[i]) {
		b.WriteString("<th>" + enLinea(c) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for i += 2; i < len(lineas) && strings.Contains(lineas[i], "|") && strings.TrimSpace(lineas[i]) != ""; i++ {
		b.WriteString("<tr>")
		for _, c := range celdas(lineas[i]) {
			b.WriteString("<td>" + enLinea(c) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return i
}

// lista pinta una lista seguida (con o sin tareas) y devuelve la primera línea
// que ya no le pertenece. Las tareas salen como las fila-casilla del editor, que
// es lo que las hace marcables; el anidado se aplana: el editor no lo distingue
// en las tareas, y una sangría perdida es mejor que un elemento perdido.
func lista(b *strings.Builder, lineas []string, i int) int {
	abierta := ""
	cerrar := func() {
		if abierta != "" {
			b.WriteString("</" + abierta + ">")
			abierta = ""
		}
	}
	for ; i < len(lineas); i++ {
		m := reLista.FindStringSubmatch(lineas[i])
		if m == nil {
			break
		}
		texto := m[2]
		if tm := reTarea.FindStringSubmatch(texto); tm != nil {
			cerrar()
			b.WriteString(filaDeTarea(tm[1] != " ", enLinea(tm[2])))
			continue
		}
		tipo := "ul"
		if m[1][0] >= '0' && m[1][0] <= '9' {
			tipo = "ol"
		}
		if abierta != tipo {
			cerrar()
			b.WriteString("<" + tipo + ">")
			abierta = tipo
		}
		b.WriteString("<li>" + enLinea(texto) + "</li>")
	}
	cerrar()
	return i
}

func celdas(fila string) []string {
	f := strings.TrimSpace(fila)
	f = strings.TrimSuffix(strings.TrimPrefix(f, "|"), "|")
	partes := strings.Split(f, "|")
	for k := range partes {
		partes[k] = strings.TrimSpace(partes[k])
	}
	return partes
}

// enLinea escapa el texto y aplica el formato en línea. El código entre comillas
// invertidas se aparta primero: dentro de él, `**` y `[x](y)` son literales.
func enLinea(s string) string {
	trozos := strings.Split(s, "`")
	var b strings.Builder
	for k, tr := range trozos {
		if k%2 == 1 && k < len(trozos)-1 {
			b.WriteString("<code>" + html.EscapeString(tr) + "</code>")
			continue
		}
		if k%2 == 1 {
			b.WriteString("`")
		}
		b.WriteString(formatoEnLinea(html.EscapeString(tr)))
	}
	return b.String()
}

// reApartado marca un enlace ya hecho mientras se aplica el resto del formato.
var reApartado = regexp.MustCompile("\x00(\\d+)\x00")

func formatoEnLinea(s string) string {
	// Los enlaces ya hechos se apartan: si no, el `_x_` o el `**` de una URL
	// acabarían como `<em>` dentro de su `href`.
	s = strings.ReplaceAll(s, "\x00", "")
	var apartados []string
	apartar := func(h string) string {
		apartados = append(apartados, h)
		return "\x00" + strconv.Itoa(len(apartados)-1) + "\x00"
	}
	// Las imágenes y los adjuntos incrustados no viajan con el texto: se deja su
	// nombre, que dice qué había ahí, en vez de un hueco.
	s = reEmbebido.ReplaceAllString(s, "$1")
	s = reImagen.ReplaceAllString(s, "$1")
	s = reEnlace.ReplaceAllStringFunc(s, func(m string) string {
		p := reEnlace.FindStringSubmatch(m)
		destino := html.UnescapeString(p[2])
		if ValidWebURL(destino) || strings.HasPrefix(destino, "mailto:") {
			return apartar(`<a href="`+html.EscapeString(destino)+`">`) + p[1] + `</a>`
		}
		// Un enlace a otro fichero de la bóveda no apunta a nada aquí: queda el texto.
		return p[1]
	})
	s = reAuto.ReplaceAllStringFunc(s, func(m string) string {
		u := reAuto.FindStringSubmatch(m)[1]
		return apartar(`<a href="` + u + `">` + u + `</a>`)
	})
	// `[[Nota]]` y `[[Nota|alias]]` de Obsidian: el texto que se ve.
	s = reWiki.ReplaceAllStringFunc(s, func(m string) string {
		p := reWiki.FindStringSubmatch(m)
		if p[2] != "" {
			return p[2]
		}
		return p[1]
	})
	s = reNegrita.ReplaceAllString(s, "<strong>$1$2</strong>")
	s = reCursiva.ReplaceAllStringFunc(s, func(m string) string {
		p := reCursiva.FindStringSubmatch(m)
		if p[1] != "" {
			return "<em>" + p[1] + "</em>"
		}
		return p[2] + "<em>" + p[3] + "</em>" + p[4]
	})
	s = reTachado.ReplaceAllString(s, "<del>$1</del>")
	s = reResalta.ReplaceAllString(s, "<mark>$1</mark>")
	return reApartado.ReplaceAllStringFunc(s, func(m string) string {
		k, _ := strconv.Atoi(reApartado.FindStringSubmatch(m)[1])
		return apartados[k]
	})
}

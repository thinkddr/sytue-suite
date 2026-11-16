// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// COMENTARIOS, NOTAS E HIPERVÍNCULOS de una hoja al importar y exportar .xlsx y .ods.
//
// Los comentarios de una hoja, como los de un documento, no viven en su contenido: son filas
// de doc_comments con un ancla. El de una celda se ancla con la identidad estable de su fila y
// de su columna (sheets/anclas.ts), para que siga a su celda aunque se inserten filas encima:
// `{"hoja": <id de hoja>, "fila": "t3", "col": "t1", "text": "B4"}`.
//
//   - Al importar, las notas de Excel (xl/commentsN.xml), sus comentarios en hilo
//     (xl/threadedComments) y las anotaciones de una celda de ODF (office:annotation) salen
//     como ImportedComment con su celda (Cell). Quien importa los guarda al crear el libro.
//   - Al exportar, cada hilo abierto anclado a una celda sale como UNA nota de esa celda (en
//     .xlsx y en .ods una celda lleva una nota): el primer comentario y detrás sus respuestas,
//     cada una con su autor. Los hilos resueltos no salen: una nota no tiene estado «resuelto»
//     y en la hoja ya no se ven. En .xlsx el hilo sale además como comentario en hilo de Excel
//     (xl/threadedComments, con sus autores en xl/persons), cada respuesta con su autor y su
//     fecha; la nota queda como la de compatibilidad que escribe Excel («tc={id}» de autor),
//     que es lo que leen LibreOffice y los Excel anteriores a 365.
//   - Los hipervínculos de una celda (<hyperlinks> de Excel, <text:a> de ODF) se leen como
//     `=HYPERLINK(destino,texto)`, que la rejilla ya abre (sheets/enlaces.ts) y que Excel y
//     Calc entienden al exportar sin nada más.

// notaCelda es un comentario o una nota colgada de una celda.
type notaCelda struct {
	ref      string // «B4»
	clave    string // id en el fichero (los hilos de Excel)
	padre    string // clave del comentario al que responde
	autor    string
	fecha    string // RFC 3339 (o la de Excel, sin zona)
	cuerpo   string
	resuelta bool
	// hilo son los comentarios del hilo de Excel de la nota (el primero y sus respuestas, con
	// clave y padre en GUID), solo al exportar a .xlsx. Vacío: la nota es una nota clásica.
	hilo []notaCelda
}

// maxNotasLibro acota los comentarios que se sacan de un libro (el servicio rechaza un lote
// de más de 1000).
const maxNotasLibro = 1000

// CeldaComentario es el ancla de un comentario importado de una hoja: la hoja (su id) y la
// identidad de la fila y de la columna de su celda, más la referencia que se ve.
type CeldaComentario struct {
	Hoja string `json:"hoja"`
	Fila string `json:"fila"`
	Col  string `json:"col"`
	Text string `json:"text"`
}

// anclaEje es una entrada de `rowIds`/`colIds` de una hoja.
type anclaEje struct {
	ID string `json:"id"`
}

// anclasHoja es la identidad de la hoja y de sus filas y columnas (sheets/idhoja.ts y
// sheets/anclas.ts): con ella se encuentra la celda de un comentario al exportar.
type anclasHoja struct {
	ID      string     `json:"id,omitempty"`
	RowIDs  []anclaEje `json:"rowIds,omitempty"`  //nolint:tagliatelle // el nombre lo fija la web
	RowBase int        `json:"rowBase,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
	ColIDs  []anclaEje `json:"colIds,omitempty"`  //nolint:tagliatelle // el nombre lo fija la web
	ColBase int        `json:"colBase,omitempty"` //nolint:tagliatelle // el nombre lo fija la web
}

// idsDeHojas es `conIdsDeHoja` de sheets/idhoja.ts: el id de cada hoja, y para las que no lo
// tienen (un libro importado) el que deduce la web de su nombre.
func idsDeHojas(hojas []sheetData) []string {
	vistos := map[string]bool{}
	out := make([]string, len(hojas))
	for i, h := range hojas {
		id := h.ID
		if id == "" {
			id = "h:" + h.Name
		}
		for vistos[id] {
			id += "#" + strconv.Itoa(i)
		}
		vistos[id] = true
		out[i] = id
	}
	return out
}

var reAnclaImplicita = regexp.MustCompile(`^t(\d{1,7})$`)

// indiceDeAncla es `indiceDe` de sheets/anclas.ts: la posición (0-based) de una identidad de
// fila o de columna, o -1 si ya no existe.
func indiceDeAncla(ids []anclaEje, base int, id string) int {
	for i, a := range ids {
		if a.ID == id {
			return i
		}
	}
	m := reAnclaImplicita.FindStringSubmatch(id)
	if m == nil {
		return -1
	}
	k, _ := strconv.Atoi(m[1])
	if k -= base; k < 0 {
		return -1
	}
	return len(ids) + k
}

// comentariosDeHojas pasa las notas leídas de un libro a comentarios de importación, cada
// raíz con el ancla de su celda. Los ids son hex y estables para el mismo fichero (markID).
func comentariosDeHojas(m *sheetModel) []ImportedComment {
	if m == nil {
		return nil
	}
	ids := idsDeHojas(m.Sheets)
	var out []ImportedComment
	local := map[string]string{} // hoja\x00clave -> id local
	for i, sd := range m.Sheets {
		for _, n := range sd.notas {
			if len(out) >= maxNotasLibro {
				return out
			}
			fila, col, ok := ParseA1(n.ref)
			if !ok || fila > maxFilasHoja || col > maxColumnasHoja || strings.TrimSpace(n.cuerpo) == "" {
				continue
			}
			c := ImportedComment{
				ID: markID(len(out), n.autor, n.fecha, n.cuerpo), Author: n.autor, Date: n.fecha,
				Body: n.cuerpo, Text: commentText(n.autor, n.fecha, n.cuerpo), Quote: n.ref, Resolved: n.resuelta,
			}
			if n.padre != "" {
				padre, ok := local[strconv.Itoa(i)+"\x00"+n.padre]
				if !ok {
					continue // una respuesta sin su hilo no tiene dónde ir
				}
				c.ParentID = padre
			} else {
				c.Cell = &CeldaComentario{Hoja: ids[i], Fila: "t" + strconv.Itoa(fila-1), Col: "t" + strconv.Itoa(col-1), Text: n.ref}
			}
			if n.clave != "" {
				local[strconv.Itoa(i)+"\x00"+n.clave] = c.ID
			}
			out = append(out, c)
		}
	}
	return out
}

// anclaCelda es el ancla de un comentario guardado sobre una celda.
type anclaCelda struct {
	Hoja string `json:"hoja"`
	Fila string `json:"fila"`
	Col  string `json:"col"`
}

// notasDeComentarios reparte los hilos abiertos de un libro por hoja: una nota por hilo, en la
// celda de su ancla. Un hilo cuya fila, columna u hoja ya no existe no sale.
func notasDeComentarios(m *sheetModel, cs []Comment) [][]notaCelda {
	if m == nil || len(cs) == 0 {
		return nil
	}
	ids := idsDeHojas(m.Sheets)
	hojaDe := map[string]int{}
	for i, id := range ids {
		hojaDe[id] = i
	}
	respuestas := map[string][]Comment{}
	for _, c := range cs {
		if c.ParentID != nil {
			respuestas[*c.ParentID] = append(respuestas[*c.ParentID], c)
		}
	}
	out := make([][]notaCelda, len(m.Sheets))
	ocupadas := map[[3]int]int{} // hoja, fila, col -> índice de su nota (una por celda)
	for _, c := range cs {
		if c.ParentID != nil || c.Resolved {
			continue
		}
		var a anclaCelda
		if json.Unmarshal(c.Anchor, &a) != nil || a.Hoja == "" {
			continue
		}
		h, ok := hojaDe[a.Hoja]
		if !ok {
			continue
		}
		sd := m.Sheets[h]
		fila := indiceDeAncla(sd.RowIDs, sd.RowBase, a.Fila)
		col := indiceDeAncla(sd.ColIDs, sd.ColBase, a.Col)
		if fila < 0 || col < 0 || fila >= maxFilasHoja || col >= maxColumnasHoja {
			continue
		}
		cuerpo := c.Body
		ref := A1Ref(fila+1, col+1)
		hilo := []notaCelda{{ref: ref, clave: guidXlsx("comentario:" + c.ID), autor: autorDe(c), fecha: fechaNota(c.CreatedAt), cuerpo: c.Body}}
		for _, r := range respuestas[c.ID] {
			cuerpo += "\n\n" + conAutor(autorDe(r), r.Body)
			hilo = append(hilo, notaCelda{ref: ref, clave: guidXlsx("comentario:" + r.ID), padre: hilo[0].clave, autor: autorDe(r), fecha: fechaNota(r.CreatedAt), cuerpo: r.Body})
		}
		pos := [3]int{h, fila, col}
		if k, hay := ocupadas[pos]; hay {
			// Dos hilos en la misma celda: una nota los lleva a los dos, y en Excel (un hilo por
			// celda) el segundo cuelga del primero como respuestas.
			nota := &out[h][k]
			nota.cuerpo += "\n\n" + conAutor(autorDe(c), cuerpo)
			for _, r := range hilo {
				r.padre = nota.hilo[0].clave
				nota.hilo = append(nota.hilo, r)
			}
			continue
		}
		ocupadas[pos] = len(out[h])
		out[h] = append(out[h], notaCelda{ref: ref, autor: autorDe(c), fecha: fechaNota(c.CreatedAt), cuerpo: cuerpo, hilo: hilo})
	}
	return out
}

// autorDe es el nombre con el que un comentario sale en un fichero.
func autorDe(c Comment) string {
	if c.AuthorName != "" {
		return c.AuthorName
	}
	return c.AuthorSub
}

// guidXlsx es un GUID de Excel («{8-4-4-4-12}» en mayúsculas) estable para la misma semilla:
// el mismo libro exportado dos veces da los mismos ids de hilo y de persona.
func guidXlsx(semilla string) string {
	h := sha256.Sum256([]byte(semilla))
	x := strings.ToUpper(hex.EncodeToString(h[:16]))
	return "{" + x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:] + "}"
}

// conAutor es «Autor: texto», o el texto si no hay autor.
func conAutor(autor, texto string) string {
	if autor == "" {
		return texto
	}
	return autor + ": " + texto
}

func fechaNota(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05")
}

// --- .xlsx: lectura ---

const (
	relComentariosXlsx = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relVMLXlsx         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/vmlDrawing"
	ctComentariosXlsx  = "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"
	ctVMLXlsx          = "application/vnd.openxmlformats-officedocument.vmlDrawing"
)

// textoRicoXlsx es un `<text>` de una nota: una cadena o una lista de tramos.
type textoRicoXlsx struct {
	T []string `xml:"t"`
	R []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (t textoRicoXlsx) String() string {
	var b strings.Builder
	for _, s := range t.T {
		b.WriteString(s)
	}
	for _, r := range t.R {
		b.WriteString(r.T)
	}
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}

// personasXlsx son los autores de los comentarios en hilo del libro (xl/persons/person.xml):
// id -> nombre.
func personasXlsx(files map[string][]byte, principal string) map[string]string {
	out := map[string]string{}
	for _, r := range parseRelsTyped(files[relsPath(principal)]) {
		if path.Base(r.Type) != "person" {
			continue
		}
		var lista struct {
			P []struct {
				ID     string `xml:"id,attr"`
				Nombre string `xml:"displayName,attr"`
			} `xml:"person"`
		}
		if xml.Unmarshal(files[destinoRel(principal, r.Target)], &lista) == nil {
			for _, p := range lista.P {
				out[p.ID] = p.Nombre
			}
		}
	}
	return out
}

// notasDeHojaXlsx lee los comentarios de la hoja `parte`: los de hilo (Excel 365) y las notas.
// Excel guarda además cada hilo como nota de compatibilidad («[Threaded comment]…»): una nota
// en una celda que ya tiene hilo no se lee.
func notasDeHojaXlsx(files map[string][]byte, parte string, personas map[string]string) []notaCelda {
	var notas []notaCelda
	conHilo := map[string]bool{}
	var partesNotas []string
	for _, r := range parseRelsTyped(files[relsPath(parte)]) {
		destino := destinoRel(parte, r.Target)
		switch path.Base(r.Type) {
		case "threadedComment":
			for _, n := range hilosXlsx(files[destino], personas) {
				conHilo[n.ref] = true
				notas = append(notas, n)
			}
		case "comments":
			partesNotas = append(partesNotas, destino)
		}
	}
	for _, p := range partesNotas {
		var doc struct {
			Autores []string `xml:"authors>author"`
			Lista   []struct {
				Ref   string        `xml:"ref,attr"`
				Autor string        `xml:"authorId,attr"`
				Texto textoRicoXlsx `xml:"text"`
			} `xml:"commentList>comment"`
		}
		if xml.Unmarshal(files[p], &doc) != nil {
			continue
		}
		for _, c := range doc.Lista {
			ref := strings.ToUpper(c.Ref)
			if conHilo[ref] {
				continue
			}
			autor := ""
			if k, err := strconv.Atoi(c.Autor); err == nil && k >= 0 && k < len(doc.Autores) {
				autor = strings.TrimSpace(doc.Autores[k])
			}
			if strings.HasPrefix(autor, "tc=") {
				// La nota de compatibilidad de un hilo cuyo hilo se ha perdido (un programa
				// que no lo conoce lo descarta al guardar): «tc={id}» no es nadie.
				autor = ""
			}
			cuerpo := c.Texto.String()
			cuerpo = strings.TrimPrefix(cuerpo, cabeceraHiloXlsx)
			// Excel empieza la nota por «Autor:» y un salto: eso ya lo dice la autoría.
			if autor != "" && strings.HasPrefix(cuerpo, autor+":") {
				cuerpo = cuerpo[len(autor)+1:]
			}
			notas = append(notas, notaCelda{ref: ref, autor: autor, cuerpo: strings.TrimSpace(cuerpo)})
		}
	}
	return notas
}

// hilosXlsx lee una parte threadedComments: cada comentario con su autor, su fecha y el id
// del que responde.
func hilosXlsx(raw []byte, personas map[string]string) []notaCelda {
	var doc struct {
		Lista []struct {
			Ref     string `xml:"ref,attr"`
			Fecha   string `xml:"dT,attr"`
			Persona string `xml:"personId,attr"`
			ID      string `xml:"id,attr"`
			Padre   string `xml:"parentId,attr"`
			Hecho   string `xml:"done,attr"`
			Texto   string `xml:"text"`
		} `xml:"threadedComment"`
	}
	if len(raw) == 0 || xml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	out := make([]notaCelda, 0, len(doc.Lista))
	for _, c := range doc.Lista {
		out = append(out, notaCelda{
			ref: strings.ToUpper(c.Ref), clave: c.ID, padre: c.Padre, autor: personas[c.Persona],
			fecha: c.Fecha, cuerpo: strings.TrimSpace(strings.ReplaceAll(c.Texto, "\r\n", "\n")), resuelta: verdadero(c.Hecho),
		})
	}
	return out
}

// enlaceXlsx es un `<hyperlink>` de la hoja.
type enlaceXlsx struct {
	Ref      string `xml:"ref,attr"`
	RID      string `xml:"id,attr"`
	Location string `xml:"location,attr"`
}

// maxCeldasEnlace acota las celdas que cubre un hipervínculo de rango.
const maxCeldasEnlace = 10000

// aplicarEnlacesXlsx pone a cada celda con hipervínculo su `=HYPERLINK(destino,texto)`.
func aplicarEnlacesXlsx(sd *sheetData, enlaces []enlaceXlsx, rels map[string]string) {
	hechas, pasos := 0, 0
	for _, e := range enlaces {
		destino := ""
		if e.RID != "" {
			destino = rels[e.RID]
		}
		if e.Location != "" {
			destino += "#" + e.Location
		}
		r, ok := leerRango(e.Ref)
		if !ok {
			continue
		}
		dentro, mirados := celdasDelRango(sd, r)
		pasos += mirados
		for _, ref := range dentro {
			if hechas >= maxCeldasEnlace {
				return
			}
			sd.Cells[ref] = celdaConEnlace(sd.Cells[ref], destino)
			hechas++
		}
		if pasos > maxPasosEnlaces {
			return
		}
	}
}

// celdasDelRango son las celdas con contenido dentro de r y cuántas posiciones se miraron.
// El rango sale del fichero y ParseA1 no acota (`A1:XFD1048576` o más): si tiene más
// posiciones que celdas la hoja, se recorren sus celdas. Lo mirado acota en
// aplicarEnlacesXlsx el trabajo de un fichero con miles de enlaces de rangos enormes.
func celdasDelRango(sd *sheetData, r rango) (dentro []string, mirados int) {
	if area := (r.r2 - r.r1 + 1) * (r.c2 - r.c1 + 1); area > 0 && area <= len(sd.Cells) {
		for fila := r.r1; fila <= r.r2; fila++ {
			for col := r.c1; col <= r.c2; col++ {
				ref := A1Ref(fila, col)
				if _, hay := sd.Cells[ref]; hay {
					dentro = append(dentro, ref)
				}
			}
		}
		return dentro, area
	}
	for ref := range sd.Cells {
		if fila, col, ok := ParseA1(ref); ok && fila >= r.r1 && fila <= r.r2 && col >= r.c1 && col <= r.c2 {
			dentro = append(dentro, ref)
		}
	}
	slices.Sort(dentro) // el tope corta siempre por las mismas
	return dentro, len(sd.Cells)
}

// maxPasosEnlaces acota las posiciones que se miran al aplicar los hipervínculos de una hoja.
const maxPasosEnlaces = 4_000_000

// --- hipervínculos ---

// maxLiteralFormula es lo más largo que admite Excel en una cadena de una fórmula: un enlace
// más largo no se convierte (el fichero no abriría).
const maxLiteralFormula = 255

// destinoEnlacePermitido dice si un hipervínculo se conserva: web, correo, FTP o un sitio del
// propio libro («#Hoja2!A1»). Un `javascript:`, un `file:` o un `data:` no entran en el libro.
func destinoEnlacePermitido(d string) bool {
	if d == "" || utf8.RuneCountInString(d) > maxLiteralFormula || strings.ContainsAny(d, "\x00\r\n\t") {
		return false
	}
	if strings.HasPrefix(d, "#") {
		return true
	}
	i := strings.IndexByte(d, ':')
	if i <= 0 {
		return false
	}
	switch strings.ToLower(d[:i]) {
	case "http", "https", "mailto", "ftp":
		return true
	}
	return false
}

// cadenaFormula es una cadena entre comillas de una fórmula (las comillas, dobladas).
func cadenaFormula(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// formulaDeEnlace es `=HYPERLINK(destino,texto)` para una celda con hipervínculo, o el texto
// tal cual si no se puede convertir: es ya una fórmula, el enlace no se conserva o no cabe.
func formulaDeEnlace(destino, texto string) string {
	if !destinoEnlacePermitido(destino) || strings.HasPrefix(texto, "=") || utf8.RuneCountInString(texto) > maxLiteralFormula {
		return texto
	}
	if texto == "" || texto == destino {
		return "=HYPERLINK(" + cadenaFormula(destino) + ")"
	}
	nombre := cadenaFormula(texto)
	if reNumeroLiteral.MatchString(texto) {
		nombre = texto // un número sigue siendo un número
	}
	return "=HYPERLINK(" + cadenaFormula(destino) + "," + nombre + ")"
}

// reNumeroLiteral es un número que se puede escribir tal cual en una fórmula («NaN» o «Inf»,
// que ParseFloat acepta, serían un nombre).
var reNumeroLiteral = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][+-]?\d+)?$`)

// celdaConEnlace es el HTML de una celda importada con su hipervínculo: el mismo formato
// (negrita, cursiva, alineación) con la fórmula dentro.
func celdaConEnlace(celdaHTML, destino string) string {
	texto := cellRaw(strings.TrimSpace(celdaHTML))
	f := formulaDeEnlace(destino, texto)
	if f == texto {
		return celdaHTML
	}
	p := formula.LeerCelda(celdaHTML)
	return envolverCelda(escaparCelda(f), p.Negrita, p.Cursiva, p.Alinear)
}

// escaparCelda escapa el texto de una celda como lo lee la web (`stripHtml` de
// sheets/format.ts): solo &, < y >. html.EscapeString escribe además las comillas como
// `&#34;`, que la web no deshace, y una fórmula con una cadena dentro dejaba de serlo.
var escaparCelda = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace

// --- .xlsx: escritura ---

const (
	relHilosXlsx    = "http://schemas.microsoft.com/office/2017/10/relationships/threadedComment"
	relPersonasXlsx = "http://schemas.microsoft.com/office/2017/10/relationships/person"
	ctHilosXlsx     = "application/vnd.ms-excel.threadedcomments+xml"
	ctPersonasXlsx  = "application/vnd.ms-excel.person+xml"
	nsHilosXlsx     = "http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments"
	// cabeceraHiloXlsx abre la nota de compatibilidad de un hilo, como la escribe Excel.
	cabeceraHiloXlsx = "[Threaded comment]\n\n"
)

// personasHilosXlsx son los autores de los hilos de un libro, en el orden en que aparecen
// (xl/persons/person.xml).
type personasHilosXlsx struct {
	nombres []string
	vistos  map[string]bool
}

// id es el GUID de la persona `nombre`, que queda apuntada.
func (p *personasHilosXlsx) id(nombre string) string {
	if p.vistos == nil {
		p.vistos = map[string]bool{}
	}
	if !p.vistos[nombre] {
		p.vistos[nombre] = true
		p.nombres = append(p.nombres, nombre)
	}
	return guidXlsx("persona:" + nombre)
}

// xml es la parte xl/persons/person.xml.
func (p *personasHilosXlsx) xml() string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<personList xmlns="` + nsHilosXlsx + `" xmlns:x="` + nsMainX + `">`)
	for _, n := range p.nombres {
		b.WriteString(`<person displayName="` + xmlEscape(n) + `" id="` + guidXlsx("persona:"+n) + `" userId="` + xmlEscape(n) + `" providerId="None"/>`)
	}
	b.WriteString(`</personList>`)
	return b.String()
}

// hilosXlsxParte es la parte threadedComments de una hoja con los hilos de sus notas, o "" si
// ninguna lleva hilo.
func hilosXlsxParte(notas []notaCelda, personas *personasHilosXlsx) string {
	var b strings.Builder
	for _, nota := range notas {
		for _, c := range nota.hilo {
			b.WriteString(`<threadedComment ref="` + c.ref + `"`)
			if c.fecha != "" {
				b.WriteString(` dT="` + xmlEscape(c.fecha) + `.00"`)
			}
			b.WriteString(` personId="` + personas.id(c.autor) + `" id="` + c.clave + `"`)
			if c.padre != "" {
				b.WriteString(` parentId="` + c.padre + `"`)
			}
			b.WriteString(`><text>` + xmlEscape(c.cuerpo) + `</text></threadedComment>`)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return xmlHeader + `<ThreadedComments xmlns="` + nsHilosXlsx + `" xmlns:x="` + nsMainX + `">` + b.String() + `</ThreadedComments>`
}

// notasXlsx son la parte de comentarios (las notas de la hoja) y su dibujo VML, sin el cual
// Excel no las enseña. La nota de una celda con hilo es la de compatibilidad de Excel: de
// autor «tc={id del hilo}», que es como Excel 365 las empareja, y el hilo entero de texto. Las formas usan los bloques de ids `primero`…`ultimo` del libro (1024
// por bloque): dos hojas con el mismo bloque, o una hoja con más formas que su bloque, hacen que
// Excel pida reparar el fichero.
func notasXlsx(notas []notaCelda, primero, ultimo int) (comentarios, vml string) {
	autores := []string{}
	idAutor := map[string]int{}
	var lista, formas strings.Builder
	for k, nota := range notas {
		autor, cuerpo := nota.autor, nota.cuerpo
		if len(nota.hilo) > 0 {
			autor, cuerpo = "tc="+nota.hilo[0].clave, cabeceraHiloXlsx+conAutor(nota.autor, nota.cuerpo)
		}
		id, hay := idAutor[autor]
		if !hay {
			id = len(autores)
			idAutor[autor] = id
			autores = append(autores, autor)
		}
		if len(nota.hilo) > 0 {
			autor = "" // sin la cabecera en negrita: el texto ya dice quién escribe
		}
		lista.WriteString(`<comment ref="` + nota.ref + `" authorId="` + strconv.Itoa(id) + `"><text>`)
		if autor != "" {
			lista.WriteString(`<r><rPr><b/><sz val="9"/><rFont val="Tahoma"/></rPr><t>` + xmlEscape(autor) + `:</t></r>`)
		}
		lista.WriteString(`<r><rPr><sz val="9"/><rFont val="Tahoma"/></rPr><t xml:space="preserve">`)
		if autor != "" {
			lista.WriteString("\n")
		}
		lista.WriteString(xmlEscape(cuerpo) + `</t></r></text></comment>`)
		fila, col, _ := ParseA1(nota.ref)
		formas.WriteString(`<v:shape id="_x0000_s` + strconv.Itoa(primero*1024+k+1) + `" type="#_x0000_t202" ` +
			`style="position:absolute;margin-left:59.25pt;margin-top:1.5pt;width:144pt;height:72pt;z-index:` + strconv.Itoa(k+1) + `;visibility:hidden" ` +
			`fillcolor="#ffffe1" o:insetmode="auto"><v:fill color2="#ffffe1"/><v:shadow on="t" color="black" obscured="t"/>` +
			`<v:path o:connecttype="none"/><v:textbox style="mso-direction-alt:auto"><div style="text-align:left"></div></v:textbox>` +
			`<x:ClientData ObjectType="Note"><x:MoveWithCells/><x:SizeWithCells/>` +
			`<x:Anchor>` + strconv.Itoa(col) + `, 15, ` + strconv.Itoa(fila-1) + `, 2, ` + strconv.Itoa(col+2) + `, 15, ` + strconv.Itoa(fila+3) + `, 16</x:Anchor>` +
			`<x:AutoFill>False</x:AutoFill><x:Row>` + strconv.Itoa(fila-1) + `</x:Row><x:Column>` + strconv.Itoa(col-1) + `</x:Column></x:ClientData></v:shape>`)
	}
	var cab strings.Builder
	for _, a := range autores {
		cab.WriteString(`<author>` + xmlEscape(a) + `</author>`)
	}
	comentarios = xmlHeader + `<comments xmlns="` + nsMainX + `"><authors>` + cab.String() + `</authors><commentList>` + lista.String() + `</commentList></comments>`
	vml = `<xml xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel">` +
		`<o:shapelayout v:ext="edit"><o:idmap v:ext="edit" data="` + bloquesIdmap(primero, ultimo) + `"/></o:shapelayout>` +
		`<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe">` +
		`<v:stroke joinstyle="miter"/><v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>` + formas.String() + `</xml>`
	return comentarios, vml
}

// bloquesIdmap es la lista «3,4,5» de bloques de `o:idmap`.
func bloquesIdmap(primero, ultimo int) string {
	partes := make([]string, 0, ultimo-primero+1)
	for b := primero; b <= ultimo; b++ {
		partes = append(partes, strconv.Itoa(b))
	}
	return strings.Join(partes, ",")
}

// --- .ods ---

// leerAnotacionODS lee una `office:annotation` ya abierta: autor, fecha y párrafos.
func leerAnotacionODS(dec *xml.Decoder) notaCelda {
	var n notaCelda
	var parrafos []string
	var actual strings.Builder
	donde := "" // "creator", "date" o "p": el texto que se está leyendo
	prof := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			prof++
			switch t.Name.Local {
			case "creator", "date":
				donde = t.Name.Local
			case "p", "h":
				donde = "p"
				actual.Reset()
			case "line-break":
				actual.WriteByte('\n')
			case "s":
				actual.WriteString(strings.Repeat(" ", max(1, min(repeticionesODS(attrLocal(t, "c")), 64))))
			case "tab":
				actual.WriteByte('\t')
			}
		case xml.CharData:
			switch donde {
			case "creator":
				n.autor += string(t)
			case "date":
				n.fecha += string(t)
			case "p":
				actual.Write(t)
			}
		case xml.EndElement:
			if prof == 0 { // </office:annotation>
				n.autor, n.fecha = strings.TrimSpace(n.autor), strings.TrimSpace(n.fecha)
				n.cuerpo = strings.TrimSpace(strings.Join(parrafos, "\n"))
				return n
			}
			prof--
			switch t.Name.Local {
			case "p", "h":
				parrafos = append(parrafos, actual.String())
				donde = ""
			case "creator", "date":
				donde = ""
			}
		}
	}
	n.cuerpo = strings.TrimSpace(strings.Join(parrafos, "\n"))
	return n
}

// anotacionODS es la `office:annotation` de una nota, que va la primera dentro de su celda.
func anotacionODS(n notaCelda) string {
	var b strings.Builder
	b.WriteString(`<office:annotation>`)
	if n.autor != "" {
		b.WriteString(`<dc:creator>` + xmlEscape(n.autor) + `</dc:creator>`)
	}
	if n.fecha != "" {
		b.WriteString(`<dc:date>` + xmlEscape(n.fecha) + `</dc:date>`)
	}
	for _, p := range strings.Split(n.cuerpo, "\n") {
		b.WriteString(`<text:p>` + xmlEscape(p) + `</text:p>`)
	}
	b.WriteString(`</office:annotation>`)
	return b.String()
}

// conAnotacionODS mete la nota en el `table:table-cell` ya escrito, delante de lo demás.
func conAnotacionODS(celda, nota string) string {
	if nota == "" || !strings.HasPrefix(celda, "<table:table-cell") {
		return celda
	}
	if strings.HasSuffix(celda, "/>") && strings.Count(celda, "<") == 1 {
		return strings.TrimSuffix(celda, "/>") + ">" + nota + "</table:table-cell>"
	}
	return strings.Replace(celda, ">", ">"+nota, 1)
}

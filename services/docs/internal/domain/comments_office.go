// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Los COMENTARIOS de un documento no viven en su HTML: el HTML solo lleva el ancla
// (<mark data-comment-id>) y el comentario es una fila de doc_comments, con su hilo.
//
//   - Al exportar a DOCX/ODT, quien llama pasa los comentarios (ExportWithComments) y el
//     ancla de cada uno se busca por el id de su <mark>.
//   - Al importar, ImportWithComments devuelve los comentarios del fichero (ImportedComment)
//     y deja en el HTML un <mark> SOLO para los que tienen un trozo de texto al que
//     anclarse. Quien importa los guarda al crear el documento; por eso los marks no se
//     emiten si nadie va a guardar los comentarios (Import a secas): quedarían resaltados sin
//     comentario.

// ImportedComment es un comentario de un fichero de fuera, listo para guardarse.
//
// Regla de autoría (decisión del dueño): lo guarda quien importa, sin usuarios inventados;
// el autor y la fecha originales van DELANTE del cuerpo, en Text («Nombre, 2026-09-30:
// texto»). Author, Date y Body conservan lo original.
type ImportedComment struct {
	ID       string `json:"id"`                  // id local; es el del <mark> si Anchored
	ParentID string `json:"parent_id,omitempty"` // id local del comentario al que responde
	Author   string `json:"author,omitempty"`
	Date     string `json:"date,omitempty"` // RFC 3339
	Body     string `json:"body"`
	Text     string `json:"text"`
	Quote    string `json:"quote,omitempty"` // el texto al que se ancla
	Anchored bool   `json:"anchored"`        // hay un <mark data-comment-id=ID> en el HTML
	Resolved bool   `json:"resolved,omitempty"`
}

// commentText antepone la autoría original al cuerpo: «Nombre, fecha: texto».
func commentText(author, date, body string) string {
	if t := parseTrackTime(date); t != 0 {
		date = time.UnixMilli(t).UTC().Format("2006-01-02")
	} else {
		date = ""
	}
	head := strings.Join(nonEmpty(author, date), ", ")
	if head == "" {
		return body
	}
	return head + ": " + body
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, x := range s {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// markID es el id del <mark> de un comentario importado: hex, como los que genera el editor
// (los usa en un selector sin escapar), y estable para el mismo fichero.
func markID(n int, author, date, body string) string {
	h := sha256.Sum256([]byte(strconv.Itoa(n) + "\x00" + author + "\x00" + date + "\x00" + body))
	return hex.EncodeToString(h[:8])
}

// rawComment es un comentario tal como sale del fichero, antes de decidir su ancla.
type rawComment struct {
	key              string // id en el fichero
	id               string // id local (el del <mark>)
	parentKey        string
	author, date     string
	paras            []string
	resolved         bool
	quote            strings.Builder
	anchored, marked bool // anchored: tiene texto al que anclarse
}

// importedComments pasa los comentarios del fichero, en orden, a los de importación.
// Un comentario que responde a otro no lleva ancla propia (el hilo cuelga del padre).
func importedComments(raw []*rawComment) []ImportedComment {
	ids := map[string]string{}
	for _, c := range raw {
		ids[c.key] = c.id
	}
	out := make([]ImportedComment, 0, len(raw))
	for _, c := range raw {
		body := strings.Join(c.paras, "\n")
		out = append(out, ImportedComment{
			ID: c.id, ParentID: ids[c.parentKey], Author: c.author, Date: c.date, Body: body,
			Text: commentText(c.author, c.date, body), Resolved: c.resolved,
			Quote: strings.TrimSpace(c.quote.String()), Anchored: c.anchored && c.marked && c.parentKey == "",
		})
	}
	return out
}

// xcomment es un comentario para exportar.
type xcomment struct {
	author   string
	date     time.Time
	paras    []string
	parent   int // índice del padre, o -1
	resolved bool
}

// commentMark es el id del <mark> de un comentario guardado: su ancla es {"mark","text"}
// o solo el texto citado.
func commentMark(anchor json.RawMessage) string {
	var a struct {
		Mark string `json:"mark"`
	}
	_ = json.Unmarshal(anchor, &a)
	return a.Mark
}

// exportComments prepara los comentarios de un documento para exportarlos: los que se
// anclan a un <mark> que está en el HTML (byMark, con sus respuestas detrás) y los demás
// (points: sin trozo de texto al que anclarse, que van como comentario de punto al principio).
func exportComments(cs []Comment, marks map[string]bool) (list []xcomment, byMark map[string][]int, points []int) {
	pos := map[string]int{}
	for i, c := range cs {
		pos[c.ID] = i
	}
	byMark = map[string][]int{}
	root := func(i int) int { // el comentario raíz de un hilo
		for n := 0; n < len(cs); n++ {
			p := cs[i].ParentID
			if p == nil {
				break
			}
			j, ok := pos[*p]
			if !ok {
				break
			}
			i = j
		}
		return i
	}
	for i, c := range cs {
		parent := -1
		if c.ParentID != nil {
			if j, ok := pos[*c.ParentID]; ok {
				parent = j
			}
		}
		author := c.AuthorName
		if author == "" {
			author = c.AuthorSub
		}
		list = append(list, xcomment{author: author, date: c.CreatedAt, paras: strings.Split(c.Body, "\n"), parent: parent, resolved: c.Resolved})
		if m := commentMark(cs[root(i)].Anchor); m != "" && marks[m] {
			byMark[m] = append(byMark[m], i)
		} else {
			points = append(points, i)
		}
	}
	return list, byMark, points
}

// initials son las iniciales de un autor (hasta tres).
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		if b.Len() < 3 {
			b.WriteString(strings.ToUpper(string([]rune(w)[:1])))
		}
	}
	return b.String()
}

// docxComments es el estado de los comentarios mientras se lee el cuerpo de un .docx.
// Todos sus métodos admiten el nil de «no se piden comentarios».
type docxComments struct {
	raw   []*rawComment
	byKey map[string]*rawComment
	open  []string       // claves de los rangos abiertos, por orden de apertura
	start map[string]int // párrafo en que se abrió cada uno
}

// span abre (start) o cierra el rango de un comentario.
func (c *docxComments) span(start bool, key string, para int) {
	if c == nil || c.byKey[key] == nil {
		return
	}
	if start {
		c.open = append(c.open, key)
		c.start[key] = para
		return
	}
	if i := slices.Index(c.open, key); i >= 0 {
		c.open = slices.Delete(c.open, i, i+1)
	}
}

// current es el id del <mark> que cubre el texto de ahora: el del primer rango abierto que
// no es una respuesta y empezó en este mismo párrafo (un <mark> no cruza párrafos). El que
// no consigue texto queda como comentario sin ancla.
func (c *docxComments) current(para int) string {
	if c == nil {
		return ""
	}
	for _, k := range c.open {
		if r := c.byKey[k]; r.parentKey == "" && c.start[k] == para {
			r.marked = true
			return r.id
		}
	}
	return ""
}

// quote añade texto al citado por el comentario con ese id de <mark>.
func (c *docxComments) quote(id, text string) {
	if c == nil || id == "" {
		return
	}
	for _, r := range c.raw {
		if r.id == id {
			r.quote.WriteString(text)
			r.anchored = true
		}
	}
}

// parseDocxComments lee comments.xml y commentsExtended.xml (hilos y «resuelto»).
func parseDocxComments(files map[string][]byte) *docxComments {
	c := &docxComments{byKey: map[string]*rawComment{}, start: map[string]int{}}
	byPara := map[string]*rawComment{} // w14:paraId del último párrafo -> comentario
	dec := xml.NewDecoder(bytes.NewReader(files["word/comments.xml"]))
	var cur *rawComment
	var para strings.Builder
	var paraID string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "comment":
				cur = &rawComment{key: attrLocal(t, "id"), author: attrLocal(t, "author"), date: attrLocal(t, "date")}
			case "p":
				para.Reset()
				paraID = attrLocal(t, "paraId")
			case "t":
				var s string
				_ = dec.DecodeElement(&s, &t)
				para.WriteString(s)
			case "tab":
				para.WriteString("\t")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if cur != nil {
					cur.paras = append(cur.paras, para.String())
					if paraID != "" {
						byPara[paraID] = cur
					}
				}
			case "comment":
				if cur != nil {
					cur.id = markID(len(c.raw), cur.author, cur.date, strings.Join(cur.paras, "\n"))
					c.raw = append(c.raw, cur)
					c.byKey[cur.key] = cur
					cur = nil
				}
			}
		}
	}
	c.applyExtended(files["word/commentsExtended.xml"], byPara)
	return c
}

// applyExtended lee commentsExtended.xml: el padre de cada respuesta y los resueltos.
func (c *docxComments) applyExtended(data []byte, byPara map[string]*rawComment) {
	ex := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := ex.Token()
		if err != nil {
			return
		}
		if t, ok := tok.(xml.StartElement); ok && t.Name.Local == "commentEx" {
			r := byPara[attrLocal(t, "paraId")]
			if r == nil {
				continue
			}
			r.resolved = attrLocal(t, "done") == "1" || attrLocal(t, "done") == "true"
			if p := byPara[attrLocal(t, "paraIdParent")]; p != nil && p != r {
				r.parentKey = p.key
			}
		}
	}
}

// odfComments es el estado de los comentarios (office:annotation) mientras se lee un .odt.
// Todos sus métodos admiten el nil de «no se piden comentarios».
type odfComments struct {
	raw     []*rawComment
	byName  map[string]*rawComment
	ends    map[string]bool // anotaciones que tienen su annotation-end (las demás son de punto)
	open    []string
	start   map[string]int
	para    int
	reading bool // leyendo el cuerpo de una anotación: su texto no es texto anclado
}

var reAnnotationEnd = regexp.MustCompile(`<office:annotation-end[^>]*\soffice:name="([^"]*)"`)

func newODFComments(content []byte) *odfComments {
	c := &odfComments{byName: map[string]*rawComment{}, ends: map[string]bool{}, start: map[string]int{}}
	for _, m := range reAnnotationEnd.FindAllSubmatch(content, -1) {
		c.ends[string(m[1])] = true
	}
	return c
}

func (c *odfComments) rawList() []*rawComment {
	if c == nil {
		return nil
	}
	return c.raw
}

// paraStart cuenta los párrafos del cuerpo (los de dentro de una anotación no).
func (c *odfComments) paraStart(until string) {
	if c != nil && !c.reading && (until == "p" || until == "h") {
		c.para++
	}
}

func (c *odfComments) end(name string) {
	if c == nil {
		return
	}
	if i := slices.Index(c.open, name); i >= 0 {
		c.open = slices.Delete(c.open, i, i+1)
	}
}

// current es el id del <mark> que cubre el texto de ahora (ver docxComments.current).
func (c *odfComments) current() string {
	if c == nil || c.reading {
		return ""
	}
	for _, n := range c.open {
		if r := c.byName[n]; r.parentKey == "" && c.start[n] == c.para {
			r.marked = true
			return r.id
		}
	}
	return ""
}

func (c *odfComments) quote(id, text string) {
	if c == nil || id == "" {
		return
	}
	for _, r := range c.raw {
		if r.id == id {
			r.quote.WriteString(text)
			r.anchored = true
		}
	}
}

// readAnnotation lee un <office:annotation> recién abierto: su autor, fecha y texto, y lo
// deja abierto si tiene fin (su rango).
func (d *odfDec) readAnnotation(t xml.StartElement) {
	if d.cm == nil {
		_ = d.dec.Skip()
		return
	}
	name := attrLocal(t, "name")
	r := &rawComment{key: name, parentKey: attrLocal(t, "parent-name"), resolved: attrLocal(t, "resolved") == "true"}
	d.cm.reading = true
	for {
		tok, err := d.dec.Token()
		if err != nil {
			break
		}
		done := false
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "creator":
				r.author = strings.TrimSpace(d.readTextOf("creator"))
			case "date":
				r.date = rfc3339(strings.TrimSpace(d.readTextOf("date")))
			case "p":
				var b strings.Builder
				for _, x := range d.readRuns("p", xrun{}) {
					b.WriteString(x.text)
				}
				r.paras = append(r.paras, b.String())
			default:
				_ = d.dec.Skip()
			}
		case xml.EndElement:
			done = e.Name.Local == "annotation"
		}
		if done {
			break
		}
	}
	d.cm.reading = false
	r.id = markID(len(d.cm.raw), r.author, r.date, strings.Join(r.paras, "\n"))
	d.cm.raw = append(d.cm.raw, r)
	d.cm.byName[name] = r
	if d.cm.ends[name] {
		d.cm.open = append(d.cm.open, name)
		d.cm.start[name] = d.cm.para
	}
}

// rfc3339 pasa la fecha de ODF (sin zona: UTC) a RFC 3339; "" si no se entiende.
func rfc3339(s string) string {
	if t := parseTrackTime(s); t != 0 {
		return time.UnixMilli(t).UTC().Format(time.RFC3339)
	}
	return ""
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Las CITAS y la BIBLIOGRAFÍA de «Referencias» (`bibliography.ts` y Bibliografia.tsx), con
// el mismo HTML: las fuentes van EMBEBIDAS en el folio, en un
// `<script type="application/json" data-sytue-sources>`; una cita es la ficha
// `<span data-citation="id" contenteditable="false">(Autor, año)</span>`, que no se edita; y
// la bibliografía, una `<section data-bibliography>` con las fuentes citadas en APA básico,
// ordenadas por autor, con sangría francesa.

// Fuente es una fuente bibliográfica (`Source`): libro, artículo o web. Editorial es la
// editorial, la revista o el sitio, según el tipo; URL, solo de una web.
type Fuente struct {
	ID        string `json:"id"`
	Tipo      string `json:"type"`
	Autor     string `json:"author"`
	Titulo    string `json:"title"`
	Anio      string `json:"year"`
	Editorial string `json:"publisher"`
	URL       string `json:"url,omitempty"`
}

// TiposFuente son los de la web, en su orden.
var TiposFuente = []string{"libro", "articulo", "web"}

// estiloEntradaBib es `ENTRY_STYLE`: la sangría francesa de 2 em.
const estiloEntradaBib = "margin:.35em 0;padding-left:2em;text-indent:-2em"

func esFuentes(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Script && n.Namespace == "" && tieneAtributo(n, "data-sytue-sources") &&
		atributo(n, "type") == "application/json"
}

func esCita(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Span && n.Namespace == "" && tieneAtributo(n, "data-citation")
}

func esEntradaBib(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.P && n.Namespace == "" && tieneAtributo(n, "data-bib-entry")
}

func esSeccionBib(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Section && tieneAtributo(n, "data-bibliography")
}

func (d *Documento) scriptFuentes() *xhtml.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if esFuentes(h) {
			return h
		}
	}
	return nil
}

// Fuentes son las embebidas en el folio (`readSources`), ya saneadas: un tipo que no es de
// los tres es un libro, y las que no tienen id no cuentan. Con el JSON roto, ninguna.
func (d *Documento) Fuentes() []Fuente {
	s := d.scriptFuentes()
	if s == nil || s.FirstChild == nil {
		return nil
	}
	var crudas []map[string]any
	if json.Unmarshal([]byte(textoDe(s)), &crudas) != nil {
		return nil
	}
	texto := func(m map[string]any, k string) string {
		v, _ := m[k].(string)
		return v
	}
	var out []Fuente
	for _, m := range crudas {
		f := Fuente{
			ID: texto(m, "id"), Tipo: texto(m, "type"), Autor: texto(m, "author"), Titulo: texto(m, "title"),
			Anio: texto(m, "year"), Editorial: texto(m, "publisher"), URL: strings.TrimSpace(texto(m, "url")),
		}
		if !slices.Contains(TiposFuente, f.Tipo) {
			f.Tipo = "libro"
		}
		if f.ID != "" {
			out = append(out, f)
		}
	}
	return out
}

// PonerFuentes es `writeSources`: escribe las fuentes en su `<script>` —con «<» como
// `\u003c`, que ningún valor cierre el bloque— o lo quita si no queda ninguna.
func (d *Documento) PonerFuentes(fs []Fuente) {
	s := d.scriptFuentes()
	if len(fs) == 0 {
		if s != nil {
			s.Parent.RemoveChild(s)
		}
		return
	}
	if s == nil {
		s = elemento("script", xhtml.Attribute{Key: "type", Val: "application/json"}, xhtml.Attribute{Key: "data-sytue-sources"})
		d.raiz.AppendChild(s)
	}
	ponerTexto(s, jsonWeb(fs))
}

var reIDFuente = regexp.MustCompile(`^src-(\d+)$`)

// SiguienteFuente es `nextSourceId`: «src-N», uno más que el mayor.
func SiguienteFuente(fs []Fuente) string {
	n := 0
	for _, f := range fs {
		if m := reIDFuente.FindStringSubmatch(f.ID); m != nil {
			v, _ := strconv.Atoi(m[1])
			n = max(n, v)
		}
	}
	return "src-" + strconv.Itoa(n+1)
}

// Apellido es `authorSurname`: lo de antes de la primera coma, o el autor entero.
func Apellido(autor string) string {
	a := strings.TrimSpace(autor)
	antes, _, _ := strings.Cut(a, ",")
	return strings.TrimSpace(antes)
}

// TextoCita es `citationText`: «(Autor, año)»; sin autor, el título; sin nada, «Anónimo»;
// sin año, «s.f.».
func TextoCita(f Fuente) string {
	quien := Apellido(f.Autor)
	if quien == "" {
		quien = strings.TrimSpace(f.Titulo)
	}
	if quien == "" {
		quien = "Anónimo"
	}
	anio := strings.TrimSpace(f.Anio)
	if anio == "" {
		anio = "s.f."
	}
	return "(" + quien + ", " + anio + ")"
}

// entradaAPA es `apaEntry`: «Autor (Año). Título. Fuente. URL», con el título en cursiva en
// un libro o una web y la revista en cursiva en un artículo; lo vacío no sale.
func entradaAPA(f Fuente) string {
	e := escaparWeb.Replace
	a, y, t, p, u := e(strings.TrimSpace(f.Autor)), strings.TrimSpace(f.Anio), e(strings.TrimSpace(f.Titulo)),
		e(strings.TrimSpace(f.Editorial)), e(strings.TrimSpace(f.URL))
	if y == "" {
		y = "s.f."
	}
	y = e(y)
	var segs []string
	if a != "" {
		segs = append(segs, a+" ("+y+").")
	} else {
		segs = append(segs, "("+y+").")
	}
	articulo := f.Tipo == "articulo"
	switch {
	case t != "" && !articulo:
		segs = append(segs, "<i>"+t+"</i>.")
	case t != "":
		segs = append(segs, t+".")
	}
	switch {
	case p != "" && articulo:
		segs = append(segs, "<i>"+p+"</i>.")
	case p != "":
		segs = append(segs, p+".")
	}
	if u != "" {
		segs = append(segs, u)
	}
	return strings.Join(segs, " ")
}

// listaBibliografia es `buildBibliographyList`: las citadas, por apellido y luego por
// título, cada una en su párrafo con sangría francesa.
func listaBibliografia(fs []Fuente, citadas []string) string {
	var sel []Fuente
	for _, f := range fs {
		if slices.Contains(citadas, f.ID) {
			sel = append(sel, f)
		}
	}
	slices.SortStableFunc(sel, func(a, b Fuente) int {
		if c := ordenIndice.CompareString(Apellido(a.Autor), Apellido(b.Autor)); c != 0 {
			return c
		}
		return ordenIndice.CompareString(a.Titulo, b.Titulo)
	})
	var b strings.Builder
	for _, f := range sel {
		b.WriteString(`<p data-bib-entry style="` + estiloEntradaBib + `">` + entradaAPA(f) + `</p>`)
	}
	return b.String()
}

// InsertarCita es `insertCitation`: la ficha de la cita en pos, que es UNA runa.
func (e *Edicion) InsertarCita(pos int, f Fuente) {
	cita := elemento("span", xhtml.Attribute{Key: "data-citation", Val: f.ID}, xhtml.Attribute{Key: "contenteditable", Val: "false"})
	cita.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: TextoCita(f)})
	pos = max(0, min(pos, len(e.runas)))
	cadena := append(slices.Clip(fueraDeMarcador(e.hereda(pos))), cita)
	e.runas = slices.Insert(e.runas, pos, ObjetoRuna)
	e.cadenas = slices.Insert(e.cadenas, pos, cadena)
	e.hayPendiente = false
	e.reescribir()
}

// GenerarBibliografia es `rebuildBibliography`: pone al día el texto de cada cita con su
// fuente y rehace la sección en su sitio o, nueva, antes de las notas al pie (o al final).
// `titulo` y `vacio` son `bib.heading` y `bib.empty`, traducidos. Devuelve cuántas
// entradas lista.
func (d *Documento) GenerarBibliografia(titulo, vacio string) int {
	fs := d.Fuentes()
	porID := map[string]Fuente{}
	for _, f := range fs {
		porID[f.ID] = f
	}
	var citadas []string
	for _, s := range elementosDe(d.raiz, atom.Span) {
		if !esCita(s) {
			continue
		}
		id := atributo(s, "data-citation")
		citadas = append(citadas, id)
		if f, ok := porID[id]; ok {
			ponerTexto(s, TextoCita(f))
		}
	}
	lista := listaBibliografia(fs, citadas)
	n := strings.Count(lista, "<p data-bib-entry")
	if lista == "" {
		lista = `<p style="color:#64748b"><i>` + escaparWeb.Replace(vacio) + `</i></p>`
	}
	nodos, err := xhtml.ParseFragment(strings.NewReader(`<p><b>`+escaparWeb.Replace(titulo)+`</b></p>`+lista), cuerpo())
	if err != nil {
		return 0
	}
	var seccion *xhtml.Node
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if esSeccionBib(h) {
			seccion = h
		}
	}
	if seccion == nil {
		seccion = elemento("section", xhtml.Attribute{Key: "data-bibliography"})
		d.raiz.InsertBefore(seccion, d.seccionNotas())
	}
	vaciar(seccion)
	for _, h := range nodos {
		seccion.AppendChild(h)
	}
	return n
}

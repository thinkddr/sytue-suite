// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// La COMBINACIÓN DE CORRESPONDENCIA de «Referencias» (`mailmerge.ts` y
// Correspondencia.tsx), con el mismo HTML: el origen de datos va embebido en un
// `<script type="application/json" data-sytue-mailmerge>` con sus campos y sus registros,
// y cada campo es la ficha `<span data-mergefield="Campo" contenteditable="false">«Campo»</span>`,
// que no se edita.

// Origen es el de la combinación (`MergeSource`): los campos, en orden, y un registro por
// fila con TODOS los campos.
type Origen struct {
	Campos    []string            `json:"fields"`
	Registros []map[string]string `json:"records"`
}

func esOrigen(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Script && n.Namespace == "" && tieneAtributo(n, "data-sytue-mailmerge") &&
		atributo(n, "type") == "application/json"
}

func esCampoCombinacion(n *xhtml.Node) bool {
	return n != nil && n.DataAtom == atom.Span && n.Namespace == "" && tieneAtributo(n, "data-mergefield")
}

func (d *Documento) scriptOrigen() *xhtml.Node {
	for h := d.raiz.FirstChild; h != nil; h = h.NextSibling {
		if esOrigen(h) {
			return h
		}
	}
	return nil
}

// OrigenCombinacion es `readMergeSource`: el origen embebido, o nil si no hay o está roto.
func (d *Documento) OrigenCombinacion() *Origen {
	s := d.scriptOrigen()
	if s == nil || s.FirstChild == nil {
		return nil
	}
	var o Origen
	if json.Unmarshal([]byte(textoDe(s)), &o) != nil || o.Campos == nil || o.Registros == nil {
		return nil
	}
	return &o
}

// PonerOrigen es `writeMergeSource` (y, con nil, `removeMergeSource`).
func (d *Documento) PonerOrigen(o *Origen) {
	s := d.scriptOrigen()
	if o == nil {
		for s != nil {
			s.Parent.RemoveChild(s)
			s = d.scriptOrigen()
		}
		return
	}
	if s == nil {
		s = elemento("script", xhtml.Attribute{Key: "type", Val: "application/json"}, xhtml.Attribute{Key: "data-sytue-mailmerge"})
		d.raiz.AppendChild(s)
	}
	ponerTexto(s, jsonWeb(o))
}

// jsonWeb es `JSON.stringify(x).replace(/</g, '\u003c')`: sin el escape HTML de Go, con
// «<» escapado para que ningún valor cierre el `<script>`.
func jsonWeb(x any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(x)
	return strings.ReplaceAll(strings.TrimSuffix(b.String(), "\n"), "<", `\u003c`)
}

// delimitador es `detectDelimiter`: el tabulador si lo hay en la cabecera; si no, el que
// más salga entre coma y punto y coma, fuera de comillas.
func delimitador(cabecera string) rune {
	var tab, coma, punto int
	entre := false
	for _, r := range cabecera {
		switch {
		case r == '"':
			entre = !entre
		case entre:
		case r == '\t':
			tab++
		case r == ',':
			coma++
		case r == ';':
			punto++
		}
	}
	switch {
	case tab > 0:
		return '\t'
	case punto > coma:
		return ';'
	}
	return ','
}

// filasDelimitadas es `splitRows`: RFC 4180 básico, con comillas dobles y `""` dentro.
func filasDelimitadas(texto string, delim rune) [][]string {
	s := strings.ReplaceAll(strings.ReplaceAll(texto, "\r\n", "\n"), "\r", "\n")
	var filas [][]string
	var fila []string
	var campo strings.Builder
	entre := false
	runas := []rune(s)
	for i := 0; i < len(runas); i++ {
		r := runas[i]
		switch {
		case entre && r == '"' && i+1 < len(runas) && runas[i+1] == '"':
			campo.WriteRune('"')
			i++
		case entre && r == '"':
			entre = false
		case entre:
			campo.WriteRune(r)
		case r == '"':
			entre = true
		case r == delim:
			fila = append(fila, campo.String())
			campo.Reset()
		case r == '\n':
			filas = append(filas, append(fila, campo.String()))
			fila = nil
			campo.Reset()
		default:
			campo.WriteRune(r)
		}
	}
	if campo.Len() > 0 || len(fila) > 0 {
		filas = append(filas, append(fila, campo.String()))
	}
	return filas
}

// ParsearOrigen es `parseMergeSource`: la primera fila son los campos —uno sin nombre es
// «Campo N»— y las demás, los registros, sin las filas en blanco y con ” en lo que falte.
func ParsearOrigen(texto string) Origen {
	limpio := strings.TrimPrefix(texto, "\uFEFF")
	if strings.TrimSpace(limpio) == "" {
		return Origen{Campos: []string{}, Registros: []map[string]string{}}
	}
	primera, _, _ := strings.Cut(limpio, "\n")
	filas := filasDelimitadas(limpio, delimitador(primera))
	o := Origen{Campos: []string{}, Registros: []map[string]string{}}
	if len(filas) == 0 {
		return o
	}
	for i, h := range filas[0] {
		if h = strings.TrimSpace(h); h == "" {
			h = "Campo " + strconv.Itoa(i+1)
		}
		o.Campos = append(o.Campos, h)
	}
	for _, f := range filas[1:] {
		if !slices.ContainsFunc(f, func(c string) bool { return strings.TrimSpace(c) != "" }) {
			continue
		}
		reg := map[string]string{}
		for i, c := range o.Campos {
			reg[c] = ""
			if i < len(f) {
				reg[c] = f[i]
			}
		}
		o.Registros = append(o.Registros, reg)
	}
	return o
}

// InsertarCampoCombinacion es `insertMergeField`: la ficha del campo en pos, UNA runa.
func (e *Edicion) InsertarCampoCombinacion(pos int, campo string) {
	span := elemento("span", xhtml.Attribute{Key: "data-mergefield", Val: campo}, xhtml.Attribute{Key: "contenteditable", Val: "false"})
	span.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "«" + campo + "»"})
	pos = max(0, min(pos, len(e.runas)))
	cadena := append(slices.Clip(fueraDeMarcador(e.hereda(pos))), span)
	e.runas = slices.Insert(e.runas, pos, ObjetoRuna)
	e.cadenas = slices.Insert(e.cadenas, pos, cadena)
	e.hayPendiente = false
	e.reescribir()
}

// reCampo es `MERGE_FIELD_RE`: el marcador entero, con su nombre (escapado) en el grupo.
var reCampo = regexp.MustCompile(`(?is)<span\b[^>]*\bdata-mergefield="([^"]*)"[^>]*>.*?</span>`)

var desescapar = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&apos;", "'")

// SustituirCampos es `substituteMergeFields`: cada marcador por el valor del registro,
// escapado; un campo que no está, por nada.
func SustituirCampos(html string, reg map[string]string) string {
	return reCampo.ReplaceAllStringFunc(html, func(m string) string {
		v, ok := reg[desescapar.Replace(reCampo.FindStringSubmatch(m)[1])]
		if !ok {
			return ""
		}
		return escaparWeb.Replace(v)
	})
}

// saltoCombinacion es el `PAGE_BREAK_HTML` de Correspondencia.tsx: sin el párrafo de detrás.
const saltoCombinacion = `<div data-page-break="" contenteditable="false" ` +
	`style="page-break-after:always;break-after:page;border-top:2px dashed #cbd5e1;margin:1.5em 0"></div>`

// Combinado es `buildMerged`: una copia del documento —sin el origen embebido— por
// registro, con los campos sustituidos, separadas por un salto de página.
func (d *Documento) Combinado(o Origen) string {
	copia := Parse(d.cuerpoHTML())
	copia.PonerOrigen(nil)
	cuerpo := copia.cuerpoHTML()
	partes := make([]string, len(o.Registros))
	for i, r := range o.Registros {
		partes[i] = SustituirCampos(cuerpo, r)
	}
	return strings.Join(partes, saltoCombinacion)
}

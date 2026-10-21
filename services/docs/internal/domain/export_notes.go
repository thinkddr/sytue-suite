// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func (w *docxWriter) noteParts() (map[string]string, string) {
	parts := map[string]string{}
	var overrides strings.Builder
	for _, class := range []string{"footnote", "endnote"} {
		w.body.Reset()
		for i, note := range w.notes {
			if note.class != class {
				continue
			}
			w.body.WriteString(`<w:` + class + ` w:id="` + strconv.Itoa(i+1) + `"><w:p><w:r><w:` + class + `Ref/></w:r>`)
			w.runs(note.runs)
			w.body.WriteString(`</w:p></w:` + class + `>`)
		}
		if w.body.Len() == 0 {
			continue
		}
		name := class + "s.xml"
		parts["word/"+name] = xmlHeader + `<w:` + class + `s xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` + w.body.String() + `</w:` + class + `s>`
		id := "rId" + strconv.Itoa(w.nextRel)
		w.nextRel++
		w.relDefs = append(w.relDefs, `<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/`+class+`s" Target="`+name+`"/>`)
		overrides.WriteString(`<Override PartName="/word/` + name + `" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.` + class + `s+xml"/>`)
	}
	return parts, overrides.String()
}

// PDF y texto plano siguen mostrando la sección del editor; OOXML/ODF trasladan
// sus notas a las partes semánticas del formato, sin cambiar los otros destinos.
func officeDocument(s *State, comments []Comment) xdoc {
	if h := strings.TrimSpace(s.documentHTML()); h != "" {
		if d := parseHTMLDoc(h, true); len(d.blocks) > 0 || d.header != nil || d.footer != nil {
			d.comments, d.byMark, d.points = exportComments(comments, d.marks)
			return d
		}
	}
	d := xdoc{blocks: documentBlocks(KindDocument, s)}
	d.comments, d.byMark, d.points = exportComments(comments, nil)
	return d
}

// Solo consumir cuerpos de nota que tengan una llamada real. Las notas huérfanas
// y los marcadores rotos conservan su texto ordinario en vez de desaparecer.
func collectHTMLNotes(nodes []*html.Node) (map[string]*xnote, map[*html.Node]bool) {
	refs := map[string]string{}
	walkNoteNodes(nodes, 0, false, func(n *html.Node, _ bool) {
		if n.DataAtom == atom.Sup {
			if id := attr(n, "data-footnote-ref"); id != "" {
				class := "footnote"
				if attr(n, "data-note-class") == "endnote" {
					class = "endnote"
				}
				refs[id] = class
			}
		}
	})
	notes := map[string]*xnote{}
	bodies := map[*html.Node]bool{}
	walkNoteNodes(nodes, 0, false, func(n *html.Node, inNotes bool) {
		if !inNotes || n.DataAtom != atom.Li {
			return
		}
		id, ok := strings.CutPrefix(attr(n, "id"), "fn-")
		class := refs[id]
		if !ok || class == "" || notes[id] != nil {
			return
		}
		note := noteFromHTML(n, class)
		if note == nil {
			return
		}
		notes[id] = note
		bodies[n] = true
	})
	return notes, bodies
}

func noteFromHTML(n *html.Node, class string) *xnote {
	// Como el cuerpo de una exportación a OOXML/ODF: el formato del texto de la nota también viaja.
	b := &rtBuilder{office: true}
	b.children(n, xrun{})
	b.flush()
	note := &xnote{class: class}
	for _, block := range b.blocks {
		if block.table != nil || block.hr {
			return nil // conservar una nota compleja en vez de degradarla
		}
		if block.para == nil {
			continue
		}
		if len(note.runs) > 0 {
			note.runs = append(note.runs, xrun{brk: true})
		}
		note.runs = append(note.runs, block.para.runs...)
	}
	if len(note.runs) == 0 {
		return nil
	}
	return note
}

func walkNoteNodes(nodes []*html.Node, depth int, inNotes bool, visit func(*html.Node, bool)) {
	if depth >= maxWalkDepth {
		return
	}
	for _, n := range nodes {
		inside := inNotes
		if n.DataAtom == atom.Section {
			for _, a := range n.Attr {
				inside = inside || a.Key == "data-footnotes"
			}
		}
		visit(n, inside)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkNoteNodes([]*html.Node{c}, depth+1, inside, visit)
		}
	}
}

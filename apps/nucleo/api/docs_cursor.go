// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"encoding/json"
	"math"
	"unicode/utf16"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los cursores de los demás por el canal en vivo: el contrato de `state/cursors.ts`. El
// servicio no mira lo que lleva un cursor (services/docs/internal/collab/protocol.go lo
// trata como carga opaca y lo difunde tal cual), así que lo que valida es cada cliente: lo
// que llega de otro se lee igual que en la web y lo que no encaja se descarta sin romper.

// Cursor es la posición de cursor o selección de un participante, por tipo de documento:
// `doc` (offsets del texto visible y, si van, los caracteres a los que se ancla), `sheet`
// (rango de celdas y las identidades de hoja, filas y columnas) o `slide` (diapositiva y
// cajas seleccionadas). Solo cuentan los campos de su `Kind`.
type Cursor struct {
	Kind string

	// doc
	Anchor, Focus int
	// Puntos ancla el rango a los caracteres (coedicion.PuntosDeOffsets); nil en un
	// emisor que no los manda: entonces mandan los offsets.
	Puntos *coedicion.Puntos

	// sheet
	Sheet, C1, R1, C2, R2 int
	SheetID               string
	Rows, Cols            *[2]string

	// slide
	Slide int
	Boxes []string
}

// MarshalJSON escribe solo los campos del tipo, con los nombres de la web.
func (c Cursor) MarshalJSON() ([]byte, error) {
	m := map[string]any{"kind": c.Kind}
	switch c.Kind {
	case "doc":
		m["anchor"], m["focus"] = c.Anchor, c.Focus
		if c.Puntos != nil {
			m["anchorDot"], m["focusDot"] = c.Puntos.AnchorDot, c.Puntos.FocusDot
		}
	case "sheet":
		m["sheet"], m["c1"], m["r1"], m["c2"], m["r2"] = c.Sheet, c.C1, c.R1, c.C2, c.R2
		if c.SheetID != "" {
			m["sheetId"] = c.SheetID
		}
		if c.Rows != nil {
			m["rows"] = c.Rows
		}
		if c.Cols != nil {
			m["cols"] = c.Cols
		}
	case "slide":
		m["slide"] = c.Slide
		m["boxes"] = append([]string{}, c.Boxes...) // `[]`, nunca `null`
	}
	return json.Marshal(m)
}

// LeerCursor es `parseCursorPos`: la posición que manda otro cliente (JSON opaco) o false
// si no encaja. Un `kind` desconocido se ignora: un emisor más nuevo no rompe a este.
func LeerCursor(crudo json.RawMessage) (Cursor, bool) {
	var o map[string]any
	if json.Unmarshal(crudo, &o) != nil || o == nil {
		return Cursor{}, false
	}
	switch o["kind"] {
	case "sheet":
		return cursorDeHoja(o)
	case "doc":
		return cursorDeDocumento(o)
	case "slide":
		return cursorDeDiapositiva(o)
	}
	return Cursor{}, false
}

func cursorDeHoja(o map[string]any) (Cursor, bool) {
	var n [5]int
	for i, k := range []string{"sheet", "c1", "r1", "c2", "r2"} {
		v, ok := entero(o[k])
		if !ok {
			return Cursor{}, false
		}
		n[i] = v
	}
	c := Cursor{Kind: "sheet", Sheet: n[0], C1: n[1], R1: n[2], C2: n[3], R2: n[4]}
	// Las anclas son opcionales (cliente anterior): sin ellas mandan los índices.
	if s, ok := o["sheetId"].(string); ok && largoJS(s) <= 128 {
		c.SheetID = s
	}
	c.Rows, c.Cols = parDeIDs(o["rows"]), parDeIDs(o["cols"])
	return c, true
}

func cursorDeDocumento(o map[string]any) (Cursor, bool) {
	anchor, oka := entero(o["anchor"])
	focus, okf := entero(o["focus"])
	if !oka || !okf {
		return Cursor{}, false
	}
	c := Cursor{Kind: "doc", Anchor: anchor, Focus: focus}
	// Los puntos van juntos: con uno solo no se puede anclar el rango.
	pa, oka := puntoDeCursor(o["anchorDot"])
	pf, okf := puntoDeCursor(o["focusDot"])
	if oka && okf {
		c.Puntos = &coedicion.Puntos{AnchorDot: pa, FocusDot: pf}
	}
	return c, true
}

func cursorDeDiapositiva(o map[string]any) (Cursor, bool) {
	slide, ok := entero(o["slide"])
	if !ok {
		return Cursor{}, false
	}
	boxes := []string{}
	if l, ok := o["boxes"].([]any); ok {
		for _, b := range l {
			if s, ok := b.(string); ok {
				boxes = append(boxes, s)
			}
		}
	}
	return Cursor{Kind: "slide", Slide: slide, Boxes: boxes}, true
}

// entero es `int()` de la web: un número finito, truncado. Fuera de lo que un número de
// JavaScript guarda sin perder unidades no es una posición: se descarta.
func entero(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 1<<53 {
		return 0, false
	}
	return int(math.Trunc(f)), true
}

// largoJS es `.length` de una cadena de JavaScript: unidades UTF-16.
func largoJS(s string) int { return len(utf16.Encode([]rune(s))) }

// parDeIDs es `par()`: dos ids no vacíos de hasta 64 unidades, o nil.
func parDeIDs(v any) *[2]string {
	l, ok := v.([]any)
	if !ok || len(l) != 2 {
		return nil
	}
	var par [2]string
	for i, x := range l {
		s, ok := x.(string)
		if !ok || s == "" || largoJS(s) > 64 {
			return nil
		}
		par[i] = s
	}
	return &par
}

// puntoDeCursor es `punto()`: un carácter de la secuencia (actor de hasta 128 unidades y
// contador no negativo).
func puntoDeCursor(v any) (coedicion.Punto, bool) {
	o, ok := v.(map[string]any)
	if !ok {
		return coedicion.Punto{}, false
	}
	a, oka := o["a"].(string)
	c, okc := entero(o["c"])
	if !oka || largoJS(a) > 128 || !okc || c < 0 {
		return coedicion.Punto{}, false
	}
	return coedicion.Punto{A: a, C: uint64(c)}, true
}

// DocPresencia es un participante en la sala. En el roster (`presence` con lista) solo
// trae `Sub`; en la difusión de un cursor trae además nombre y posición.
type DocPresencia struct {
	Sub  string `json:"sub"`
	Name string `json:"name,omitempty"`
	// Cursor es la posición ya validada; nil si no venía o no encajaba.
	Cursor *Cursor `json:"-"`
	// Color es el token de su color (`avatar-1`…`avatar-6`), el `peerColor` de la web por
	// el `sub`: el mismo que su avatar y sus marcas.
	Color string `json:"-"`
}

// UnmarshalJSON lee el `cursor` opaco con LeerCursor: una posición que no encaja se
// queda en nil sin tirar el resto del mensaje.
func (p *DocPresencia) UnmarshalJSON(b []byte) error {
	var crudo struct {
		Sub    string          `json:"sub"`
		Name   string          `json:"name"`
		Cursor json.RawMessage `json:"cursor"`
	}
	if err := json.Unmarshal(b, &crudo); err != nil {
		return err
	}
	*p = DocPresencia{Sub: crudo.Sub, Name: crudo.Name}
	if crudo.Sub != "" {
		p.Color = tokens.Persona(crudo.Sub)
	}
	if len(crudo.Cursor) > 0 && !bytes.Equal(crudo.Cursor, []byte("null")) {
		if c, ok := LeerCursor(crudo.Cursor); ok {
			p.Cursor = &c
		}
	}
	return nil
}

// cursorDePar es `parsePeerCursor`: la difusión de un cursor solo vale con su `sub` y una
// posición que encaje; si no, se descarta entera (no hay a quién ni dónde pintarla).
func cursorDePar(p *DocPresencia) *DocPresencia {
	if p == nil || p.Sub == "" || p.Cursor == nil {
		return nil
	}
	return p
}

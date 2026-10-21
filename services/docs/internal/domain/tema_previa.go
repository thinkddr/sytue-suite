// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"slices"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// CambioTema es una línea del diff legible de «Actualizar a la marca vigente»: qué
// parte de la presentación cambiará, en cuántos sitios y de qué valor a cuál. Nunca
// habla de contenido: aplicar un tema no lo toca.
type CambioTema struct {
	// Scope: headings, table_headers, body_font (documento), sheet_header (hoja) o
	// slide_theme (presentación). La web lo traduce; el servicio no manda textos.
	Scope string `json:"scope"`
	// Sheet es el nombre de la hoja, solo en sheet_header.
	Sheet string `json:"sheet,omitempty"`
	Count int    `json:"count"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// PrevisualizarTema dice qué cambiaría AplicarTema(kind, contenido, t) sin escribir nada.
// Lista vacía = el contenido ya tiene ese tema. Es la misma función que escribe (se compara
// el resultado con el original), así que lo que se enseña es exactamente lo que se aplica.
func PrevisualizarTema(kind Kind, contenido string, t TemaComun) ([]CambioTema, error) {
	nuevo, err := AplicarTema(kind, contenido, t)
	if err != nil {
		return nil, err
	}
	if nuevo == contenido {
		return []CambioTema{}, nil
	}
	var cambios []CambioTema
	switch kind {
	case KindDocument:
		cambios, err = cambiosTexto(contenido, nuevo)
	case KindSpreadsheet:
		cambios, err = cambiosHoja(contenido, nuevo)
	default:
		cambios, err = cambiosDiapos(contenido, nuevo)
	}
	if err != nil {
		return nil, err
	}
	// El logotipo de la organización: se pone, se cambia por el vigente o se quita.
	if antes, despues := versionLogo(kind, contenido), versionLogo(kind, nuevo); antes != despues {
		c := CambioTema{Scope: "logo", Count: 1}
		if antes != "" {
			c.From = "logo"
		}
		if despues != "" {
			c.To = "logo"
		}
		cambios = append(cambios, c)
	}
	return cambios, nil
}

func fragmento(contenido string) ([]*xhtml.Node, error) {
	cuerpo := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodos, err := xhtml.ParseFragment(strings.NewReader(contenido), cuerpo)
	if err != nil {
		return nil, fmt.Errorf("%w: HTML ilegible: %w", ErrInvalidFormat, err)
	}
	return nodos, nil
}

// valorEstilo es el valor de la propiedad CSS `nombre` en un atributo style ("" si no está).
func valorEstilo(style, nombre string) string {
	for _, p := range splitEstilo(style) {
		if nombrePropiedad(p) == nombre {
			_, v, _ := strings.Cut(p, ":")
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// cambiosTexto recorre a la vez el HTML de antes y el de después (solo cambian atributos,
// la forma del árbol es la misma) y agrupa por tipo de elemento.
func cambiosTexto(antes, despues string) ([]CambioTema, error) {
	a, err := fragmento(antes)
	if err != nil {
		return nil, err
	}
	d, err := fragmento(despues)
	if err != nil {
		return nil, err
	}
	grupos := map[string]*CambioTema{}
	var andar func(x, y *xhtml.Node)
	andar = func(x, y *xhtml.Node) {
		if x.Type == xhtml.ElementNode && attrVal(x, "style") != attrVal(y, "style") {
			scope, prop := "body_font", "font-family"
			switch x.DataAtom {
			case atom.Th:
				scope, prop = "table_headers", "background-color"
			case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				scope, prop = "headings", "color"
			default: // párrafos, listas, celdas y citas: la fuente del cuerpo
			}
			g := grupos[scope]
			if g == nil {
				g = &CambioTema{Scope: scope, From: valorEstilo(attrVal(x, "style"), prop), To: valorEstilo(attrVal(y, "style"), prop)}
				grupos[scope] = g
			}
			g.Count++
		}
		for cx, cy := x.FirstChild, y.FirstChild; cx != nil && cy != nil; cx, cy = cx.NextSibling, cy.NextSibling {
			andar(cx, cy)
		}
	}
	// El párrafo del logo se pone o se quita entero: fuera de la comparación nodo a nodo
	// (descuadraría las dos listas) y contado aparte (PrevisualizarTema).
	sinLogo := func(ns []*xhtml.Node) []*xhtml.Node {
		return slices.DeleteFunc(ns, func(n *xhtml.Node) bool {
			return n.Type == xhtml.ElementNode && attrVal(n, attrLogoMarca) != ""
		})
	}
	a, d = sinLogo(a), sinLogo(d)
	for i := 0; i < len(a) && i < len(d); i++ {
		andar(a[i], d[i])
	}
	out := []CambioTema{}
	for _, scope := range []string{"headings", "table_headers", "body_font"} {
		if g := grupos[scope]; g != nil {
			out = append(out, *g)
		}
	}
	return out, nil
}

func cambiosHoja(antes, despues string) ([]CambioTema, error) {
	a, err := decodificarJSON(antes)
	if err != nil {
		return nil, err
	}
	d, err := decodificarJSON(despues)
	if err != nil {
		return nil, err
	}
	ha, _ := a["sheets"].([]any)
	hd, _ := d["sheets"].([]any)
	out := []CambioTema{}
	for i := 0; i < len(ha) && i < len(hd); i++ {
		oa, _ := ha[i].(map[string]any)
		od, _ := hd[i].(map[string]any)
		sa, _ := oa["styles"].(map[string]any)
		sd, _ := od["styles"].(map[string]any)
		c := CambioTema{Scope: "sheet_header"}
		c.Sheet, _ = oa["name"].(string)
		for ref := range unir(sa, sd) {
			ea, _ := sa[ref].(map[string]any)
			ed, _ := sd[ref].(map[string]any)
			if fmt.Sprint(ea) == fmt.Sprint(ed) {
				continue
			}
			if c.Count == 0 || ref < c.From { // la celda más a la izquierda y arriba da el ejemplo: estable
				c.From, _ = ea["fill"].(string)
				c.To, _ = ed["fill"].(string)
			}
			c.Count++
		}
		if c.Count > 0 {
			out = append(out, c)
		}
	}
	return out, nil
}

func unir(a, b map[string]any) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

func cambiosDiapos(antes, despues string) ([]CambioTema, error) {
	a, err := decodificarJSON(antes)
	if err != nil {
		return nil, err
	}
	d, err := decodificarJSON(despues)
	if err != nil {
		return nil, err
	}
	de, hasta := descTemaDiapos(a), descTemaDiapos(d)
	if de == hasta {
		return []CambioTema{}, nil // solo cambió el logo
	}
	return []CambioTema{{Scope: "slide_theme", Count: 1, From: de, To: hasta}}, nil
}

// descTemaDiapos nombra el tema de una presentación; el propio de la marca (siempre
// «custom-marca») se distingue por su fondo, que es lo que cambia entre versiones.
func descTemaDiapos(m map[string]any) string {
	id, _ := m["theme"].(string)
	if id != idTemaMarcaDiapos {
		return id
	}
	propios, _ := m["themes"].([]any)
	for _, v := range propios {
		if o, _ := v.(map[string]any); o != nil && o["id"] == id {
			if bg, _ := o["bg"].(string); bg != "" {
				return bg
			}
		}
	}
	return id
}

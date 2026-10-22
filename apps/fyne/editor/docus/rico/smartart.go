// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rico

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// El SmartArt del documento (`editor/smartart.ts`): `<figure data-smartart data-layout
// data-items contenteditable="false">` con el SVG horneado. La geometría es la de las
// diapositivas, en `editor/figuras`.

// SmartArt es la disposición y los elementos de un diagrama.
type SmartArt struct {
	Disposicion string
	Elementos   []ElementoSmart
}

// ElementoSmart es `SmartArtItem`: su texto y, en la jerarquía, su sangría.
type ElementoSmart struct {
	Texto string `json:"text"`
	Nivel int    `json:"level,omitempty"`
}

// AnchoSmart es VIEW_W: el ancho del viewBox. La figura va al 100 % del folio.
const AnchoSmart = 640

// SanearDisposicion es `sanitizeSmartArtLayout`.
func SanearDisposicion(d string) string {
	if figuras.DisposicionesSmart[d] {
		return d
	}
	return "list"
}

// LeerElementos es `parseSmartArtItems`: lo que no se lee es una lista vacía, y cada
// entrada da su texto (” si falta) y su nivel entero, nunca negativo.
func LeerElementos(crudo string) []ElementoSmart {
	var datos []any
	if json.Unmarshal([]byte(crudo), &datos) != nil {
		return []ElementoSmart{}
	}
	out := make([]ElementoSmart, 0, len(datos))
	for _, d := range datos {
		o, _ := d.(map[string]any)
		e := ElementoSmart{}
		e.Texto, _ = o["text"].(string)
		if n, ok := o["level"].(float64); ok && !math.IsNaN(n) {
			e.Nivel = max(0, int(math.Floor(n)))
		}
		out = append(out, e)
	}
	return out
}

// ElementosIniciales es DEFAULT_SMARTART_ITEMS, con los textos que la web no traduce.
func ElementosIniciales(disposicion string) []ElementoSmart {
	switch disposicion {
	case "process":
		return []ElementoSmart{{Texto: "Inicio"}, {Texto: "Desarrollo"}, {Texto: "Resultado"}}
	case "cycle":
		return []ElementoSmart{{Texto: "Planificar"}, {Texto: "Hacer"}, {Texto: "Revisar"}, {Texto: "Ajustar"}}
	case "hierarchy":
		return []ElementoSmart{{Texto: "Dirección"}, {Texto: "Equipo A", Nivel: 1}, {Texto: "Equipo B", Nivel: 1}}
	}
	return []ElementoSmart{{Texto: "Primer elemento"}, {Texto: "Segundo elemento"}, {Texto: "Tercer elemento"}}
}

// AltoSmart es `viewHeight`: crece con los elementos donde se apilan (la lista) o por
// filas (la jerarquía); fijo donde se reparten a lo ancho o en anillo.
func (sa SmartArt) AltoSmart() float64 {
	n := max(1, len(sa.Elementos))
	switch sa.Disposicion {
	case "process":
		return 170
	case "cycle":
		return 380
	case "hierarchy":
		hondo := 0
		for _, e := range sa.Elementos {
			hondo = max(hondo, e.Nivel)
		}
		return figuras.Acotar(float64(min(hondo, 6)+1)*130, 170, 900)
	}
	return figuras.Acotar(float64(n)*52+24, 120, 900)
}

// Geometria son los nodos y los conectores en el viewBox de AnchoSmart × AltoSmart.
func (sa SmartArt) Geometria() ([]figuras.NodoSmart, []figuras.ConectorSmart) {
	niveles := make([]int, len(sa.Elementos))
	for i, e := range sa.Elementos {
		niveles[i] = e.Nivel
	}
	return figuras.GeometriaSmart(sa.Disposicion, niveles, AnchoSmart, sa.AltoSmart())
}

// HTML es `smartArtFigureHtml`. `aria` es `smartart.aria` y `vacio`, `smartart.empty`,
// ya traducidos.
func (sa SmartArt) HTML(aria, vacio string) string {
	elems := slices.Clone(sa.Elementos)
	if elems == nil {
		elems = []ElementoSmart{}
	}
	var datos bytes.Buffer
	enc := json.NewEncoder(&datos)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(elems) //nolint:errcheck // una lista de estructuras siempre se codifica
	return `<figure data-smartart data-layout="` + escapar(sa.Disposicion) + `" data-items="` + escapar(strings.TrimSuffix(datos.String(), "\n")) +
		`" contenteditable="false" style="margin:1em auto;width:100%">` + sa.svgWeb(aria, vacio) + `</figure>`
}

// svgWeb es `smartArtSvg`: conectores y nodos en `currentColor`, con el texto en un
// `foreignObject` por nodo.
func (sa SmartArt) svgWeb(aria, vacio string) string {
	n2 := figuras.N2
	h := sa.AltoSmart()
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %s" width="100%%" style="display:block;height:auto" role="img" aria-label="%s">`,
		AnchoSmart, n2(h), escapar(aria))
	nodos, con := sa.Geometria()
	if len(nodos) == 0 {
		fmt.Fprintf(&b, `<text x="%d" y="%s" text-anchor="middle" dominant-baseline="middle" fill="currentColor" fill-opacity="0.6" `+
			`font-family="var(--font-sans,Inter,sans-serif)" font-size="16">%s</text></svg>`, AnchoSmart/2, n2(h/2), escapar(vacio))
		return b.String()
	}
	conectores, formas := figuras.MarcasSmart(nodos, con, AnchoSmart, h, "currentColor")
	b.WriteString(strings.Join(conectores, ""))
	vineta := sa.Disposicion == "list"
	for k, nd := range nodos {
		texto := ""
		if nd.I < len(sa.Elementos) {
			texto = sa.Elementos[nd.I].Texto
		}
		b.WriteString("<g>" + formas[k] + textoNodoWeb(nd, texto, vineta) + "</g>")
	}
	b.WriteString("</svg>")
	return b.String()
}

// textoNodoWeb es el `foreignObject` de `nodeSvg`.
func textoNodoWeb(nd figuras.NodoSmart, texto string, vineta bool) string {
	n2 := figuras.N2
	py, px := figuras.RellenoNodo(nd)
	justifica, alinea, punto := "center", "center", ""
	if vineta {
		justifica, alinea, punto = "flex-start", "left", `<span aria-hidden="true" style="opacity:0.55;flex:0 0 auto">&#8226;</span>`
	}
	return fmt.Sprintf(`<foreignObject x="%s" y="%s" width="%s" height="%s"><div xmlns="http://www.w3.org/1999/xhtml" style="`+
		`box-sizing:border-box;width:100%%;height:100%%;display:flex;align-items:center;justify-content:%s;text-align:%s;gap:0.4em;`+
		`padding:%spx %spx;font-family:var(--font-sans,Inter,sans-serif);font-size:%spx;font-weight:500;line-height:1.2;color:currentColor;`+
		`overflow:hidden;word-break:break-word">%s<span style="min-width:0">%s</span></div></foreignObject>`,
		n2(nd.X), n2(nd.Y), n2(nd.W), n2(nd.H), justifica, alinea, n2(py), n2(px), n2(figuras.LetraNodo(nd)), punto, escapar(texto))
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/figuras"
)

// # Las formas, los gráficos, los diagramas y los conectores son el MISMO SVG
//
// La web pinta todo lo que no es texto ni imagen con un `<svg>` en línea cuyo viewBox es
// la caja en píxeles del escenario (`renderBox`, `SlideChartView`, `SmartArtView`,
// `ConnectorLayer`). Aquí se escribe ese mismo SVG, número a número, y lo rasteriza
// `iconos.DeSVG` a píxeles de dispositivo: no hay una segunda geometría que se separe
// de la de la web con el tiempo. El texto va aparte (`oksvg` no pinta `<text>`): lo
// coloca el lienzo con `canvas.Text` en la línea base que el SVG le habría dado.

// Colores de contenido por defecto de una forma: SHAPE_FILL y SHAPE_STROKE.
const (
	rellenoForma = "#475569" // CONTENT_PALETTE[1]
	trazoForma   = "#0A0A0A" // CONTENT_PALETTE[0]
)

var n2 = figuras.N2

// colorSVG es el color de un atributo `fill`/`stroke`: el hex tal cual y, para
// 'transparent' (sin relleno), `none`, que es lo que `oksvg` entiende. Lo que no sea un
// hex de contenido cae al de por defecto, como hace el saneado de la web al leer.
func colorSVG(c, porDefecto string) string {
	if c == "transparent" {
		return "none"
	}
	if _, ok := editor.Hex(c); ok {
		return c
	}
	return porDefecto
}

// cabeceraSVG abre el `<svg>` con el viewBox en px de la caja.
func cabeceraSVG(b *strings.Builder, w, h float64) {
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">`,
		n2(w), n2(h), n2(w), n2(h))
}

// svgForma es la rama `kind === 'shape'` de `renderBox`: el SVG de la forma en una
// caja de w×h px del escenario.
func svgForma(c Caja, w, h float64) []byte {
	relleno := colorSVG(c.Relleno, rellenoForma)
	trazo := colorSVG(c.Trazo, trazoForma)
	var b strings.Builder
	cabeceraSVG(&b, w, h)
	if pts := figuras.Vertices(c.Forma, w, h); len(pts) > 0 {
		fmt.Fprintf(&b, `<polygon points="%s" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`,
			figuras.PuntosSVG(pts), relleno, trazo)
	} else {
		switch c.Forma {
		case "cylinder":
			d := figuras.MedidasCilindro(w, h)
			fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`,
				d.Cuerpo(), relleno, trazo)
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round"/>`,
				d.Costura(), trazo)
		case "ellipse":
			fmt.Fprintf(&b, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
				n2(w/2), n2(h/2), n2(max(1, w/2-1)), n2(max(1, h/2-1)), relleno, trazo)
		case "line", "arrow":
			fmt.Fprintf(&b, `<line x1="2" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="3" stroke-linecap="round"/>`,
				n2(h/2), n2(w-2), n2(h/2), trazo)
			if c.Forma == "arrow" {
				fmt.Fprintf(&b, `<path d="M%s %s L%s %s L%s %s" fill="none" stroke="%s" stroke-width="3"`+
					` stroke-linecap="round" stroke-linejoin="round"/>`,
					n2(w-14), n2(h/2-7), n2(w-2), n2(h/2), n2(w-14), n2(h/2+7), trazo)
			}
		default:
			fmt.Fprintf(&b, `<rect x="1" y="1" width="%s" height="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
				n2(max(1, w-2)), n2(max(1, h-2)), relleno, trazo)
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// punto es un punto del escenario, en px.
type punto struct{ x, y float64 }

var acotar = figuras.Acotar

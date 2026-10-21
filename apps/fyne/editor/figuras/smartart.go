// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package figuras

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// El diagrama SmartArt: la geometría de `slides/smartart.ts`, pura, que la web usa en
// las diapositivas y en los documentos. Los nodos y los conectores van en el SVG; el texto
// de cada nodo lo coloca quien pinta (`editor.TextoNodoSmart`), que `oksvg` no pinta el
// `foreignObject`.

// NodoSmart es `SmartArtNode`.
type NodoSmart struct {
	I          int
	X, Y, W, H float64
	Elipse     bool
}

// ConectorSmart es `SmartArtConnector`: flecha recta o codo sin punta.
type ConectorSmart struct {
	X1, Y1, X2, Y2 float64
	Codo           bool
}

// DisposicionesSmart son SMARTART_IDS; lo que no esté aquí se pinta como lista.
var DisposicionesSmart = map[string]bool{"list": true, "process": true, "cycle": true, "hierarchy": true}

// GeometriaSmart es `smartArtLayout`.
func GeometriaSmart(disp string, niveles []int, vw, vh float64) ([]NodoSmart, []ConectorSmart) {
	n := len(niveles)
	if n == 0 || vw <= 0 || vh <= 0 {
		return nil, nil
	}
	pad := Acotar(min(vw, vh)*0.06, 4, 22)
	switch disp {
	case "process":
		return proceso(n, vw, vh, pad)
	case "cycle":
		return ciclo(n, vw, vh, pad)
	case "hierarchy":
		return jerarquia(niveles, vw, vh, pad)
	}
	return lista(n, vw, vh, pad), nil
}

func lista(n int, vw, vh, pad float64) []NodoSmart {
	ancho := max(1, vw-2*pad)
	hay := max(1, vh-2*pad)
	hueco := Acotar(hay*0.05, 2, 14)
	alto := max(1, (hay-hueco*float64(n-1))/float64(n))
	out := make([]NodoSmart, n)
	for k := range n {
		out[k] = NodoSmart{I: k, X: pad, Y: pad + float64(k)*(alto+hueco), W: ancho, H: alto}
	}
	return out
}

func proceso(n int, vw, vh, pad float64) ([]NodoSmart, []ConectorSmart) {
	alto := max(1, vh-2*pad)
	hay := max(1, vw-2*pad)
	hueco := Acotar(hay*0.06, 6, 48)
	ancho := max(1, (hay-hueco*float64(n-1))/float64(n))
	cy := vh / 2
	nodos := make([]NodoSmart, n)
	for k := range n {
		nodos[k] = NodoSmart{I: k, X: pad + float64(k)*(ancho+hueco), Y: pad, W: ancho, H: alto}
	}
	con := make([]ConectorSmart, 0, n-1)
	for k := range n - 1 {
		x1 := pad + float64(k)*(ancho+hueco) + ancho
		con = append(con, ConectorSmart{X1: x1, Y1: cy, X2: x1 + hueco, Y2: cy})
	}
	return nodos, con
}

// alcanceElipse es `ellipseReach`: del centro al borde en la dirección (ux, uy).
func alcanceElipse(rx, ry, ux, uy float64) float64 {
	d := math.Hypot(uy*rx, ux*ry)
	if d == 0 {
		return 0
	}
	return rx * ry / d
}

func ciclo(n int, vw, vh, pad float64) ([]NodoSmart, []ConectorSmart) {
	cx, cy := vw/2, vh/2
	lado := min(vw, vh)
	f := 0.2
	switch {
	case n <= 4:
		f = 0.32
	case n <= 6:
		f = 0.26
	}
	nw := Acotar(lado*f, 12, lado*0.5)
	nh := nw * 0.66
	if n == 1 {
		return []NodoSmart{{I: 0, X: cx - nw/2, Y: cy - nh/2, W: nw, H: nh, Elipse: true}}, nil
	}
	rx, ry := max(1, vw/2-pad-nw/2), max(1, vh/2-pad-nh/2)
	ncx, ncy := make([]float64, n), make([]float64, n)
	nodos := make([]NodoSmart, n)
	for k := range n {
		a := -math.Pi/2 + float64(k)*2*math.Pi/float64(n)
		ncx[k], ncy[k] = cx+rx*math.Cos(a), cy+ry*math.Sin(a)
		nodos[k] = NodoSmart{I: k, X: ncx[k] - nw/2, Y: ncy[k] - nh/2, W: nw, H: nh, Elipse: true}
	}
	con := make([]ConectorSmart, n)
	for k := range n {
		j := (k + 1) % n
		dx, dy := ncx[j]-ncx[k], ncy[j]-ncy[k]
		l := math.Hypot(dx, dy)
		if l == 0 {
			l = 1
		}
		ux, uy := dx/l, dy/l
		r := alcanceElipse(nw/2, nh/2, ux, uy)
		con[k] = ConectorSmart{X1: ncx[k] + ux*r, Y1: ncy[k] + uy*r, X2: ncx[j] - ux*r, Y2: ncy[j] - uy*r}
	}
	return nodos, con
}

// arbolSmart es `TreeNode`.
type arbolSmart struct {
	i, prof int
	hijos   []*arbolSmart
}

// bosque es `buildForest`: cada elemento cuelga del anterior con nivel ESTRICTAMENTE
// menor; los saltos de sangría se absorben.
func bosque(niveles []int) []*arbolSmart {
	var raices []*arbolSmart
	type apilado struct {
		nodo    *arbolSmart
		reclama int
	}
	var pila []apilado
	for i, nivel := range niveles {
		reclama := max(0, nivel)
		nodo := &arbolSmart{i: i}
		for len(pila) > 0 && pila[len(pila)-1].reclama >= reclama {
			pila = pila[:len(pila)-1]
		}
		if len(pila) > 0 {
			padre := pila[len(pila)-1].nodo
			nodo.prof = padre.prof + 1
			padre.hijos = append(padre.hijos, nodo)
		} else {
			raices = append(raices, nodo)
		}
		pila = append(pila, apilado{nodo, reclama})
	}
	return raices
}

func jerarquia(niveles []int, vw, vh, pad float64) ([]NodoSmart, []ConectorSmart) {
	raices := bosque(niveles)
	hojas, hondo := 0, 0
	var medir func(*arbolSmart)
	medir = func(a *arbolSmart) {
		hondo = max(hondo, a.prof)
		if len(a.hijos) == 0 {
			hojas++
		}
		for _, h := range a.hijos {
			medir(h)
		}
	}
	for _, r := range raices {
		medir(r)
	}
	hojas = max(1, hojas)
	filas := hondo + 1
	anchoHueco := max(1, vw-2*pad) / float64(hojas)
	altoFila := max(1, vh-2*pad) / float64(filas)
	nw := max(1, anchoHueco*0.86)
	nh := max(1, min(altoFila*0.58, nw*0.52))

	centros := map[int][2]float64{}
	var nodos []NodoSmart
	hoja := 0
	var colocar func(*arbolSmart)
	colocar = func(a *arbolSmart) {
		var cx float64
		if len(a.hijos) == 0 {
			cx = pad + (float64(hoja)+0.5)*anchoHueco
			hoja++
		} else {
			for _, h := range a.hijos {
				colocar(h)
			}
			cx = (centros[a.hijos[0].i][0] + centros[a.hijos[len(a.hijos)-1].i][0]) / 2
		}
		cy := pad + (float64(a.prof)+0.5)*altoFila
		centros[a.i] = [2]float64{cx, cy}
		nodos = append(nodos, NodoSmart{I: a.i, X: cx - nw/2, Y: cy - nh/2, W: nw, H: nh})
	}
	for _, r := range raices {
		colocar(r)
	}
	var con []ConectorSmart
	var enlazar func(*arbolSmart)
	enlazar = func(a *arbolSmart) {
		p := centros[a.i]
		for _, h := range a.hijos {
			c := centros[h.i]
			con = append(con, ConectorSmart{X1: p[0], Y1: p[1] + nh/2, X2: c[0], Y2: c[1] - nh/2, Codo: true})
			enlazar(h)
		}
	}
	for _, r := range raices {
		enlazar(r)
	}
	sort.Slice(nodos, func(a, b int) bool { return nodos[a].I < nodos[b].I })
	return nodos, con
}

// SVGSmart es el SVG de `SmartArtView` sin el texto: conectores primero y encima los
// nodos, en la tinta dada (`currentColor` en la web) con sus opacidades.
func SVGSmart(nodos []NodoSmart, con []ConectorSmart, vw, vh float64, tinta string) []byte {
	conectores, formas := MarcasSmart(nodos, con, vw, vh, tinta)
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + N2(vw) + " " + N2(vh) + `" width="` + N2(vw) + `" height="` + N2(vh) + `">` +
		strings.Join(conectores, "") + strings.Join(formas, "") + `</svg>`)
}

// MarcasSmart son las marcas del diagrama, una cadena por conector y otra por nodo, en el
// orden de los nodos: quien hornea el SVG de la web mete el texto de cada nodo tras la suya.
func MarcasSmart(nodos []NodoSmart, con []ConectorSmart, vw, vh float64, tinta string) (conectores, formas []string) {
	punta := Acotar(min(vw, vh)*0.03, 4, 12)
	for _, c := range con {
		if c.Codo {
			my := N2((c.Y1 + c.Y2) / 2)
			conectores = append(conectores, fmt.Sprintf(`<path d="M%s %sV%sH%sV%s" fill="none" stroke="%s" stroke-opacity="0.42"`+
				` stroke-width="1.5" stroke-linejoin="round"/>`, N2(c.X1), N2(c.Y1), my, N2(c.X2), N2(c.Y2), tinta))
			continue
		}
		ang := math.Atan2(c.Y2-c.Y1, c.X2-c.X1)
		conectores = append(conectores, fmt.Sprintf(`<g stroke="%s" stroke-opacity="0.5" stroke-width="2" fill="none">`+
			`<line x1="%s" y1="%s" x2="%s" y2="%s" stroke-linecap="round"/>`+
			`<path d="M%s %sL%s %sL%s %s" stroke-linecap="round" stroke-linejoin="round"/></g>`, tinta,
			N2(c.X1), N2(c.Y1), N2(c.X2), N2(c.Y2),
			N2(c.X2-punta*math.Cos(ang-math.Pi/7)), N2(c.Y2-punta*math.Sin(ang-math.Pi/7)), N2(c.X2), N2(c.Y2),
			N2(c.X2-punta*math.Cos(ang+math.Pi/7)), N2(c.Y2-punta*math.Sin(ang+math.Pi/7))))
	}
	for _, nd := range nodos {
		if nd.Elipse {
			formas = append(formas, fmt.Sprintf(`<ellipse cx="%s" cy="%s" rx="%s" ry="%s" fill="%s" fill-opacity="0.08" stroke="%s"`+
				` stroke-opacity="0.4" stroke-width="1.5"/>`,
				N2(nd.X+nd.W/2), N2(nd.Y+nd.H/2), N2(nd.W/2), N2(nd.H/2), tinta, tinta))
			continue
		}
		formas = append(formas, fmt.Sprintf(`<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s" fill-opacity="0.08"`+
			` stroke="%s" stroke-opacity="0.4" stroke-width="1.5"/>`,
			N2(nd.X), N2(nd.Y), N2(nd.W), N2(nd.H), N2(Acotar(min(nd.W, nd.H)*0.16, 2, 10)), tinta, tinta))
	}
	return conectores, formas
}

// LetraNodo es el `fontSize` del texto de un nodo.
func LetraNodo(nd NodoSmart) float64 { return Acotar(min(nd.H*0.32, nd.W*0.15), 7, 20) }

// RellenoNodo es el `padding` del texto de un nodo: vertical y horizontal.
func RellenoNodo(nd NodoSmart) (float64, float64) {
	return Acotar(nd.H*0.08, 1, 6), Acotar(nd.W*0.06, 3, 14)
}

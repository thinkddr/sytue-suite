// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

// # La transición «morph»
//
// Es la de `anim.ts` (`morphPairs`, `morphDelta`) y SlidesCanvas.tsx: al AVANZAR a una
// diapositiva con transición «morph», cada caja que ya estaba en la anterior —la misma por
// id o, si no, una con el mismo contenido— sale de donde estaba y con su tamaño, y llega a
// su sitio en 520 ms (`cubic-bezier(0.4, 0, 0.2, 1)`, el FLIP con el origen en la esquina
// de arriba a la izquierda); las nuevas entran con el fundido de 300 ms de la
// presentación, y las que ya no están, no se pintan.

// duracionMorph y curvaMorph son los de `.sytue-morph`.
const duracionMorph = 520 * time.Millisecond

var curvaMorph = curvaCSS(0.4, 0, 0.2, 1)

var reEtiquetaMorph = regexp.MustCompile(`<[^>]*>`)

// claveMorph es `morphKey`: por CONTENIDO. Dos cajas con la misma clave morfean.
func claveMorph(c Caja) string {
	switch c.Tipo {
	case "image":
		return "img:" + c.Src
	case "media":
		return "media:" + c.TipoMedio + ":" + c.Src
	case "shape":
		f := c.Forma
		if f == "" {
			f = "rect"
		}
		return "shape:" + f
	case "chart":
		if c.Grafico == nil {
			return "chart:"
		}
		return "chart:" + c.Grafico.Tipo
	case "smartart":
		if c.Diagrama == nil {
			return "smart:"
		}
		return "smart:" + c.Diagrama.Disposicion
	}
	texto := strings.Join(strings.Fields(reEtiquetaMorph.ReplaceAllString(c.HTML, " ")), " ")
	return "txt:" + strings.ToLower(texto)
}

// ParMorph es un par [origen, destino] que se interpola.
type ParMorph struct{ Desde, Hasta Caja }

// ParesMorph es `morphPairs`: emparejado voraz uno a uno, primero por id y después por
// clave de contenido; cada caja de origen se usa una sola vez. Devuelve los pares y las
// cajas del destino que entran nuevas.
func ParesMorph(desde, hasta Diapo) (pares []ParMorph, entran []Caja) {
	porID := map[string]int{}
	porClave := map[string][]int{}
	for i, c := range desde.Cajas {
		porID[c.ID] = i
		k := claveMorph(c)
		porClave[k] = append(porClave[k], i)
	}
	usada := make([]bool, len(desde.Cajas))
	for _, t := range hasta.Cajas {
		i, ok := porID[t.ID]
		if ok && usada[i] {
			ok = false
		}
		if !ok {
			for _, j := range porClave[claveMorph(t)] {
				if !usada[j] {
					i, ok = j, true
					break
				}
			}
		}
		if !ok {
			entran = append(entran, t)
			continue
		}
		usada[i] = true
		pares = append(pares, ParMorph{desde.Cajas[i], t})
	}
	return pares, entran
}

// DeltaMorph es `morphDelta`: el desfase de la caja de origen, en puntos porcentuales
// del lienzo, y su escala respecto a la de destino.
func DeltaMorph(p ParMorph) (dx, dy, sx, sy float64) {
	sx, sy = 1, 1
	if p.Hasta.W != 0 {
		sx = p.Desde.W / p.Hasta.W
	}
	if p.Hasta.H != 0 {
		sy = p.Desde.H / p.Hasta.H
	}
	return p.Desde.X - p.Hasta.X, p.Desde.Y - p.Hasta.Y, sx, sy
}

// morfear lanza el morph desde la diapositiva `desde` hasta la de ahora.
func (p *Presentacion) morfear(desde int) {
	if desde < 0 || desde >= len(p.M.Diapos) || desde == p.Diapo {
		return
	}
	pares, entran := ParesMorph(p.M.Diapos[desde], p.M.Diapos[p.Diapo])
	l := p.lienzo
	k := l.escala(l.Size())
	w, h := AnchoEscenario*k/100, p.M.AltoEscenario()*k/100
	for _, par := range pares {
		id := par.Hasta.ID
		dx, dy, sx, sy := DeltaMorph(par)
		a := fyne.NewAnimation(duracionMorph, func(t float32) {
			e := curvaMorph(t)
			if t >= 1 {
				e = 1
			}
			r := 1 - e
			l.Transformar(id, 1, fyne.Delta{DX: float32(dx) * w * r, DY: float32(dy) * h * r},
				1+(float32(sx)-1)*r, 1+(float32(sy)-1)*r, false)
		})
		a.Curve = fyne.AnimationLinear
		p.efectos = append(p.efectos, a)
		a.Start()
	}
	for _, c := range entran {
		id := c.ID
		a := fyne.NewAnimation(DuracionTransicion, func(e float32) { l.Efecto(id, e, fyne.Delta{}, 1) })
		a.Curve = salidaCSS
		p.efectos = append(p.efectos, a)
		a.Start()
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"fmt"
	"math"
	"strings"
)

// Los conectores de una diapositiva: `slides/connector.ts` (el trazado, puro) y
// `ConnectorLayer` (una capa SVG del tamaño del escenario ENCIMA de las cajas). Se
// recalculan al pintar desde la posición actual de las cajas ancladas.

// rectC es `CRect` en px del escenario.
type rectC struct{ x, y, w, h float64 }

func (r rectC) centro() punto { return punto{r.x + r.w/2, r.y + r.h/2} }

// extremoC es `ConnEnd`: anclado a un rectángulo por un lado, o punto libre.
type extremoC struct {
	rect    *rectC
	anclaje string
	punto   punto
}

func (e extremoC) objetivo() punto {
	if e.rect != nil {
		return e.rect.centro()
	}
	return e.punto
}

// remate es `EndCap`: dónde acaba el trazado y hacia dónde apunta.
type remate struct{ x, y, ang float64 }

// rutaC es `ConnectorRoute`, y `puntos` el trazado como polilínea (la curva, en
// tramos) para saber si una pulsación cae encima.
type rutaC struct {
	d             string
	inicio, final remate
	puntos        []punto
}

func angulo(de, a punto) float64 { return math.Atan2(a.y-de.y, a.x-de.x) }

// salidaRayo es `rayExit`: dónde corta el borde el rayo del centro hacia `a`.
func salidaRayo(r rectC, a punto) punto {
	c := r.centro()
	dx, dy := a.x-c.x, a.y-c.y
	if dx == 0 && dy == 0 {
		return punto{r.x + r.w, c.y}
	}
	tx, ty := math.Inf(1), math.Inf(1)
	if dx != 0 {
		tx = r.w / 2 / math.Abs(dx)
	}
	if dy != 0 {
		ty = r.h / 2 / math.Abs(dy)
	}
	t := min(tx, ty)
	return punto{c.x + dx*t, c.y + dy*t}
}

func mitadLado(r rectC, lado string) punto {
	switch lado {
	case "top":
		return punto{r.x + r.w/2, r.y}
	case "bottom":
		return punto{r.x + r.w/2, r.y + r.h}
	case "left":
		return punto{r.x, r.y + r.h/2}
	case "right":
		return punto{r.x + r.w, r.y + r.h/2}
	}
	return r.centro()
}

// puntoAnclaje es `anchorPoint`.
func puntoAnclaje(e extremoC, a punto) punto {
	if e.rect == nil {
		return e.punto
	}
	if e.anclaje == "" || e.anclaje == "auto" {
		return salidaRayo(*e.rect, a)
	}
	return mitadLado(*e.rect, e.anclaje)
}

// dirSalida es `outDir`: perpendicular al borde de la forma, o hacia el otro extremo.
func dirSalida(e extremoC, en, a punto) punto {
	if r := e.rect; r != nil {
		switch {
		case math.Abs(en.x-r.x) < 0.5:
			return punto{-1, 0}
		case math.Abs(en.x-(r.x+r.w)) < 0.5:
			return punto{1, 0}
		case math.Abs(en.y-r.y) < 0.5:
			return punto{0, -1}
		case math.Abs(en.y-(r.y+r.h)) < 0.5:
			return punto{0, 1}
		}
	}
	dx, dy := a.x-en.x, a.y-en.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		l = 1
	}
	return punto{dx / l, dy / l}
}

// rutaConector es `connectorRoute`.
func rutaConector(desde, hasta extremoC, tipo string) rutaC {
	a := puntoAnclaje(desde, hasta.objetivo())
	b := puntoAnclaje(hasta, desde.objetivo())
	switch tipo {
	case "elbow":
		var m1, m2 punto
		if math.Abs(b.x-a.x) >= math.Abs(b.y-a.y) {
			mx := (a.x + b.x) / 2
			m1, m2 = punto{mx, a.y}, punto{mx, b.y}
		} else {
			my := (a.y + b.y) / 2
			m1, m2 = punto{a.x, my}, punto{b.x, my}
		}
		return rutaC{
			d: fmt.Sprintf("M %s %s L %s %s L %s %s L %s %s",
				n2(a.x), n2(a.y), n2(m1.x), n2(m1.y), n2(m2.x), n2(m2.y), n2(b.x), n2(b.y)),
			inicio: remate{a.x, a.y, angulo(m1, a)}, final: remate{b.x, b.y, angulo(m2, b)},
			puntos: []punto{a, m1, m2, b},
		}
	case "curved":
		da, db := dirSalida(desde, a, b), dirSalida(hasta, b, a)
		k := max(24, math.Hypot(b.x-a.x, b.y-a.y)*0.4)
		c1, c2 := punto{a.x + da.x*k, a.y + da.y*k}, punto{b.x + db.x*k, b.y + db.y*k}
		const tramos = 24
		pts := make([]punto, 0, tramos+1)
		for i := range tramos + 1 {
			t := float64(i) / tramos
			u := 1 - t
			pts = append(pts, punto{
				u*u*u*a.x + 3*u*u*t*c1.x + 3*u*t*t*c2.x + t*t*t*b.x,
				u*u*u*a.y + 3*u*u*t*c1.y + 3*u*t*t*c2.y + t*t*t*b.y,
			})
		}
		return rutaC{
			d: fmt.Sprintf("M %s %s C %s %s %s %s %s %s",
				n2(a.x), n2(a.y), n2(c1.x), n2(c1.y), n2(c2.x), n2(c2.y), n2(b.x), n2(b.y)),
			inicio: remate{a.x, a.y, angulo(c1, a)}, final: remate{b.x, b.y, angulo(c2, b)},
			puntos: pts,
		}
	}
	return rutaC{
		d:      fmt.Sprintf("M %s %s L %s %s", n2(a.x), n2(a.y), n2(b.x), n2(b.y)),
		inicio: remate{a.x, a.y, angulo(b, a)}, final: remate{b.x, b.y, angulo(a, b)},
		puntos: []punto{a, b},
	}
}

// distancia es la de `p` a la polilínea, en px del escenario.
func distancia(p punto, pts []punto) float64 {
	mejor := math.Inf(1)
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dx, dy := b.x-a.x, b.y-a.y
		t := 0.0
		if l := dx*dx + dy*dy; l > 0 {
			t = min(max(((p.x-a.x)*dx+(p.y-a.y)*dy)/l, 0), 1)
		}
		mejor = min(mejor, math.Hypot(p.x-(a.x+t*dx), p.y-(a.y+t*dy)))
	}
	return mejor
}

// rutaDe es un conector con su trazado resuelto.
type rutaDe struct {
	c Conector
	r rutaC
}

// rutasConectores resuelve los extremos contra las cajas (en px del escenario) y traza
// cada conector; el que ha perdido su caja no está, como no se pinta.
func rutasConectores(con []Conector, cajas []Caja, sh float64) []rutaDe {
	sw := float64(AnchoEscenario)
	rects := make(map[string]rectC, len(cajas))
	for _, c := range cajas {
		rects[c.ID] = rectC{c.X / 100 * sw, c.Y / 100 * sh, c.W / 100 * sw, c.H / 100 * sh}
	}
	extremo := func(e Extremo) (extremoC, bool) {
		if e.Caja != "" {
			r, ok := rects[e.Caja]
			return extremoC{rect: &r, anclaje: e.Anclaje}, ok
		}
		var x, y float64
		if e.X != nil {
			x = *e.X
		}
		if e.Y != nil {
			y = *e.Y
		}
		return extremoC{punto: punto{x / 100 * sw, y / 100 * sh}}, true
	}
	fuera := make([]rutaDe, 0, len(con))
	for _, c := range con {
		desde, ok1 := extremo(c.Desde)
		hasta, ok2 := extremo(c.Hasta)
		if ok1 && ok2 {
			fuera = append(fuera, rutaDe{c, rutaConector(desde, hasta, c.Trazado)})
		}
	}
	return fuera
}

// holguraConector es la mitad del trazo invisible de 14 que agranda la zona de clic.
const holguraConector = 7

// puntaFlecha es `arrowHead`: las dos barbas en (x, y) apuntando a `ang`.
func puntaFlecha(r remate) string {
	const tam, abre = 9.0, math.Pi / 7
	return fmt.Sprintf("M %s %s L %s %s L %s %s",
		n2(r.x-tam*math.Cos(r.ang-abre)), n2(r.y-tam*math.Sin(r.ang-abre)), n2(r.x), n2(r.y),
		n2(r.x-tam*math.Cos(r.ang+abre)), n2(r.y-tam*math.Sin(r.ang+abre)))
}

// svgConectores es `ConnectorLayer`: la capa del tamaño del escenario con cada conector,
// sus puntas y el trazo en su color o en la tinta de la diapositiva, y bajo el
// `elegido` el halo del acento al 30 % y de 7. Un conector cuya caja ya no está no se
// pinta. nil si no hay ninguno.
func svgConectores(con []Conector, cajas []Caja, sh float64, tinta, elegido, acento string) []byte {
	if len(con) == 0 {
		return nil
	}
	var b strings.Builder
	cabeceraSVG(&b, float64(AnchoEscenario), sh)
	for _, rd := range rutasConectores(con, cajas, sh) {
		c, ruta := rd.c, rd.r
		if c.ID == elegido {
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-opacity="0.3" stroke-width="7"`+
				` stroke-linecap="round" stroke-linejoin="round"/>`, ruta.d, acento)
		}
		trazo := colorSVG(c.Trazo, tinta)
		linea := func(d string) {
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round"`+
				` stroke-linejoin="round"/>`, d, trazo)
		}
		linea(ruta.d)
		if c.PuntaInicio != nil && *c.PuntaInicio {
			linea(puntaFlecha(ruta.inicio))
		}
		if c.PuntaFin != nil && *c.PuntaFin {
			linea(puntaFlecha(ruta.final))
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

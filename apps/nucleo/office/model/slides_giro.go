// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Giro de las cajas de Diapos (`SlideBox.rot`): grados en sentido horario alrededor del
// centro de la caja, con x/y/w/h los de la caja SIN girar. Es el mismo convenio que el
// `rot` de DrawingML (en 60 000-avos de grado), así que en PPTX va y viene tal cual. ODF
// lo dice de otra manera: `draw:transform="rotate(θ) translate(x y)"` gira la forma, puesta
// en el origen, θ radianes en sentido ANTIHORARIO alrededor de su esquina superior
// izquierda y la lleva luego a (x, y); sin svg:x/svg:y. Aquí se traduce entre los dos.

// unidadesGiroDML son las del atributo `rot` de `a:xfrm` por grado (ST_Angle).
const unidadesGiroDML = 60000

// normalizarGiro lleva el giro a [0, 360) con tres decimales; 0 si no hay o no es un
// número. Es `normalizarGiro` de la web (slides/girar.ts): una caja sin girar no lleva el
// campo.
func normalizarGiro(g float64) float64 {
	if math.IsNaN(g) || math.IsInf(g, 0) {
		return 0
	}
	r := math.Round(math.Mod(math.Mod(g, 360)+360, 360)*1000) / 1000
	if r >= 360 {
		return 0
	}
	return r
}

// giroDML es el giro de un atributo `rot` de DrawingML («5400000» = 90°).
func giroDML(rot string) float64 {
	v, err := strconv.ParseInt(strings.TrimSpace(rot), 10, 64)
	if err != nil {
		return 0
	}
	return normalizarGiro(float64(v) / unidadesGiroDML)
}

// atributoGiroDML es el ` rot="…"` del `a:xfrm` de una caja girada; "" sin giro.
func atributoGiroDML(g float64) string {
	g = normalizarGiro(g)
	if g == 0 {
		return ""
	}
	return ` rot="` + strconv.Itoa(int(math.Round(g*unidadesGiroDML))) + `"`
}

// giraHorario gira (dx, dy) g grados en sentido horario con la y hacia abajo (la pantalla).
func giraHorario(dx, dy, g float64) (float64, float64) {
	s, c := math.Sincos(g * math.Pi / 180)
	return dx*c - dy*s, dx*s + dy*c
}

// transformODP es el `draw:transform` de una caja girada en un lienzo de pw × ph cm, o ""
// sin giro. El destino del translate es donde acaba la esquina superior izquierda tras
// girar la caja alrededor de su centro.
func transformODP(box slideBox, pw, ph float64) string {
	g := normalizarGiro(box.Rot)
	if g == 0 {
		return ""
	}
	w, h := box.W/100*pw, box.H/100*ph
	cx, cy := box.X/100*pw+w/2, box.Y/100*ph+h/2
	dx, dy := giraHorario(-w/2, -h/2, g)
	theta := normalizarGiro(360-g) * math.Pi / 180
	return `draw:transform="rotate (` + strconv.FormatFloat(math.Round(theta*1e9)/1e9, 'f', -1, 64) +
		`) translate (` + cmStr(cx+dx) + ` ` + cmStr(cy+dy) + `)"`
}

var reTransformODP = regexp.MustCompile(`(rotate|translate|skewX|skewY|scale|matrix)\s*\(([^)]*)\)`)

// leerTransformODP interpreta un `draw:transform` de LibreOffice (giro y traslado; lo
// demás no se sabe pintar y se ignora) y pone en `b` el giro y la x/y de la caja sin
// girar. `w`/`h` son el ancho y alto de la forma en cm; `pw`/`ph`, los del lienzo. No toca
// `b` si no hay giro ni traslado que leer.
func leerTransformODP(b *slideBox, transform string, w, h, pw, ph float64) {
	if transform == "" || pw <= 0 || ph <= 0 {
		return
	}
	var theta, tx, ty float64
	hay := false
	for _, m := range reTransformODP.FindAllStringSubmatch(transform, -1) {
		args := strings.FieldsFunc(m[2], func(r rune) bool { return r == ' ' || r == ',' || r == '\t' })
		switch m[1] {
		case "rotate":
			if len(args) > 0 {
				theta, hay = atof(args[0]), true
			}
		case "translate":
			if len(args) > 0 {
				tx, hay = parseLenCm(args[0]), true
			}
			if len(args) > 1 {
				ty = parseLenCm(args[1])
			}
		}
	}
	if !hay {
		return
	}
	// θ es antihorario: en el convenio de la caja, g = −θ.
	g := normalizarGiro(-theta * 180 / math.Pi)
	dx, dy := giraHorario(w/2, h/2, g)
	cx, cy := tx+dx, ty+dy
	b.X = roundPct((cx - w/2) / pw * 100)
	b.Y = roundPct((cy - h/2) / ph * 100)
	b.Rot = g
}

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

// Diapositivas ocultas: se ven (atenuadas) en el carril y se editan igual, pero al
// presentar avanzar, retroceder, Inicio/Fin, la «siguiente» del moderador y Cast las
// saltan; solo se llega a una con «número + Intro». Es `hiddenMask`/`nextShown` de
// `slides/anim.ts`, y la tabla que las dos cumplen está en
// `packages/office/test/vectores/diapos-ocultas.json`.

// EstaOculta dice si la diapositiva se salta al presentar.
func (d Diapo) EstaOculta() bool { return d.Oculta != nil && *d.Oculta }

// Saltables es `hiddenMask`: qué diapositivas se saltan al presentar, por índice. Si
// TODAS están ocultas no se salta ninguna: presentar no podría enseñar nada, y quedarse
// en una oculta sin poder moverse sería peor que no hacer caso de la marca.
func Saltables(diapos []Diapo) []bool {
	ocultas := make([]bool, len(diapos))
	todas := len(diapos) > 0
	for i, d := range diapos {
		ocultas[i] = d.EstaOculta()
		todas = todas && ocultas[i]
	}
	if todas {
		return make([]bool, len(diapos))
	}
	return ocultas
}

// SiguienteVisible es `nextShown`: la primera diapositiva que se ENSEÑA desde `desde`
// (sin contarla) en el sentido `dir` (1 o −1); SinDestino si no queda ninguna. `desde`
// puede ser −1 o len(ocultas) para buscar desde un extremo.
func SiguienteVisible(ocultas []bool, desde, dir int) int {
	for i := desde + dir; i >= 0 && i < len(ocultas); i += dir {
		if !ocultas[i] {
			return i
		}
	}
	return SinDestino
}

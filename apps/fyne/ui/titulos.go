// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// En la web: apps/frontend/packages/ui/src/styles.css (una hoja, no una pantalla: sin marcador de paridad).

import "git.sytue.eu/sytue/one/apps/fyne/tema"

// Los títulos de la hoja: `.sytue-h1` a `.sytue-h4` de styles.css. Su cuerpo y su peso
// son tokens de FORMA (`--sytue-title-scale`, `--sytue-title-weight` y
// `--sytue-subtitle-weight`): dentro de la hoja de Marco van un 10 % más grandes y un
// escalón más gruesos; en Carril, como siempre (700/600 y escala 1).
//
// Son para los títulos del CONTENIDO. El de un diálogo no cambia: en la web el diálogo
// se cuelga fuera de la hoja, y sigue en su `sytue-h4` de 600.

// EscalonTitulo es el cuerpo del título de ese nivel (1 a 4), con la escala de la forma
// sobre su `--text-*`: 3xl, 2xl, xl y lg.
func EscalonTitulo(nivel int) tema.Escalon {
	base := [...]tema.Escalon{tema.Texto3XL, tema.Texto2XL, tema.TextoXL, tema.TextoLG}[min(max(nivel, 1), 4)-1]
	escala := tema.Forma("title-scale")
	return tema.Escalon{Tam: base.Tam * escala, Linea: base.Linea * escala}
}

// PesoTitulo es el peso del título de ese nivel: el de título para h1 y h2 y el de
// subtítulo para h3 y h4.
func PesoTitulo(nivel int) Peso {
	if nivel <= 2 {
		return pesoDe(tema.Forma("title-weight"))
	}
	return pesoDe(tema.Forma("subtitle-weight"))
}

// PesoNavActivo es el del ítem activo del menú lateral (`--sytue-nav-active-weight`): en
// Marco no hay barra de «estás aquí» y lo dicen el velo y la negrita; en Carril, el 500
// de siempre.
func PesoNavActivo() Peso { return pesoDe(tema.Forma("nav-active-weight")) }

// pesoDe es la cara embebida para un `font-weight` de la hoja.
//
// ponytail: la web pinta Figtree VARIABLE y un 650 sale exacto; aquí las caras son
// estáticas (400 a 800 de cien en cien) y un peso intermedio baja a la de debajo: el 650
// del ítem activo se ve en 600. Para clavarlo haría falta embeber una instancia a 650.
func pesoDe(n float32) Peso {
	switch {
	case n >= 800:
		return ExtraNegrita
	case n >= 700:
		return Negrita
	case n >= 600:
		return SemiNegrita
	case n >= 500:
		return Medio
	}
	return Normal
}

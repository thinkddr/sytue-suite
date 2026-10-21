// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package nombres limpia el nombre de un fichero que viene de fuera (el adjunto de
// un correo, el de un canal de Rooms) antes de enseñarlo o de guardarlo en disco.
package nombres

import (
	"strings"
	"unicode"
)

// Limpio quita los caracteres de control y los de dirección del texto (U+202E y los
// demás de Bidi_Control). Con ellos, «factura<U+202E>fdp.exe» se veía como «factura-exe.pdf»
// y se guardaba así en Descargas (FYC-04).
func Limpio(nombre string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, nombre)
}

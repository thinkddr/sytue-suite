// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import "embed"

// Noto Color Emoji es la cara CBDT/CBLC (PNG) de la release estable
// v2026-09-24-unicode18_0 de googlefonts/noto-emoji, commit
// e20cbc2bbec1926686be9f9bee7d1d2cfa1fea0e, 2D/fonts/NotoColorEmoji.ttf.
// El pintor de Fyne admite estos bitmaps, pero no los GlyphColor de COLRv1.
//
// Símbolos separados del FS de Figtree: importar fuentes para servir las letras
// del IdP no debe incorporar diez megas de emoji al binario ni a sus rutas HTTP.
//
//go:embed NotoColorEmoji.ttf OFL-NotoColorEmoji.txt
var emoji embed.FS

// Emoji devuelve la fuente de color completa, sin depender de letras del sistema.
func Emoji() []byte { return leerEmoji("NotoColorEmoji.ttf") }

// LicenciaEmoji conserva el SIL OFL 1.1 original junto al recurso distribuido.
func LicenciaEmoji() []byte { return leerEmoji("OFL-NotoColorEmoji.txt") }

func leerEmoji(nombre string) []byte {
	datos, err := emoji.ReadFile(nombre)
	if err != nil {
		panic("fuentes: " + err.Error())
	}
	return datos
}

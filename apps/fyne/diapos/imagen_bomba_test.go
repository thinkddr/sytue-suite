// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// pngGrande es un PNG de verdad de 7000×7000 (49 MP, por encima de MaxPixeles) que
// comprime a casi nada: lo que decodificado ocupa decenas de MiB.
func pngGrande(t *testing.T) string {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 7000, 7000), color.Palette{color.White})
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// TestUnaImagenIncrustadaEnormeNoSeDescodifica (FYE-04): Diapos pasa por la
// barrera de tamaño común antes de reservar los píxeles (Docus, en su paquete).
func TestUnaImagenIncrustadaEnormeNoSeDescodifica(t *testing.T) {
	bomba := pngGrande(t)
	if _, ok := imagenDe(bomba); ok {
		t.Error("Diapos descodificó una imagen de 49 MP")
	}
}

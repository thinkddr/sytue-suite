// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestUnaImagenDelFolioEnormeNoSeDescodifica (FYE-04): una imagen de 49 MP de un
// documento (por encima de MaxPixeles) no reserva sus píxeles al pintarla.
func TestUnaImagenDelFolioEnormeNoSeDescodifica(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 7000, 7000), color.Palette{color.White})
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if imagenDe("data:image/png;base64,"+base64.StdEncoding.EncodeToString(b.Bytes())) != nil {
		t.Fatal("se descodificó una imagen de 49 MP")
	}
}

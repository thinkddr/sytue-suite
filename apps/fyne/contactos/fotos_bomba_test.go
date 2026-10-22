// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestUnaFotoEnormeNoSeDescodifica (FYC-02): la foto de un contacto la pone
// cualquiera de la libreta de la organización; una de 49 MP (por encima del tope
// común) no reserva sus píxeles.
func TestUnaFotoEnormeNoSeDescodifica(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 7000, 7000), color.Palette{color.White})
	var b bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if _, err := FotoDeFichero(b.Bytes()); err == nil {
		t.Error("FotoDeFichero aceptó una imagen de 49 MP")
	}
	if imagenDeURI("data:image/png;base64,"+base64.StdEncoding.EncodeToString(b.Bytes())) != nil {
		t.Error("imagenDeURI descodificó una imagen de 49 MP")
	}
}

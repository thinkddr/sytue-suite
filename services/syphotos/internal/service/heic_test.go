// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/gen2brain/h265/heic"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/services/syphotos/internal/exif"
)

// Un HEIC se mide con su cabecera, como un JPEG, y sus derivadas salen de la misma
// decodificación: la foto del iPhone que llega sin convertir (Windows, Linux) entra en
// la galería en vez de quedarse en Drive.
//
// VISTO EN ROJO (2026-11-25): con `image.DecodeConfig` sin decodificador de HEIC, la
// cabecera da «formato desconocido» y Indexar la rechaza («no es una imagen que se pueda
// medir»).
func TestUnHEICSeIndexaYSeDeriva(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	for y := range 80 {
		for x := range 120 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := heic.Encode(&b, img, heic.EncodeOptions{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	cab := b.Bytes()[:min(b.Len(), exif.BytesDeCabecera)]
	cfg, formato, err := miniatura.Cabecera(cab)
	if err != nil || formato != "heic" || cfg.Width != 120 || cfg.Height != 80 {
		t.Fatalf("cabecera %+v, %q, %v", cfg, formato, err)
	}
	// Sin elemento Exif no hay metadatos, sin error ni pánico.
	if _, err := leerExif(cab, formato); !errors.Is(err, exif.ErrSinExif) {
		t.Errorf("un HEIC sin EXIF: %v", err)
	}
	img2, err := aplanarDe(b.Bytes())
	if err != nil || img2.Bounds().Dx() != 120 {
		t.Fatalf("decodificar para derivar: %v, %v", img2, err)
	}
}

// aplanarDe es la decodificación que usan las derivadas (photorender.Imagen).
func aplanarDe(datos []byte) (image.Image, error) {
	img, err := miniatura.Decodificar(datos)
	if err != nil {
		return nil, err
	}
	return aplanarSobreBlanco(img), nil
}

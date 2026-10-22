// SPDX-License-Identifier: Apache-2.0
// Copyright (c) Sytue. Licensed under Apache-2.0.

package visual

import (
	"image"
	"image/color"
	"testing"
)

func TestUnPixelAlteradoBloqueaLaParidad(t *testing.T) {
	web := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	native := image.NewNRGBA(web.Bounds())
	initial, _, err := Compare(web, native, Tolerance{})
	if err != nil || !initial.Accepted || !initial.Exact {
		t.Fatalf("la captura idéntica debe pasar: %+v, %v", initial, err)
	}
	// Mutar un canal en UN punto invierte el guardián, sin umbrales implícitos.
	native.SetNRGBA(2, 1, color.NRGBA{R: 1})
	r, diff, err := Compare(web, native, Tolerance{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Accepted || r.Exact || r.ChangedPixels != 1 || r.PixelsOverTolerance != 1 || r.MaxChannelDelta != 1 {
		t.Fatalf("una diferencia de un canal debe fallar: %+v", r)
	}
	if r.Bounds == nil || *r.Bounds != (Bounds{MinX: 2, MinY: 1, MaxX: 3, MaxY: 2}) {
		t.Fatalf("posición del cambio incorrecta: %+v", r.Bounds)
	}
	if diff.NRGBAAt(2, 1) != (color.NRGBA{R: 255, B: 255, A: 255}) || diff.NRGBAAt(0, 0).A != 0 {
		t.Fatal("el diff debe señalar el cambio y conservar transparente lo demás")
	}
}

func TestToleranciaExplicitaNoDeclaraIgualdad(t *testing.T) {
	web := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	native := image.NewNRGBA(web.Bounds())
	native.SetNRGBA(0, 0, color.NRGBA{A: 2})
	native.SetNRGBA(1, 0, color.NRGBA{A: 5})
	for _, tc := range []struct {
		name      string
		tolerance Tolerance
		accepted  bool
		over      uint64
	}{
		{"exacta", Tolerance{}, false, 2},
		{"canal", Tolerance{Channel: 2}, false, 1},
		{"un píxel", Tolerance{Channel: 2, Pixels: 1}, true, 1},
		{"todos los canales", Tolerance{Channel: 5}, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _, err := Compare(web, native, tc.tolerance)
			if err != nil || r.Exact || r.Accepted != tc.accepted || r.ChangedPixels != 2 || r.PixelsOverTolerance != tc.over {
				t.Fatalf("resultado incorrecto: %+v, %v", r, err)
			}
		})
	}
}

func TestDimensionesDistintasNoSeNormalizan(t *testing.T) {
	_, _, err := Compare(image.NewNRGBA(image.Rect(0, 0, 2, 2)), image.NewNRGBA(image.Rect(0, 0, 1, 2)), Tolerance{Channel: 255, Pixels: 10})
	if err == nil {
		t.Fatal("ni una tolerancia explícita permite redimensionar la captura")
	}
}

func TestOrigenDeLaImagenYCanalAlfa(t *testing.T) {
	web := image.NewNRGBA(image.Rect(5, 7, 7, 8))
	native := image.NewNRGBA(image.Rect(-2, -3, 0, -2))
	web.SetNRGBA(6, 7, color.NRGBA{A: 255})
	native.SetNRGBA(-1, -3, color.NRGBA{A: 254})
	r, _, err := Compare(web, native, Tolerance{})
	if err != nil || r.Accepted || r.ChangedPixels != 1 || r.MaxChannelDelta != 1 || r.Bounds == nil || r.Bounds.MinX != 1 {
		t.Fatalf("el alfa y el origen forman parte de la comparación: %+v, %v", r, err)
	}
}

func TestCanalesDe16BitsNoSeReducenSilenciosamente(t *testing.T) {
	web := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	native := image.NewNRGBA64(web.Bounds())
	web.SetNRGBA64(0, 0, color.NRGBA64{R: 0x0101, A: 0xffff})
	native.SetNRGBA64(0, 0, color.NRGBA64{R: 0x0102, A: 0xffff})
	// Ambos rojos resultarían 1 al reducirlos a ocho bits: el guardián debe
	// rechazar el formato antes de llegar a una falsa igualdad de píxeles.
	r, diff, err := Compare(web, native, Tolerance{})
	if err == nil || r.Exact || r.Accepted || diff != nil {
		t.Fatalf("no debe certificar una captura reducida: %+v, %v", r, err)
	}
	for _, capture := range []image.Image{
		image.NewRGBA64(web.Bounds()), image.NewGray16(web.Bounds()), image.NewAlpha16(web.Bounds()),
		image.NewUniform(color.NRGBA64{R: 1, A: 0xffff}),
	} {
		if _, _, err := Compare(capture, capture, Tolerance{}); err == nil {
			t.Fatalf("modelo de dieciséis bits aceptado: %T", capture)
		}
	}
}

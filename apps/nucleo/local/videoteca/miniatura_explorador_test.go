// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"runtime"
	"testing"
)

// bgra hace los píxeles de un mapa de bits de GDI.
func bgra(px ...[4]byte) []byte {
	var b []byte
	for _, p := range px {
		b = append(b, p[:]...)
	}
	return b
}

func TestMiniaturaDelExploradorSinAlfaEsOpaca(t *testing.T) {
	// Lo normal en un fotograma: el alfa a cero en todo el mapa.
	img, err := deBGRA(bgra([4]byte{10, 20, 30, 0}, [4]byte{200, 100, 50, 0}), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Pix; !bytes.Equal(got, []byte{30, 20, 10, 255, 50, 100, 200, 255}) {
		t.Fatalf("RGBA: %v", got)
	}
}

func TestMiniaturaDelExploradorConAlfa(t *testing.T) {
	// Premultiplicado (ningún color pasa de su alfa): tal cual.
	img, err := deBGRA(bgra([4]byte{10, 20, 30, 128}, [4]byte{0, 0, 0, 0}), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Pix; !bytes.Equal(got, []byte{30, 20, 10, 128, 0, 0, 0, 0}) {
		t.Fatalf("premultiplicado: %v", got)
	}
	// Sin premultiplicar (un color pasa de su alfa): se premultiplica.
	img, err = deBGRA(bgra([4]byte{255, 0, 200, 128}, [4]byte{1, 2, 3, 255}), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Pix; !bytes.Equal(got, []byte{100, 0, 128, 128, 3, 2, 1, 255}) {
		t.Fatalf("sin premultiplicar: %v", got)
	}
}

func TestMiniaturaDelExploradorTamanoMalo(t *testing.T) {
	if _, err := deBGRA(make([]byte, 7), 2, 1); err == nil {
		t.Fatal("pocos píxeles para el tamaño")
	}
	if _, err := deBGRA(make([]byte, 16), 0, 4); err == nil {
		t.Fatal("sin ancho")
	}
	if _, err := deBGRA(nil, maxLadoExplorador+1, 1); err == nil {
		t.Fatal("demasiado grande")
	}
}

func TestMiniaturaDelExploradorEnPNG(t *testing.T) {
	img, err := deBGRA(bgra([4]byte{1, 2, 3, 0}, [4]byte{4, 5, 6, 0}, [4]byte{7, 8, 9, 0}, [4]byte{10, 11, 12, 0}), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	datos, err := aPNG(img)
	if err != nil {
		t.Fatal(err)
	}
	vuelta, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, a := vuelta.At(1, 1).RGBA(); r>>8 != 12 || g>>8 != 11 || b>>8 != 10 || a>>8 != 255 {
		t.Fatalf("(1,1) = %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestMiniaturaDelExploradorSoloEnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("en Windows la da el Explorador")
	}
	if _, err := miniaturaExplorador(context.Background(), "C:/no/existe.mp4"); !errors.Is(err, ErrSinMiniatura) {
		t.Fatalf("fuera de Windows: %v", err)
	}
}

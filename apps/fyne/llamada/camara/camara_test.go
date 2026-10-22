// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import (
	"image"
	"image/color"
	"os"
	"testing"
	"time"
)

// Las conversiones a I420: un YUYV, un NV12 y un RGBA de colores conocidos salen con su
// luma y su croma en su sitio.
//
// VISTO EN ROJO (2026-09-26, en el frontal): con Cb y Cr cruzados en DeNV12, el azul
// sale rojo.
func TestLasConversionesAI420(t *testing.T) {
	const w, h = 4, 2
	yuyv := make([]byte, w*h*2)
	for i := 0; i < len(yuyv); i += 4 {
		yuyv[i], yuyv[i+1], yuyv[i+2], yuyv[i+3] = 50, 200, 60, 30 // Y0 Cb Y1 Cr
	}
	a := DeYUYV(yuyv, w, h)
	if a.Y[0] != 50 || a.Y[1] != 60 || a.Cb[0] != 200 || a.Cr[0] != 30 {
		t.Errorf("YUYV: Y %v Cb %v Cr %v", a.Y[:2], a.Cb[0], a.Cr[0])
	}
	nv12 := append(make([]byte, w*h), 200, 30, 201, 31, 200, 30, 201, 31)[:w*h+w*h/2]
	for i := range w * h {
		nv12[i] = 90
	}
	b := DeNV12(nv12, w, h)
	if b.Y[0] != 90 || b.Cb[0] != 200 || b.Cr[0] != 30 || b.Cb[1] != 201 {
		t.Errorf("NV12: Cb %v Cr %v", b.Cb, b.Cr)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			rgba.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	c := DeRGBA(rgba)
	if r, _, bl, _ := c.At(0, 0).RGBA(); bl>>8 < 200 || r>>8 > 40 {
		t.Errorf("el azul de la pantalla sale %v", c.At(0, 0))
	}
	i4 := deI420(append(make([]byte, w*h), 1, 2, 3, 4), w, h)
	if i4.Cb[0] != 1 || i4.Cb[1] != 2 || i4.Cr[0] != 3 || i4.Cr[1] != 4 {
		t.Errorf("I420: Cb %v Cr %v", i4.Cb, i4.Cr)
	}
}

// Con SYTUE_CAMARA_REAL=1 abre la cámara de este equipo y espera tres fotogramas. Es la
// prueba en un aparato de verdad; sin la variable se salta.
func TestLaCamaraDeVerdad(t *testing.T) {
	if os.Getenv("SYTUE_CAMARA_REAL") == "" {
		t.Skip("sin SYTUE_CAMARA_REAL")
	}
	t.Logf("cámaras: %+v", Dispositivos())
	f, err := Abrir(PedidoPorOmision)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Cerrar()
	n := 0
	for fin := time.Now().Add(10 * time.Second); n < 3 && time.Now().Before(fin); time.Sleep(30 * time.Millisecond) {
		if img, ok := f.Leer(); ok {
			n++
			t.Logf("fotograma %d: %v", n, img.Rect)
		}
	}
	if n < 3 {
		t.Fatalf("solo %d fotogramas en 10 s", n)
	}
}

// Escalar cabe dentro conservando la proporción y no amplía.
func TestEscalar(t *testing.T) {
	if got := Escalar(I420(1920, 1080), 640, 360).Rect; got.Dx() != 640 || got.Dy() != 360 {
		t.Errorf("1080p a 640×360 da %v", got)
	}
	if got := Escalar(I420(640, 480), 640, 360).Rect; got.Dx() != 480 || got.Dy() != 360 {
		t.Errorf("4:3 a 640×360 da %v", got)
	}
	if got := Escalar(I420(320, 240), 640, 360).Rect; got.Dx() != 320 {
		t.Errorf("amplía: %v", got)
	}
}

// Con SYTUE_PANTALLA_REAL=1 captura la pantalla de este equipo dos veces.
func TestLaPantallaDeVerdad(t *testing.T) {
	if os.Getenv("SYTUE_PANTALLA_REAL") == "" {
		t.Skip("sin SYTUE_PANTALLA_REAL")
	}
	f, err := AbrirPantalla()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Cerrar()
	n := 0
	for i := 0; i < 10 && n < 2; i++ {
		if img, ok := f.Leer(); ok {
			n++
			t.Logf("captura %d: %v", n, img.Rect)
		}
	}
	if n < 2 {
		t.Fatalf("solo %d capturas", n)
	}
}

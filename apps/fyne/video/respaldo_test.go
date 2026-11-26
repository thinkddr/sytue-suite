// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package video

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

// testdata/vp9-perfil1.webm es un VP9 de PERFIL 1 (4:4:4, `gbrp`), que es lo que saca
// libvpx de una fuente RGB y govpx no descodifica (solo el perfil 0). 1 s, 160×120 y
// 10 fps, sin sonido, hecho con
//
//	ffmpeg -f lavfi -i mandelbrot=size=160x120:rate=10 -t 1 -c:v libvpx-vp9 -b:v 100k \
//	  -g 10 vp9-perfil1.webm
const perfil1 = "testdata/vp9-perfil1.webm"

// sistemaFalso es un descodificador del sistema que da fotogramas de un color que no sale
// de ningún WebM de prueba, para saber de dónde viene la imagen.
type sistemaFalso struct{ n int }

var verdeSistema = color.RGBA{G: 200, A: 255}

func (*sistemaFalso) Info() InfoSistema {
	return InfoSistema{Duracion: time.Second, Ancho: 160, Alto: 120, ConVideo: true}
}

func (s *sistemaFalso) Siguiente() (Muestra, error) {
	if s.n >= 10 {
		return Muestra{}, io.EOF
	}
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = verdeSistema.R, verdeSistema.G, verdeSistema.B, verdeSistema.A
	}
	m := Muestra{Tiempo: time.Duration(s.n) * 100 * time.Millisecond, Imagen: img}
	s.n++
	return m, nil
}

func (s *sistemaFalso) Buscar(t time.Duration) (time.Duration, error) {
	s.n = int(t / (100 * time.Millisecond))
	return t, nil
}

func (*sistemaFalso) Close() error { return nil }

// conSistemaFalso registra el sistema falso mientras dura la prueba y apunta qué rutas le
// piden abrir.
func conSistemaFalso(t *testing.T) func() []string {
	t.Helper()
	muSistemas.Lock()
	antes := sistemas
	sistemas = nil
	muSistemas.Unlock()
	t.Cleanup(func() {
		muSistemas.Lock()
		sistemas = antes
		muSistemas.Unlock()
	})
	var mu sync.Mutex
	var abiertas []string
	RegistrarSistema(func(ruta string) (Sistema, error) {
		mu.Lock()
		abiertas = append(abiertas, ruta)
		mu.Unlock()
		return &sistemaFalso{}, nil
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), abiertas...)
	}
}

// esVerdeSistema dice si la imagen es la del sistema falso.
func esVerdeSistema(img image.Image) bool {
	r, g, b, _ := img.At(80, 60).RGBA()
	return r>>8 == 0 && g>>8 == 200 && b>>8 == 0
}

// fotogramaInicial arranca r y espera su primer fotograma (nil si no llega en 5 s).
func fotogramaInicial(t *testing.T, r *Reproductor) image.Image {
	t.Helper()
	llega := make(chan image.Image, 1)
	r.AlFotograma = func(img image.Image, _ time.Duration) {
		select {
		case llega <- img:
		default:
		}
	}
	r.Arrancar()
	t.Cleanup(r.Cerrar)
	select {
	case img := <-llega:
		return img
	case <-time.After(5 * time.Second):
		return nil
	}
}

// Un WebM con un VP9 que Go no descodifica (perfil 1): en Go el reproductor lo lee
// entero sin dar ni un fotograma. Con un descodificador del sistema registrado y el WebM
// en un fichero, va por el sistema, como lo que no es WebM: el visor de Photos y el de
// Cast enseñan el vídeo, y el póster (Fotograma) sale también.
func TestUnVP9QueGoNoSacaVaPorElSistema(t *testing.T) {
	abiertas := conSistemaFalso(t)
	f, err := os.Open(perfil1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	r, err := Nuevo(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := fotogramaInicial(t, r)
	if img == nil || !esVerdeSistema(img) {
		t.Fatalf("el reproductor no enseña lo del sistema: %v", img)
	}
	if w, h := r.Tamano(); w != 160 || h != 120 || r.Duracion() != time.Second {
		t.Errorf("del sistema: %dx%d %v", w, h, r.Duracion())
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	poster, err := Fotograma(f, 0)
	if err != nil || !esVerdeSistema(poster) {
		t.Fatalf("el póster: %v", err)
	}
	if got := abiertas(); len(got) != 2 || got[0] != perfil1 || got[1] != perfil1 {
		t.Errorf("al sistema se le pidió %v", got)
	}
}

// Lo que Go sí descodifica se queda en Go aunque haya sistema (la prueba del primer
// fotograma no lo saca de ahí), y sin fichero con nombre no hay sistema al que pasar: el
// WebM de perfil 1 desde memoria se queda como siempre, sin imagen.
func TestLoQueGoSacaSeQuedaEnGo(t *testing.T) {
	abiertas := conSistemaFalso(t)
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	r, err := Nuevo(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := fotogramaInicial(t, r)
	if img == nil || esVerdeSistema(img) || r.sistema != nil {
		t.Fatalf("un VP9 de perfil 0 no se ve con Go: %v", img)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if poster, err := Fotograma(f, 0); err != nil || esVerdeSistema(poster) {
		t.Fatalf("el póster de un perfil 0: %v", err)
	}

	b, err := os.ReadFile(perfil1)
	if err != nil {
		t.Fatal(err)
	}
	enMemoria, err := Nuevo(bytes.NewReader(b), nil)
	if err != nil {
		t.Fatal(err)
	}
	if enMemoria.sistema != nil {
		t.Fatal("desde memoria no hay fichero que dar al sistema")
	}
	enMemoria.Cerrar()
	if got := abiertas(); len(got) != 0 {
		t.Errorf("se pidió al sistema %v", got)
	}
}

// Sin descodificador del sistema (la edición conectada no registra ninguno) un WebM de
// perfil 1 se abre en Go como siempre, sin probar nada.
func TestSinSistemaNadaCambia(t *testing.T) {
	muSistemas.Lock()
	antes := sistemas
	sistemas = nil
	muSistemas.Unlock()
	t.Cleanup(func() {
		muSistemas.Lock()
		sistemas = antes
		muSistemas.Unlock()
	})
	f, err := os.Open(perfil1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	r, err := Nuevo(f, nil)
	if err != nil || r.sistema != nil || r.vdec == nil {
		t.Fatalf("sin sistema: %v", err)
	}
	r.Cerrar()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := Fotograma(f, 0); err == nil {
		t.Fatal("sin sistema, el póster de un perfil 1 no se puede sacar")
	}
}

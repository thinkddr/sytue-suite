// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !android && !ios && !windows

package sistema

import (
	"errors"
	"image"
	"io"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
)

const corto = "../../../nucleo/local/conversor/testdata/corto.mp4"

func TestLeerDescubrimiento(t *testing.T) {
	salida := `Properties:
  Duration: 0:01:02.500000000
  container #0: video/quicktime
    video #1: video/x-h264
      Width: 640
      Height: 360
      Frame rate: 30000/1001
    audio #2: audio/mpeg
`
	d, err := leerDescubrimiento([]byte(salida))
	if err != nil {
		t.Fatal(err)
	}
	if d.Duracion != 62500*time.Millisecond || d.Ancho != 640 || d.Alto != 360 || !d.Video || !d.Audio || d.Cadencia < 29.9 || d.Cadencia > 30 {
		t.Fatalf("%+v", d)
	}
	if _, err := leerDescubrimiento([]byte("nada")); err == nil {
		t.Fatal("sin pistas no vale")
	}
}

// TestGStreamerDescodificaUnMP4 corre solo donde GStreamer está instalado (con el
// descodificador de H.264 y AAC: gst-libav).
func TestGStreamerDescodificaUnMP4(t *testing.T) {
	p, err := BuscarGStreamer()
	if err != nil {
		t.Skip(err)
	}
	f, err := p.Abrir(corto)
	if err != nil {
		t.Skip("GStreamer no descodifica el MP4 de prueba aquí: ", err)
	}
	defer f.Close()
	in := f.Info()
	if !in.ConVideo || in.Ancho != 320 || in.Duracion <= 0 {
		t.Fatalf("info: %+v", in)
	}
	var cuadros, trozos int
	ultimo := time.Duration(-1)
	for {
		m, err := f.Siguiente()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if m.Tiempo+time.Second < ultimo {
			t.Fatalf("el tiempo va hacia atrás: %v tras %v", m.Tiempo, ultimo)
		}
		ultimo = max(ultimo, m.Tiempo)
		if m.Imagen != nil {
			cuadros++
			if m.Imagen.Bounds().Dx() != 320 {
				t.Fatalf("tamaño %v", m.Imagen.Bounds())
			}
		} else {
			trozos++
		}
	}
	if cuadros == 0 || (in.ConAudio && trozos == 0) {
		t.Fatalf("%d fotogramas y %d trozos de audio", cuadros, trozos)
	}
	// Buscar a la mitad: lo siguiente empieza ahí.
	mitad := in.Duracion / 2
	if _, err := f.Buscar(mitad); err != nil {
		t.Fatal(err)
	}
	m, err := f.Siguiente()
	if err != nil || m.Tiempo < mitad-100*time.Millisecond {
		t.Fatalf("tras buscar: %v %v", m.Tiempo, err)
	}

	// Por el reproductor: Nuevo con un *os.File que no es WebM va por el sistema, y
	// Fotograma saca el póster.
	video.RegistrarSistema(func(ruta string) (video.Sistema, error) { return p.Abrir(ruta) })
	fh, err := os.Open(corto)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	img, err := video.Fotograma(fh, time.Second)
	if err != nil || img.Bounds().Dx() != 320 {
		t.Fatalf("fotograma: %v %v", img, err)
	}
	r, err := video.Nuevo(fh, nil)
	if err != nil {
		t.Fatal(err)
	}
	llego := make(chan struct{}, 1)
	r.AlFotograma = func(image.Image, time.Duration) {
		select {
		case llego <- struct{}{}:
		default:
		}
	}
	r.Arrancar()
	defer r.Cerrar()
	select {
	case <-llego:
	case <-time.After(10 * time.Second):
		t.Fatal("el reproductor no enseñó el primer fotograma")
	}
	if r.Duracion() != in.Duracion {
		t.Fatalf("duración %v", r.Duracion())
	}
}

// El VP9 de perfil 1 que govpx no descodifica (el `mandelbrot` de ffmpeg con libvpx), con
// GStreamer de verdad: el reproductor y el póster lo sacan por el sistema. Solo donde
// GStreamer está instalado con su descodificador de VP9 (gst-plugins-good, libvpx).
func TestGStreamerSacaElVP9QueGoNo(t *testing.T) {
	const perfil1 = "../testdata/vp9-perfil1.webm"
	p, err := BuscarGStreamer()
	if err != nil {
		t.Skip(err)
	}
	if s, err := p.Abrir(perfil1); err != nil {
		t.Skip("GStreamer no descodifica VP9 aquí: ", err)
	} else {
		_ = s.Close()
	}
	video.RegistrarSistema(func(ruta string) (video.Sistema, error) { return p.Abrir(ruta) })
	f, err := os.Open(perfil1)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := video.Fotograma(f, 0)
	if err != nil || img.Bounds().Dx() != 160 || img.Bounds().Dy() != 120 {
		t.Fatalf("póster: %v %v", img, err)
	}
	r, err := video.Nuevo(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	llego := make(chan image.Image, 1)
	r.AlFotograma = func(img image.Image, _ time.Duration) {
		select {
		case llego <- img:
		default:
		}
	}
	r.Arrancar()
	defer r.Cerrar()
	select {
	case img := <-llego:
		if img.Bounds().Dx() != 160 {
			t.Fatalf("fotograma de %v", img.Bounds())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("el reproductor no enseñó el primer fotograma del VP9 de perfil 1")
	}
}

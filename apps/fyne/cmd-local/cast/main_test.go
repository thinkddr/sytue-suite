// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/conversor"
)

func TestSalidaLibreNoPisaNada(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "peli.webm")
	_ = os.WriteFile(orig, []byte("x"), 0o600)
	if s := salidaLibre(orig, ".mp4"); s != filepath.Join(dir, "peli.mp4") {
		t.Errorf("primera: %s", s)
	}
	// La misma extensión que el original: nunca el original.
	if s := salidaLibre(orig, ".webm"); s != filepath.Join(dir, "peli (convertido).webm") {
		t.Errorf("misma extensión: %s", s)
	}
	_ = os.WriteFile(filepath.Join(dir, "peli (convertido).webm"), []byte("x"), 0o600)
	if s := salidaLibre(orig, ".webm"); s != filepath.Join(dir, "peli (convertido 2).webm") {
		t.Errorf("segunda: %s", s)
	}
}

// Las dos tablas de Cast cuadran en los 26 idiomas (C-12): las mismas claves, nada vacío y
// los mismos huecos (%s, %d; y las `{variables}` del catálogo en `propios`).
func TestLasTablasDeCastCuadran(t *testing.T) {
	for _, p := range []local.Propios{tablaTextos, tablaPropios} {
		for _, f := range p.Faltan() {
			t.Error(f)
		}
	}
	llaves := regexp.MustCompile(`\{[a-zA-Z]+\}`)
	for loc, tr := range traduccionesPropias {
		for clave, v := range tr {
			if a, b := llaves.FindAllString(v, -1), llaves.FindAllString(propios[clave][1], -1); !slices.Equal(a, b) {
				t.Errorf("%s: %q lleva %v y el inglés %v", loc, clave, a, b)
			}
		}
	}
}

func TestTextosPorIdioma(t *testing.T) {
	for loc, quiero := range map[i18n.Locale]string{"es": "Convertir a…", "en": "Convert to…", "fr": "Convertir en…", "pt-BR": "Converter para…", "xx": "Convert to…"} {
		if got := textosDe(loc)("convertir"); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
	}
	if s, ok := propiosDe("de")("visibility.unlisted"); !ok || s != "Entwurf" {
		t.Errorf("propio en alemán: %q %v", s, ok)
	}
	if s, ok := propiosDe("de")("watch.views"); !ok || s != "" {
		t.Errorf("las visualizaciones no se quitan: %q %v", s, ok)
	}
	if _, ok := propiosDe("de")("nav.home"); ok {
		t.Error("una clave sin texto propio debe ir al catálogo")
	}
}

// Lo que convierte ffmpeg para verse en la app lo abre de verdad el reproductor propio.
func TestIntegracionElVisionadoLoLeeElReproductor(t *testing.T) {
	h, err := conversor.Buscar("")
	if err != nil {
		t.Skip("ffmpeg no está instalado")
	}
	cods, _ := h.Codificadores(context.Background())
	if !cods["libvpx-vp9"] || !cods["libopus"] {
		t.Skip("este ffmpeg no trae VP9/Opus")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mkv")
	cmd := exec.Command(h.FFmpeg, "-hide_banner", "-nostdin", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "mpeg4", "-c:a", "mp2", "-shortest", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, ok := sondearWebM(clip); ok {
		t.Fatal("un MKV con MPEG-4 no es nativo")
	}
	sal := filepath.Join(dir, "ver.webm")
	if err := h.Convertir(context.Background(), clip, sal, conversor.Visionado, time.Second, nil); err != nil {
		t.Fatal(err)
	}
	info, ok := sondearWebM(sal)
	if !ok || info.Ancho != 320 || info.Alto != 240 || info.Audio != "opus" {
		t.Fatalf("visionado: %+v %v", info, ok)
	}
	f, err := os.Open(sal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := video.Nuevo(f, nil)
	if err != nil {
		t.Fatalf("el reproductor no lo abre: %v", err)
	}
	defer r.Cerrar()
	if ancho, alto := r.Tamano(); ancho != 320 || alto != 240 || r.Duracion() < 900*time.Millisecond {
		t.Errorf("reproductor: %dx%d %v", ancho, alto, r.Duracion())
	}
}

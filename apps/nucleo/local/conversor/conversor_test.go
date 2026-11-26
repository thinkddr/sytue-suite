// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conversor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestArgumentosDeCadaFormato(t *testing.T) {
	for _, d := range append(Destinos(), Destino{Formato: Visionado}) {
		args, err := Argumentos("/en/entrada.mp4", "/sal/salida"+d.Extension, d.Formato)
		if err != nil {
			t.Fatalf("%s: %v", d.Formato, err)
		}
		// Lo común: sin preguntar, sin entrada estándar, con progreso legible.
		for _, x := range []string{"-nostdin", "-y", "-nostats"} {
			if !slices.Contains(args, x) {
				t.Errorf("%s: falta %s en %v", d.Formato, x, args)
			}
		}
		if i := slices.Index(args, "-progress"); i < 0 || args[i+1] != "pipe:1" {
			t.Errorf("%s: el progreso no va por la salida estándar: %v", d.Formato, args)
		}
		if i := slices.Index(args, "-i"); i < 0 || args[i+1] != "/en/entrada.mp4" {
			t.Errorf("%s: entrada mal puesta: %v", d.Formato, args)
		}
		if args[len(args)-1] != "/sal/salida"+d.Extension {
			t.Errorf("%s: la salida no va al final: %v", d.Formato, args)
		}
		if i := slices.Index(args, "-f"); i < 0 {
			t.Errorf("%s: sin -f, ffmpeg adivinaría el formato por la extensión del temporal", d.Formato)
		}
	}
	if _, err := Argumentos("a", "b", "avi"); err == nil {
		t.Error("un formato desconocido tiene que fallar")
	}
}

func TestArgumentosVisionadoEsLoQueLeeElReproductor(t *testing.T) {
	args, _ := Argumentos("in.mov", "out.webm", Visionado)
	orden := strings.Join(args, " ")
	for _, x := range []string{"-c:v libvpx-vp9", "-c:a libopus", "-pix_fmt yuv420p", "-ar 48000", "-f webm", "-cues_to_front 1", "-map 0:v:0?", "-map 0:a:0?"} {
		if !strings.Contains(orden, x) {
			t.Errorf("falta %q en %s", x, orden)
		}
	}
	mp4, _ := Argumentos("in.mov", "out.mp4", MP4)
	if o := strings.Join(mp4, " "); !strings.Contains(o, "-c:v libx264") || !strings.Contains(o, "-c:a aac") || !strings.Contains(o, "+faststart") {
		t.Errorf("MP4 sin H.264/AAC/faststart: %s", o)
	}
	mkv, _ := Argumentos("in.mov", "out.mkv", MKV)
	if o := strings.Join(mkv, " "); !strings.Contains(o, "-c copy") {
		t.Errorf("MKV debería cambiar de contenedor sin recodificar: %s", o)
	}
	mp3, _ := Argumentos("in.mov", "out.mp3", MP3)
	if !slices.Contains(mp3, "-vn") {
		t.Errorf("MP3 es solo audio: %v", mp3)
	}
}

func TestLeerProgreso(t *testing.T) {
	salida := strings.Join([]string{
		"frame=10", "out_time_us=500000", "out_time_ms=500000", "out_time=00:00:00.500000", "speed=2.5x", "progress=continue",
		"frame=20", "out_time=00:00:01.250000", "speed=N/A", "progress=continue",
		"out_time_us=2000000", "progress=end", "",
	}, "\n")
	var vistos []Progreso
	if err := LeerProgreso(strings.NewReader(salida), 2*time.Second, func(p Progreso) { vistos = append(vistos, p) }); err != nil {
		t.Fatal(err)
	}
	if len(vistos) != 3 {
		t.Fatalf("bloques = %d, quiero 3: %+v", len(vistos), vistos)
	}
	if vistos[0].Hecho != 500*time.Millisecond || vistos[0].Velocidad != "2.5x" || vistos[0].Fraccion() != 0.25 {
		t.Errorf("primer bloque %+v (%.3f)", vistos[0], vistos[0].Fraccion())
	}
	if vistos[1].Hecho != 1250*time.Millisecond {
		t.Errorf("sin out_time_us se lee out_time: %+v", vistos[1])
	}
	if !vistos[2].Fin || vistos[2].Fraccion() != 1 {
		t.Errorf("el último es el fin: %+v", vistos[2])
	}
	// Sin total, la fracción no se inventa.
	if (Progreso{Hecho: time.Second}).Fraccion() != 0 {
		t.Error("sin total la fracción es 0")
	}
	// Nunca llega a 1 antes de `progress=end`.
	if f := (Progreso{Hecho: 3 * time.Second, Total: 2 * time.Second}).Fraccion(); f >= 1 {
		t.Errorf("fracción %v antes del fin", f)
	}
}

func TestLeerInstante(t *testing.T) {
	casos := map[string]time.Duration{
		"00:00:00.000000": 0, "01:02:03.500000": time.Hour + 2*time.Minute + 3500*time.Millisecond, "-00:00:00.023000": 0,
	}
	for s, quiero := range casos {
		if d, ok := LeerInstante(s); !ok || d != quiero {
			t.Errorf("%s = %v %v, quiero %v", s, d, ok, quiero)
		}
	}
	if _, ok := LeerInstante("N/A"); ok {
		t.Error("N/A no es un instante")
	}
}

const sondeoMP4 = `{
  "streams": [
    {"index": 0, "codec_name": "h264", "codec_type": "video", "width": 1920, "height": 1080, "pix_fmt": "yuv420p", "duration": "12.500000"},
    {"index": 1, "codec_name": "aac", "codec_type": "audio", "duration": "12.480000"},
    {"index": 2, "codec_name": "mjpeg", "codec_type": "video", "width": 300, "height": 300, "disposition": {"attached_pic": 1}}
  ],
  "format": {"format_name": "mov,mp4,m4a,3gp,3g2,mj2", "duration": "12.500000"}
}`

const sondeoWebM = `{
  "streams": [
    {"codec_name": "vp9", "codec_type": "video", "width": 640, "height": 360, "pix_fmt": "yuv420p"},
    {"codec_name": "opus", "codec_type": "audio"}
  ],
  "format": {"format_name": "matroska,webm", "duration": "3.000000"}
}`

func TestLeerSondeoYNativo(t *testing.T) {
	i, err := LeerSondeo([]byte(sondeoMP4))
	if err != nil {
		t.Fatal(err)
	}
	if i.Video != "h264" || i.Audio != "aac" || i.Ancho != 1920 || i.Alto != 1080 || i.Duracion != 12500*time.Millisecond {
		t.Errorf("mp4: %+v", i)
	}
	if i.Nativo() {
		t.Error("un MP4 H.264 no lo lee el reproductor propio")
	}
	w, err := LeerSondeo([]byte(sondeoWebM))
	if err != nil {
		t.Fatal(err)
	}
	if !w.Nativo() {
		t.Errorf("WebM VP9/Opus es nativo: %+v", w)
	}
	for _, x := range []Info{
		{Contenedor: "matroska,webm", Video: "vp8", Audio: "vorbis"},
		{Contenedor: "matroska,webm", Video: "vp9", PixFmt: "yuv420p10le"},
		{Contenedor: "matroska,webm", Video: "h264", Audio: "opus"},
		{Contenedor: "matroska,webm"},
	} {
		if x.Nativo() {
			t.Errorf("%+v no es nativo", x)
		}
	}
	if (Info{Contenedor: "matroska,webm", Audio: "opus"}).Nativo() != true {
		t.Error("solo Opus en WebM se oye en la app")
	}
	if _, err := LeerSondeo([]byte("no es json")); err == nil {
		t.Error("JSON roto tiene que fallar")
	}
}

func TestLeerCodificadores(t *testing.T) {
	salida := []byte(`Encoders:
 V..... = Video
 A..... = Audio
 ------
 V....D libx264              libx264 H.264
 V....D libvpx-vp9           libvpx VP9
 A....D aac                  AAC
 A....D libopus              libopus Opus
`)
	c := LeerCodificadores(salida)
	for _, x := range []string{"libx264", "libvpx-vp9", "aac", "libopus"} {
		if !c[x] {
			t.Errorf("falta %s: %v", x, c)
		}
	}
	if c["="] || c["Video"] {
		t.Errorf("la leyenda no son codificadores: %v", c)
	}
	mp3, _ := DestinoDe(MP3)
	if mp3.Posible(c) {
		t.Error("sin libmp3lame no se ofrece MP3")
	}
	mp4, _ := DestinoDe(MP4)
	if !mp4.Posible(c) || !mp4.Posible(nil) {
		t.Error("MP4 es posible con libx264 y aac, y sin saber los codificadores")
	}
}

func TestBuscar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permisos de ejecución de Unix")
	}
	dir := t.TempDir()
	ff := filepath.Join(dir, "ffmpeg")
	fp := filepath.Join(dir, "ffprobe")
	for _, f := range []string{ff, fp} {
		if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// La carpeta configurada o la ruta del ejecutable; ffprobe, al lado.
	for _, conf := range []string{dir, ff} {
		h, err := Buscar(conf)
		if err != nil || h.FFmpeg != ff || h.FFprobe != fp {
			t.Errorf("Buscar(%s) = %+v, %v", conf, h, err)
		}
	}
	// Una ruta que no es ejecutable no vale.
	nox := filepath.Join(dir, "otro")
	_ = os.WriteFile(nox, []byte("x"), 0o644)
	if _, err := Buscar(nox); !errors.Is(err, ErrSinFFmpeg) {
		t.Errorf("no ejecutable: %v", err)
	}
	// Sin configurar y sin nada en el PATH.
	antes := buscarEnPath
	defer func() { buscarEnPath = antes }()
	buscarEnPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := Buscar(""); !errors.Is(err, ErrSinFFmpeg) {
		t.Errorf("sin ffmpeg: %v", err)
	}
	buscarEnPath = func(n string) (string, error) {
		if n == "ffmpeg" {
			return ff, nil
		}
		return "", exec.ErrNotFound
	}
	if h, err := Buscar(""); err != nil || h.FFmpeg != ff || h.FFprobe != fp {
		t.Errorf("del PATH: %+v %v", h, err)
	}
}

func TestConvertirNuncaEscribeEncimaDeLaEntrada(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a.webm")
	_ = os.WriteFile(f, []byte("x"), 0o600)
	h := Herramientas{FFmpeg: "/no/existe/ffmpeg"}
	if err := h.Convertir(context.Background(), f, f, WebM, 0, nil); !errors.Is(err, ErrMismoFichero) {
		t.Errorf("misma ruta: %v", err)
	}
	if err := (Herramientas{}).Convertir(context.Background(), f, f+".mp4", MP4, 0, nil); !errors.Is(err, ErrSinFFmpeg) {
		t.Errorf("sin ffmpeg: %v", err)
	}
}

// ---- Con el ffmpeg de verdad (solo si está instalado) ----

func herramientasReales(t *testing.T) Herramientas {
	t.Helper()
	h, err := Buscar("")
	if err != nil {
		t.Skip("ffmpeg no está instalado en este equipo")
	}
	if err := h.LeerVersion(context.Background()); err != nil {
		t.Skipf("ffmpeg no responde: %v", err)
	}
	return h
}

// clipDePrueba genera un vídeo de 1 s con imagen y tono, en MP4 H.264/AAC si este
// ffmpeg sabe, y si no en MKV con lo que haya.
func clipDePrueba(t *testing.T, h Herramientas, dir string) string {
	t.Helper()
	cods, _ := h.Codificadores(context.Background())
	sal, vc, ac := filepath.Join(dir, "clip.mp4"), "libx264", "aac"
	if !cods["libx264"] || !cods["aac"] {
		sal, vc, ac = filepath.Join(dir, "clip.mkv"), "mpeg4", "mp2"
	}
	cmd := exec.Command(h.FFmpeg, "-hide_banner", "-nostdin", "-y", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", vc, "-pix_fmt", "yuv420p", "-c:a", ac, "-shortest", sal)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no se pudo generar el clip: %v\n%s", err, out)
	}
	return sal
}

func TestIntegracionConvertirConFFmpeg(t *testing.T) {
	h := herramientasReales(t)
	dir := t.TempDir()
	clip := clipDePrueba(t, h, dir)
	ctx, fin := context.WithTimeout(context.Background(), 2*time.Minute)
	defer fin()
	cods, err := h.Codificadores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var info Info
	if h.FFprobe != "" {
		if info, err = h.Sondear(ctx, clip); err != nil {
			t.Fatal(err)
		}
		if info.Nativo() || info.Ancho != 320 || info.Alto != 240 || info.Duracion < 900*time.Millisecond {
			t.Errorf("sondeo del clip: %+v", info)
		}
	}
	for _, f := range []Formato{Visionado, MP4, MKV, GIF, Opus} {
		d, _ := DestinoDe(f)
		if !d.Posible(cods) {
			t.Logf("%s: este ffmpeg no trae %v", f, d.Necesita)
			continue
		}
		sal := filepath.Join(dir, "salida-"+string(f)+d.Extension)
		var ultimo Progreso
		avisos := 0
		if err := h.Convertir(ctx, clip, sal, f, info.Duracion, func(p Progreso) { ultimo = p; avisos++ }); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if avisos == 0 || !ultimo.Fin || ultimo.Fraccion() != 1 {
			t.Errorf("%s: progreso %+v en %d avisos", f, ultimo, avisos)
		}
		if fi, err := os.Stat(sal); err != nil || fi.Size() == 0 {
			t.Fatalf("%s: sin salida: %v", f, err)
		}
		if h.FFprobe != "" && f == Visionado {
			v, err := h.Sondear(ctx, sal)
			if err != nil || !v.Nativo() {
				t.Errorf("el WebM de visionado tiene que ser nativo: %+v %v", v, err)
			}
		}
	}
	// No quedan temporales.
	restos, _ := filepath.Glob(filepath.Join(dir, ".*parcial*"))
	if len(restos) > 0 {
		t.Errorf("temporales sin borrar: %v", restos)
	}
	// El póster.
	jpg := filepath.Join(dir, "poster.jpg")
	if err := h.Fotograma(ctx, clip, jpg, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(jpg); len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Error("el póster no es un JPEG")
	}
}

func TestIntegracionCancelarNoDejaNada(t *testing.T) {
	h := herramientasReales(t)
	dir := t.TempDir()
	clip := clipDePrueba(t, h, dir)
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	sal := filepath.Join(dir, "cancelado.mp4")
	if err := h.Convertir(ctx, clip, sal, MP4, 0, nil); err == nil {
		t.Fatal("una conversión cancelada no acaba bien")
	}
	if _, err := os.Stat(sal); !os.IsNotExist(err) {
		t.Error("la cancelada dejó la salida")
	}
	restos, _ := filepath.Glob(filepath.Join(dir, ".*parcial*"))
	if len(restos) > 0 {
		t.Errorf("temporales sin borrar: %v", restos)
	}
}

func TestIntegracionErrorLegible(t *testing.T) {
	h := herramientasReales(t)
	dir := t.TempDir()
	roto := filepath.Join(dir, "roto.mp4")
	_ = os.WriteFile(roto, []byte("esto no es un vídeo"), 0o600)
	err := h.Convertir(context.Background(), roto, filepath.Join(dir, "x.webm"), WebM, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "roto.mp4") {
		t.Errorf("el error tiene que decir qué fichero: %v", err)
	}
}

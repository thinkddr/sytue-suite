// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// valorDe devuelve lo que sigue a la PRIMERA aparición de `flag` en los
// argumentos, o "" si no está.
func valorDe(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func cuenta(args []string, flag string) int {
	n := 0
	for _, a := range args {
		if a == flag {
			n++
		}
	}
	return n
}

// contieneSeguidos dice si `sub` aparece tal cual, en ese orden, en args.
func contieneSeguidos(args []string, sub ...string) bool {
	for i := 0; i+len(sub) <= len(args); i++ {
		if slices.Equal(args[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func TestArgsHLSConAudioYTresPeldanos(t *testing.T) {
	s := Sonda{Ancho: 1920, Alto: 1080, TieneAudio: true, Duracion: 10 * time.Second}
	esc, _ := escaleraPara(s.Ancho, s.Alto, nil)
	salida := "/tmp/x/salida"
	args := argsHLS(entrada{ruta: "/tmp/x/entrada"}, salida, esc, s, 2)

	if got := valorDe(args, "-var_stream_map"); got != "v:0,a:0,name:1080p v:1,a:1,name:720p v:2,a:2,name:360p" {
		t.Errorf("var_stream_map: %q", got)
	}
	if !contieneSeguidos(args, "-b:v:0", "5000k") || !contieneSeguidos(args, "-b:v:1", "2500k") || !contieneSeguidos(args, "-b:v:2", "800k") {
		t.Errorf("tasas por peldaño: %v", args)
	}
	if !contieneSeguidos(args, "-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high") {
		t.Errorf("x264 veryfast high: %v", args)
	}
	if !contieneSeguidos(args, "-c:a", "aac", "-b:a", "128k") {
		t.Errorf("AAC 128k: %v", args)
	}
	if !contieneSeguidos(args, "-f", "hls") || valorDe(args, "-hls_segment_type") != "fmp4" || valorDe(args, "-hls_time") != "4" || valorDe(args, "-hls_playlist_type") != "vod" {
		t.Errorf("HLS fMP4 de 4 s VOD: %v", args)
	}
	if valorDe(args, "-master_pl_name") != "master.m3u8" {
		t.Errorf("master: %v", args)
	}
	// Seis -map: vídeo y audio por peldaño; la entrada una sola vez; los hilos acotados.
	if cuenta(args, "-map") != 6 || cuenta(args, "-i") != 1 || !contieneSeguidos(args, "-threads", "2") {
		t.Errorf("mapas/entrada/hilos: %v", args)
	}
	fc := valorDe(args, "-filter_complex")
	if !strings.Contains(fc, "split=3[v0][v1][v2]") || !strings.Contains(fc, "[v0]scale=-2:1080,format=yuv420p[v0o]") || !strings.Contains(fc, "[v2]scale=-2:360,format=yuv420p[v2o]") {
		t.Errorf("filter_complex: %s", fc)
	}
	// Las salidas cuelgan de hls/%v: el master sube solo a hls/master.m3u8.
	if args[len(args)-1] != filepath.Join(salida, "hls", "%v", "index.m3u8") || valorDe(args, "-hls_segment_filename") != filepath.Join(salida, "hls", "%v", "seg_%05d.m4s") {
		t.Errorf("rutas de salida: %v", args[len(args)-4:])
	}
	if !slices.Contains(args, "-nostdin") || !slices.Contains(args, "-y") {
		t.Errorf("sin consola y sobrescribiendo: %v", args)
	}
	// Una subida solo se abre a sí misma (FILE-12): ni red ni demuxers que abran
	// otras rutas.
	if valorDe(args, "-protocol_whitelist") != "file" || strings.Contains(valorDe(args, "-format_whitelist"), "hls") ||
		strings.Contains(valorDe(args, "-format_whitelist"), "concat") || valorDe(args, "-format_whitelist") == "" {
		t.Errorf("una subida va con protocolo file y sin hls ni concat: %v", args)
	}
}

func TestArgsHLSSinAudioNiMil(t *testing.T) {
	s := Sonda{Ancho: 1280, Alto: 720, TieneAudio: false}
	esc, _ := escaleraPara(s.Ancho, s.Alto, nil)
	args := argsHLS(entrada{ruta: "e"}, "d", esc, s, 2)
	if got := valorDe(args, "-var_stream_map"); got != "v:0,name:720p v:1,name:360p" {
		t.Errorf("sin audio el mapa no lleva a:: %q", got)
	}
	if slices.Contains(args, "-c:a") || slices.Contains(args, "0:a:0") {
		t.Errorf("sin audio no se codifica audio: %v", args)
	}
	if cuenta(args, "-map") != 2 {
		t.Errorf("dos peldaños, dos mapas: %v", args)
	}
}

func TestArgsHLSVerticalEscalaPorElAncho(t *testing.T) {
	s := Sonda{Ancho: 1080, Alto: 1920, TieneAudio: true}
	esc, _ := escaleraPara(s.Ancho, s.Alto, nil)
	fc := valorDe(argsHLS(entrada{ruta: "e"}, "d", esc, s, 2), "-filter_complex")
	if !strings.Contains(fc, "[v0]scale=1080:-2,") || !strings.Contains(fc, "[v1]scale=720:-2,") || !strings.Contains(fc, "[v2]scale=360:-2,") {
		t.Errorf("vertical: %s", fc)
	}
}

func TestArgsHLSDeGrabacion(t *testing.T) {
	s := Sonda{Ancho: 1280, Alto: 720, TieneAudio: true}
	esc, _ := escaleraPara(s.Ancho, s.Alto, nil)
	args := argsHLS(entrada{ruta: "g/index.m3u8", hls: true}, "d", esc, s, 2)
	// La lista de protocolos va ANTES de -i, o no vale para esa entrada.
	i, w := slices.Index(args, "-i"), slices.Index(args, "-protocol_whitelist")
	if w < 0 || w > i || valorDe(args, "-protocol_whitelist") != "file,crypto,data" {
		t.Errorf("protocolos de la grabación: %v", args)
	}
}

func TestArgsWebMUnoPorCalidad(t *testing.T) {
	s := Sonda{Ancho: 1920, Alto: 1080, TieneAudio: true}
	esc, _ := escaleraPara(s.Ancho, s.Alto, nil)
	args := argsWebM(entrada{ruta: "e"}, "d", esc, s, 2)

	// Una decodificación, un split y una salida por calidad, en las MISMAS alturas que el HLS.
	if cuenta(args, "-i") != 1 {
		t.Errorf("una sola entrada: %v", args)
	}
	fc := valorDe(args, "-filter_complex")
	if !strings.Contains(fc, "split=3[w0][w1][w2]") || !strings.Contains(fc, "[w0]scale=-2:1080,format=yuv420p[w0o]") ||
		!strings.Contains(fc, "[w1]scale=-2:720,") || !strings.Contains(fc, "[w2]scale=-2:360,") {
		t.Errorf("filter_complex: %s", fc)
	}
	salidas := []string{}
	for i, a := range args {
		if a == "webm" && i > 0 && args[i-1] == "-f" {
			salidas = append(salidas, args[i+1])
		}
	}
	quiere := []string{filepath.Join("d", "webm", "1080p.webm"), filepath.Join("d", "webm", "720p.webm"), filepath.Join("d", "webm", "360p.webm")}
	if !slices.Equal(salidas, quiere) {
		t.Errorf("salidas %v, se esperaban %v", salidas, quiere)
	}
	// Cada salida con su VP9 a su tasa, Opus, Cues delante e hilos acotados.
	for _, tasa := range []string{"3000k", "1800k", "500k"} {
		if !contieneSeguidos(args, "-c:v", "libvpx-vp9", "-b:v", tasa, "-deadline", "good", "-cpu-used", "4", "-row-mt", "1") {
			t.Errorf("VP9 a %s: %v", tasa, args)
		}
	}
	if cuenta(args, "libopus") != 3 || cuenta(args, "-cues_to_front") != 3 || !contieneSeguidos(args, "-c:a", "libopus", "-b:a", "96k") {
		t.Errorf("Opus y Cues por salida: %v", args)
	}
	// Los hilos: los del decodificador (argsComunes) y uno por codificador.
	if cuenta(args, "-threads") != 4 {
		t.Errorf("-threads acotado en cada codificador: %v", args)
	}

	// Sin audio: -an y nada de Opus. Un 720p no sube a 1080.
	s = Sonda{Ancho: 1280, Alto: 720}
	esc, _ = escaleraPara(s.Ancho, s.Alto, nil)
	args = argsWebM(entrada{ruta: "e"}, "d", esc, s, 2)
	if cuenta(args, "-an") != 2 || slices.Contains(args, "libopus") {
		t.Errorf("sin audio: %v", args)
	}
	if strings.Contains(strings.Join(args, " "), "1080") {
		t.Errorf("un 720p no da 1080: %v", args)
	}
	// Vertical: por el ancho.
	s = Sonda{Ancho: 1080, Alto: 1920}
	esc, _ = escaleraPara(s.Ancho, s.Alto, []int{720})
	if fc := valorDe(argsWebM(entrada{ruta: "e"}, "d", esc, s, 2), "-filter_complex"); !strings.Contains(fc, "[w0]scale=720:-2,") {
		t.Errorf("vertical: %q", fc)
	}
}

func TestArgsPoster(t *testing.T) {
	args := argsPoster(entrada{ruta: "e"}, 1500*time.Millisecond, "d", 2)
	// -ss antes de -i (búsqueda en el contenedor), y las dos salidas.
	ss, i := slices.Index(args, "-ss"), slices.Index(args, "-i")
	if ss < 0 || ss > i || valorDe(args, "-ss") != "1.500" {
		t.Errorf("-ss: %v", args)
	}
	if !slices.Contains(args, filepath.Join("d", "poster.jpg")) || !slices.Contains(args, filepath.Join("d", "poster.webp")) {
		t.Errorf("dos pósteres: %v", args)
	}
	if !contieneSeguidos(args, "-c:v", "libwebp", "-quality", "85") || !contieneSeguidos(args, "-q:v", "4") {
		t.Errorf("calidades: %v", args)
	}
	if cuenta(args, "-frames:v") != 2 || cuenta(args, "-vf") != 2 {
		t.Errorf("un fotograma y el escalado por salida: %v", args)
	}
	if !strings.Contains(filtroPoster, "1280") || !strings.Contains(filtroPoster, "force_original_aspect_ratio=decrease") {
		t.Errorf("el póster cabe en 1280 sin agrandar: %s", filtroPoster)
	}
}

func TestInstantePoster(t *testing.T) {
	d := 20 * time.Second
	if got := instantePoster(d, nil); got != 2*time.Second {
		t.Errorf("sin petición, el 10 %%: %v", got)
	}
	if got := instantePoster(d, ptr(7*time.Second)); got != 7*time.Second {
		t.Errorf("lo pedido: %v", got)
	}
	if got := instantePoster(d, ptr(time.Hour)); got != d-100*time.Millisecond {
		t.Errorf("acotado dentro del vídeo: %v", got)
	}
	if got := instantePoster(0, nil); got != 0 {
		t.Errorf("sin duración conocida, el principio: %v", got)
	}
	if got := instantePoster(0, ptr(3*time.Second)); got != 3*time.Second {
		t.Errorf("sin duración pero con petición, la petición: %v", got)
	}
	if got := instantePoster(50*time.Millisecond, nil); got != 0 {
		t.Errorf("un vídeo de 50 ms no tiene décima antes del final: %v", got)
	}
}

func TestUltimaLineaYAcotar(t *testing.T) {
	salida := "aviso cualquiera\n[libx264 @ 0x55] Error: algo\n\n  Conversion failed!  \n\n"
	if got := ultimaLinea(salida, "exit 1"); got != "Conversion failed!" {
		t.Errorf("última línea: %q", got)
	}
	if got := ultimaLinea("   \n", "exit status 1"); got != "exit status 1" {
		t.Errorf("sin salida, el error del proceso: %q", got)
	}
	largo := strings.Repeat("á", 400)
	if got := acotar(largo); len([]rune(got)) != 300 || !strings.HasSuffix(got, "…") {
		t.Errorf("acotar: %d runas", len([]rune(got)))
	}
	if e := (&errorFfmpeg{paso: "hls", motivo: "x"}).Error(); e != "ffmpeg (hls): x" {
		t.Errorf("mensaje: %q", e)
	}
}

func TestMotivoDe(t *testing.T) {
	if m := motivoDe(nil, time.Hour); m != "" {
		t.Errorf("sin error no hay motivo: %q", m)
	}
	if m := motivoDe(context.DeadlineExceeded, 2*time.Hour); m != "tiempo agotado: el trabajo pasó de 2h0m0s" {
		t.Errorf("plazo: %q", m)
	}
	if m := motivoDe(context.Canceled, time.Hour); !strings.HasPrefix(m, "interrumpido") {
		t.Errorf("apagado: %q", m)
	}
	err := errors.Join(errors.New("ffmpeg (webm): línea uno"), errors.New("línea dos"))
	if m := motivoDe(err, time.Hour); strings.Contains(m, "\n") {
		t.Errorf("una sola línea: %q", m)
	}
	if m := motivoDe(errors.New(strings.Repeat("x", 500)), time.Hour); len([]rune(m)) != 300 {
		t.Errorf("acotado: %d", len([]rune(m)))
	}
}

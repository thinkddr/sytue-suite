// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T, nombre string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "ffprobe", nombre))
	if err != nil {
		t.Fatalf("fixture %s: %v", nombre, err)
	}
	return b
}

// Las fixtures son salidas de `ffprobe -print_format json -show_format
// -show_streams` con la forma de ffprobe 5.1 (la de debian:bookworm): un MP4 de
// cámara, un MOV de iPhone en vertical (rotación en side_data y en tags), un WebM
// 4K sin audio, la grabación HLS de un directo y un MP3 sin vídeo.
func TestParsearSonda(t *testing.T) {
	casos := []struct {
		fichero string
		quiere  Sonda
	}{
		{"mp4_h264_1080p.json", Sonda{Duracion: 12*time.Second + 512500*time.Microsecond, Ancho: 1920, Alto: 1080, CodecVideo: "h264", CodecAudio: "aac", TieneAudio: true, FPS: 30000.0 / 1001, Formato: "mov,mp4,m4a,3gp,3g2,mj2", Bytes: 12908773}},
		// El iPhone guarda 1920×1080 y dice «gira 90°»: se presenta 1080×1920.
		{"mov_iphone_vertical.json", Sonda{Duracion: 8900 * time.Millisecond, Ancho: 1080, Alto: 1920, CodecVideo: "hevc", CodecAudio: "aac", TieneAudio: true, FPS: 30, Formato: "mov,mp4,m4a,3gp,3g2,mj2", Bytes: 17812345}},
		{"webm_vp9_4k_sin_audio.json", Sonda{Duracion: 30 * time.Second, Ancho: 3840, Alto: 2160, CodecVideo: "vp9", TieneAudio: false, FPS: 60, Formato: "matroska,webm", Bytes: 56123456}},
		{"hls_grabacion_ts.json", Sonda{Duracion: 612*time.Second + 33333*time.Microsecond, Ancho: 1280, Alto: 720, CodecVideo: "h264", CodecAudio: "aac", TieneAudio: true, FPS: 30, Formato: "hls", Bytes: 1734}},
	}
	for _, c := range casos {
		t.Run(c.fichero, func(t *testing.T) {
			s, err := parsearSonda(fixture(t, c.fichero))
			if err != nil {
				t.Fatalf("parsearSonda: %v", err)
			}
			if d := (s.Duracion - c.quiere.Duracion).Abs(); d > time.Millisecond {
				t.Errorf("duración %v, se esperaba %v", s.Duracion, c.quiere.Duracion)
			}
			if s.Ancho != c.quiere.Ancho || s.Alto != c.quiere.Alto {
				t.Errorf("dimensiones %d×%d, se esperaba %d×%d", s.Ancho, s.Alto, c.quiere.Ancho, c.quiere.Alto)
			}
			if s.CodecVideo != c.quiere.CodecVideo || s.CodecAudio != c.quiere.CodecAudio || s.TieneAudio != c.quiere.TieneAudio {
				t.Errorf("códecs %q/%q (audio %v), se esperaba %q/%q (%v)", s.CodecVideo, s.CodecAudio, s.TieneAudio, c.quiere.CodecVideo, c.quiere.CodecAudio, c.quiere.TieneAudio)
			}
			if math.Abs(s.FPS-c.quiere.FPS) > 0.01 {
				t.Errorf("fps %.3f, se esperaba %.3f", s.FPS, c.quiere.FPS)
			}
			if s.Formato != c.quiere.Formato || s.Bytes != c.quiere.Bytes {
				t.Errorf("formato %q %d bytes, se esperaba %q %d", s.Formato, s.Bytes, c.quiere.Formato, c.quiere.Bytes)
			}
		})
	}
}

func TestParsearSondaVerticalYLadoCorto(t *testing.T) {
	s, err := parsearSonda(fixture(t, "mov_iphone_vertical.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Vertical() || s.LadoCorto() != 1080 {
		t.Fatalf("un vídeo de móvil girado es vertical con lado corto 1080: %+v", s)
	}
	s, _ = parsearSonda(fixture(t, "webm_vp9_4k_sin_audio.json"))
	if s.Vertical() || s.LadoCorto() != 2160 {
		t.Fatalf("4K horizontal: %+v", s)
	}
}

func TestParsearSondaSinVideo(t *testing.T) {
	if _, err := parsearSonda(fixture(t, "mp3_sin_video.json")); !errors.Is(err, ErrSinVideo) {
		t.Fatalf("un MP3 no es un vídeo: %v", err)
	}
}

func TestParsearSondaIlegible(t *testing.T) {
	if _, err := parsearSonda([]byte("esto no es json")); !errors.Is(err, ErrSondaIlegible) {
		t.Fatalf("basura: %v", err)
	}
	// JSON válido pero sin pistas: tampoco es un vídeo.
	if _, err := parsearSonda([]byte(`{"streams":[],"format":{"duration":"3"}}`)); !errors.Is(err, ErrSinVideo) {
		t.Fatalf("sin pistas: %v", err)
	}
}

func TestParsearSondaDuracionDeLaPista(t *testing.T) {
	// Un contenedor sin duración (N/A) la toma de la pista de vídeo.
	js := []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":640,"height":360,"duration":"4.5","r_frame_rate":"25/1","avg_frame_rate":"0/0"}],"format":{"format_name":"mpegts","duration":"N/A"}}`)
	s, err := parsearSonda(js)
	if err != nil {
		t.Fatal(err)
	}
	if s.Duracion != 4500*time.Millisecond || s.FPS != 25 {
		t.Fatalf("duración %v fps %v", s.Duracion, s.FPS)
	}
}

func TestGiraNoventa(t *testing.T) {
	for _, g := range []float64{90, -90, 270, -270, 450} {
		if !giraNoventa(g) {
			t.Errorf("%v° gira", g)
		}
	}
	for _, g := range []float64{0, 180, -180, 360} {
		if giraNoventa(g) {
			t.Errorf("%v° no gira", g)
		}
	}
}

func TestTasaDeYSegundos(t *testing.T) {
	if tasaDe("0/0") != 0 || tasaDe("") != 0 || tasaDe("x/2") != 0 {
		t.Fatal("una tasa desconocida es 0")
	}
	if math.Abs(tasaDe("30000/1001")-29.97) > 0.01 || tasaDe("25") != 25 {
		t.Fatal("tasas normales")
	}
	if segundosADuracion("N/A") != 0 || segundosADuracion("-1") != 0 || segundosADuracion("1.5") != 1500*time.Millisecond {
		t.Fatal("segundos")
	}
}

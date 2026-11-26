// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conversor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Los ficheros de testdata salen de ffmpeg:
//
//	ffmpeg -f lavfi -i testsrc=duration=2:size=320x240:rate=10 -f lavfi -i sine=duration=2 \
//	  -c:v libx264 -pix_fmt yuv420p -c:a aac -shortest corto.mp4
//	ffmpeg -f lavfi -i testsrc=duration=1:size=176x144:rate=10 -c:v mpeg4 mudo.mov
func TestSondearMP4SinFFprobe(t *testing.T) {
	i, err := SondearMP4("testdata/corto.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if i.Video != "h264" || i.Audio != "aac" || i.Ancho != 320 || i.Alto != 240 {
		t.Fatalf("%+v", i)
	}
	if i.Duracion < 1900*time.Millisecond || i.Duracion > 2100*time.Millisecond {
		t.Fatalf("duración %v", i.Duracion)
	}
	if i.Nativo() {
		t.Fatal("un MP4 no es nativo")
	}
	m, err := SondearMP4("testdata/mudo.mov")
	if err != nil || m.Video != "mpeg4" || m.Audio != "" || m.Ancho != 176 || m.Alto != 144 {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestSondearMP4RechazaLoQueNoLoEs(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "x.mp4")
	for _, contenido := range [][]byte{nil, []byte("hola, esto no es un vídeo"), {0, 0, 0, 200, 'm', 'o', 'o', 'v'}} {
		if err := os.WriteFile(ruta, contenido, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := SondearMP4(ruta); err == nil {
			t.Fatalf("aceptó %q", contenido)
		}
	}
}

// Contrastado con ffprobe cuando lo hay: los dos tienen que decir lo mismo.
func TestSondearMP4DiceLoMismoQueFFprobe(t *testing.T) {
	h, err := Buscar("")
	if err != nil || h.FFprobe == "" {
		t.Skip("sin ffprobe")
	}
	a, err := h.Sondear(t.Context(), "testdata/corto.mp4")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := SondearMP4("testdata/corto.mp4")
	if a.Video != b.Video || a.Audio != b.Audio || a.Ancho != b.Ancho || a.Alto != b.Alto {
		t.Fatalf("ffprobe %+v, Go %+v", a, b)
	}
	if d := a.Duracion - b.Duracion; d > 100*time.Millisecond || d < -100*time.Millisecond {
		t.Fatalf("duración ffprobe %v, Go %v", a.Duracion, b.Duracion)
	}
}

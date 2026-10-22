// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

const playlistEgress = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:EVENT
#EXTINF:4.000,
cast-acme-1234_00000.ts
#EXTINF:4.000,
cast-acme-1234_00001.ts
#EXTINF:2.500,
cast-acme-1234_00002.ts
#EXT-X-ENDLIST
`

func TestParsearPlaylist(t *testing.T) {
	segs, cerrado, err := parsearPlaylist(playlistEgress)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 3 || !cerrado {
		t.Fatalf("segmentos %d cerrado %v", len(segs), cerrado)
	}
	if segs[2].URI != "cast-acme-1234_00002.ts" || segs[2].Duracion != 2.5 || segs[0].Duracion != 4 {
		t.Fatalf("segmentos: %+v", segs)
	}
	// Sin ENDLIST (el egress aún emitía cuando se leyó) se sigue igual, pero se sabe.
	_, cerrado, err = parsearPlaylist(strings.Replace(playlistEgress, "#EXT-X-ENDLIST\n", "", 1))
	if err != nil || cerrado {
		t.Fatalf("abierto: %v %v", cerrado, err)
	}
	if n := objetivo(segs); n != 4 {
		t.Fatalf("TARGETDURATION %d", n)
	}
	if n := objetivo([]segmento{{Duracion: 4.001}}); n != 5 {
		t.Fatalf("TARGETDURATION redondea hacia arriba: %d", n)
	}
}

func TestParsearPlaylistRechaza(t *testing.T) {
	casos := map[string]string{
		"sin cabecera":        "#EXTINF:4,\nx.ts\n",
		"maestro":             "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n720p/index.m3u8\n",
		"sube de directorio":  "#EXTM3U\n#EXTINF:4,\n../otro/x.ts\n",
		"absoluta":            "#EXTM3U\n#EXTINF:4,\n/etc/passwd\n",
		"url":                 "#EXTM3U\n#EXTINF:4,\nhttps://fuera.example/x.ts\n",
		"segmento sin EXTINF": "#EXTM3U\nx.ts\n",
		"duración ilegible":   "#EXTM3U\n#EXTINF:abc,\nx.ts\n",
		"vacío":               "#EXTM3U\n#EXT-X-ENDLIST\n",
	}
	for nombre, texto := range casos {
		if _, _, err := parsearPlaylist(texto); !errors.Is(err, ErrPlaylistInvalido) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	// Un subdirectorio relativo sí vale (el egress puede prefijar con carpeta).
	if _, _, err := parsearPlaylist("#EXTM3U\n#EXTINF:4,\nseg/x.ts\n"); err != nil {
		t.Errorf("subdirectorio relativo: %v", err)
	}
}

func TestDescargarGrabacion(t *testing.T) {
	ctx := context.Background()
	alm := nuevoAlmacenFalso()
	acme := tenancy.Tenant{Slug: "acme"}
	clave := "vid-1/live/index.m3u8"
	alm.poner(acme, clave, "application/vnd.apple.mpegurl", []byte(playlistEgress))
	tam := []int{1000, 2000, 500}
	for i, n := range tam {
		alm.poner(acme, "vid-1/live/cast-acme-1234_0000"+string(rune('0'+i))+".ts", "video/mp2t", make([]byte, n))
	}
	w := trabajadorDePrueba(nuevaColaFalsa(), alm, t.TempDir())

	t.Run("entera", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "grabacion")
		g, err := w.descargarGrabacion(ctx, acme, clave, dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		if g.bytes != int64(len(playlistEgress)+3500) || g.duracion != 10.5 {
			t.Fatalf("bytes %d duración %v", g.bytes, g.duracion)
		}
		for i := range tam {
			if _, err := os.Stat(filepath.Join(dir, "cast-acme-1234_0000"+string(rune('0'+i))+".ts")); err != nil {
				t.Errorf("segmento %d: %v", i, err)
			}
		}
		local, _ := os.ReadFile(g.playlist)
		if !strings.HasSuffix(strings.TrimSpace(string(local)), "#EXT-X-ENDLIST") || strings.Count(string(local), "#EXTINF") != 3 || !strings.Contains(string(local), "#EXT-X-PLAYLIST-TYPE:VOD") {
			t.Errorf("playlist local:\n%s", local)
		}
	})

	t.Run("hasta un instante", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "grabacion")
		// A los 5 s hacen falta los dos primeros segmentos (0-4 y 4-8), no el tercero.
		g, err := w.descargarGrabacion(ctx, acme, clave, dir, 5)
		if err != nil {
			t.Fatal(err)
		}
		if g.duracion != 8 || g.bytes != int64(len(playlistEgress)+3000) {
			t.Fatalf("recortado: duración %v bytes %d", g.duracion, g.bytes)
		}
		if _, err := os.Stat(filepath.Join(dir, "cast-acme-1234_00002.ts")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("el tercer segmento no hacía falta: %v", err)
		}
		local, _ := os.ReadFile(g.playlist)
		if strings.Count(string(local), "#EXTINF") != 2 {
			t.Errorf("playlist recortado:\n%s", local)
		}
	})

	t.Run("segmento que falta", func(t *testing.T) {
		alm2 := nuevoAlmacenFalso()
		alm2.poner(acme, clave, "", []byte(playlistEgress))
		w2 := trabajadorDePrueba(nuevaColaFalsa(), alm2, t.TempDir())
		if _, err := w2.descargarGrabacion(ctx, acme, clave, filepath.Join(t.TempDir(), "g"), 0); err == nil || !strings.Contains(err.Error(), "bajar") {
			t.Fatalf("sin segmentos tiene que fallar diciéndolo: %v", err)
		}
	})

	t.Run("sin playlist", func(t *testing.T) {
		if _, err := w.descargarGrabacion(ctx, acme, "no/existe.m3u8", filepath.Join(t.TempDir(), "g"), 0); err == nil {
			t.Fatal("un playlist que no está es un error")
		}
	})
}

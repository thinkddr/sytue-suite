// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"context"
	"crypto/md5" //nolint:gosec // G501: el nombre que fija freedesktop
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMiniaturaDeLaCacheFreedesktop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("caché de freedesktop")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("XDG_DATA_DIRS", t.TempDir()) // sin miniaturizadores
	t.Setenv("HOME", t.TempDir())
	vid := filepath.Join(t.TempDir(), "mi viaje #1.mp4")
	escribir(t, vid, "mp4")
	if !strings.Contains(uriFichero(vid), "mi%20viaje%20%231.mp4") {
		t.Fatalf("uri: %s", uriFichero(vid))
	}
	if _, _, err := MiniaturaDelSistema(context.Background(), vid); err == nil {
		t.Fatal("sin caché ni miniaturizador no hay miniatura")
	}
	suma := md5.Sum([]byte(uriFichero(vid))) //nolint:gosec // G401: ver el import
	r := filepath.Join(cache, "thumbnails", "large", hex.EncodeToString(suma[:])+".png")
	escribir(t, r, "PNG de la caché")
	datos, ext, err := MiniaturaDelSistema(context.Background(), vid)
	if err != nil || ext != "png" || string(datos) != "PNG de la caché" {
		t.Fatalf("%q %q %v", datos, ext, err)
	}
	// Más vieja que el vídeo: no vale.
	viejo := time.Now().Add(-time.Hour)
	_ = os.Chtimes(r, viejo, viejo)
	if _, _, err := MiniaturaDelSistema(context.Background(), vid); err == nil {
		t.Fatal("una miniatura más vieja que el vídeo no vale")
	}
}

func TestMiniaturizadorInstalado(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin sh")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "falso-thumbnailer")
	escribir(t, script, "#!/bin/sh\n# $1=-s $2=tam $3=entrada $4=salida\nprintf 'miniatura de %s a %s' \"$3\" \"$2\" > \"$4\"\n")
	if err := os.Chmod(script, 0o700); err != nil {
		t.Fatal(err)
	}
	escribir(t, filepath.Join(dir, "thumbnailers", "falso.thumbnailer"), "[Thumbnailer Entry]\nTryExec="+script+"\nExec="+script+" -s %s \"%i\" %o\nMimeType=video/mp4;video/quicktime;\n")
	escribir(t, filepath.Join(dir, "thumbnailers", "otro.thumbnailer"), "[Thumbnailer Entry]\nExec=/no/existe %i %o\nMimeType=image/svg+xml;\n")
	vid := filepath.Join(dir, "con espacios.mp4")
	escribir(t, vid, "mp4")
	datos, err := conMiniaturizador(context.Background(), vid, []string{filepath.Join(dir, "thumbnailers")})
	if err != nil || string(datos) != "miniatura de "+vid+" a 512" {
		t.Fatalf("%q %v", datos, err)
	}
	if _, err := conMiniaturizador(context.Background(), filepath.Join(dir, "x.avi"), []string{filepath.Join(dir, "thumbnailers")}); err == nil {
		t.Fatal("un tipo sin miniaturizador no tiene miniatura")
	}
	if got := partirExec(`a "b c" d\ e`); len(got) != 4 || got[1] != "b c" {
		t.Fatalf("%q", got)
	}
}

func TestPosterSinFFmpegYColecciones(t *testing.T) {
	o := opciones(t)
	o.Fotograma = func(_ context.Context, ruta string, en time.Duration) ([]byte, error) {
		return []byte("jpeg de " + filepath.Base(ruta)), nil
	}
	o.LeeElSistema = func(ruta string) bool { return strings.HasSuffix(ruta, ".mp4") }
	escribir(t, filepath.Join(o.Carpeta, "uno.webm"), "webm")
	escribir(t, filepath.Join(o.Carpeta, "dos.mp4"), "mp4")
	v, err := Abrir(o)
	if err != nil {
		t.Fatal(err)
	}
	var uno, dos Entrada
	for _, e := range v.Lista() {
		if strings.HasSuffix(e.Ruta, "uno.webm") {
			uno = e
		} else {
			dos = e
		}
	}
	// Con descodificador del sistema el MP4 se ve tal cual, sin convertir.
	if est := v.Visionado(dos.ID); !est.Listo || est.Ruta != dos.Ruta {
		t.Fatalf("visionado por el sistema: %+v", est)
	}
	e, err := v.PosterDeFotograma(context.Background(), uno.ID, time.Second)
	if err != nil || e.Poster == "" {
		t.Fatalf("póster sin ffmpeg: %+v %v", e, err)
	}
	m, err := v.OpenMedia(context.Background(), "local:"+uno.ID+"/poster")
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	n := e.PosterN
	if e, _ = v.PonerFotograma(uno.ID, []byte("otro jpeg")); e.PosterN <= n {
		t.Fatal("el póster nuevo cambia su número (y su dirección)")
	}

	viajes := "Viajes"
	if _, err := v.Actualizar(uno.ID, Cambios{Coleccion: &viajes}); err != nil {
		t.Fatal(err)
	}
	minus := "viajes"
	if _, err := v.Actualizar(dos.ID, Cambios{Coleccion: &minus}); err != nil {
		t.Fatal(err)
	}
	cs := v.Colecciones()
	if len(cs) != 1 || cs[0].Videos != 2 || cs[0].Publicados != 0 {
		t.Fatalf("colecciones: %+v", cs)
	}
	ch, err := v.Channel(context.Background(), PrefijoColeccion+"Viajes", "")
	if err != nil || ch.Name != "Viajes" || len(ch.Videos) != 2 {
		t.Fatalf("canal de la colección: %+v %v", ch, err)
	}
	if ch, _ := v.Channel(context.Background(), "local", ""); len(ch.Videos) != 0 {
		t.Fatal("el canal de siempre es lo publicado")
	}
}

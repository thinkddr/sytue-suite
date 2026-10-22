// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package transcoder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestSubirDirectorioYTamano(t *testing.T) {
	ctx := context.Background()
	alm := nuevoAlmacenFalso()
	acme := tenancy.Tenant{Slug: "acme"}
	w := trabajadorDePrueba(nuevaColaFalsa(), alm, t.TempDir())

	dir := t.TempDir()
	escribir := func(rel string, n int) {
		ruta := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ruta, make([]byte, n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	escribir("hls/master.m3u8", 120)
	escribir("hls/720p/index.m3u8", 300)
	escribir("hls/720p/seg_00000.m4s", 4096)
	escribir("webm/720p.webm", 8192)
	escribir("poster.jpg", 2048)
	escribir("vacio.m3u8", 0)

	total, err := w.subirDirectorio(ctx, acme, dir, "vid-1/")
	if err != nil {
		t.Fatal(err)
	}
	if total != 120+300+4096+8192+2048 {
		t.Fatalf("total %d", total)
	}
	datos, tipo, ok := alm.leer(acme, "vid-1/hls/720p/seg_00000.m4s")
	if !ok || len(datos) != 4096 || tipo != "video/iso.segment" {
		t.Fatalf("segmento: %v %d %q", ok, len(datos), tipo)
	}
	if _, tipo, _ := alm.leer(acme, "vid-1/webm/720p.webm"); tipo != "video/webm" {
		t.Fatalf("webm: %q", tipo)
	}
	if _, _, ok := alm.leer(acme, "vid-1/vacio.m3u8"); !ok {
		t.Fatal("un fichero vacío se sube igual")
	}
	// Otra organización no ve nada: la clave lleva su prefijo.
	if _, _, ok := alm.leer(tenancy.Tenant{Slug: "globex"}, "vid-1/poster.jpg"); ok {
		t.Fatal("el prefijo de la organización se perdió")
	}

	if n, err := w.tamanoDe(ctx, acme, "vid-1/poster.jpg"); err != nil || n != 2048 {
		t.Fatalf("tamaño del póster: %d %v", n, err)
	}
	if n, err := w.tamanoDe(ctx, acme, "vid-1/no-esta.webp"); err != nil || n != 0 {
		t.Fatalf("lo que no existe ocupa 0: %d %v", n, err)
	}
}

func TestLatido(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), NombreLatido)
	ahora := time.Now()
	if err := ComprobarLatido(ruta, EdadMaximaLatido, ahora); err == nil {
		t.Fatal("sin fichero no hay latido")
	}
	if err := escribirLatido(ruta, ahora); err != nil {
		t.Fatal(err)
	}
	if err := ComprobarLatido(ruta, EdadMaximaLatido, ahora); err != nil {
		t.Fatalf("recién escrito: %v", err)
	}
	if err := ComprobarLatido(ruta, EdadMaximaLatido, ahora.Add(EdadMaximaLatido+time.Second)); err == nil {
		t.Fatal("un latido viejo es un trabajador colgado")
	}
	w := trabajadorDePrueba(nuevaColaFalsa(), nuevoAlmacenFalso(), t.TempDir())
	w.latir()
	if err := ComprobarLatido(w.latido, time.Minute, time.Now()); err != nil {
		t.Fatalf("latir: %v", err)
	}
}

func TestRutaLatido(t *testing.T) {
	cfg := configDe(t.TempDir())
	if got := RutaLatido(cfg); got != filepath.Join(cfg.TranscoderTmpDir, NombreLatido) {
		t.Fatalf("ruta: %s", got)
	}
	cfg.TranscoderTmpDir = ""
	if got := RutaLatido(cfg); got != filepath.Join(os.TempDir(), NombreLatido) {
		t.Fatalf("sin directorio, el temporal del sistema: %s", got)
	}
}

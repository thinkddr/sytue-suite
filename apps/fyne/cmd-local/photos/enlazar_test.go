// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// De una carpeta, las fotos y los vídeos se ENLAZAN (FO-09): la fototeca no copia sus
// originales, los vídeos entran con su póster, lo movido dentro de la carpeta se
// recoloca por su huella y, con «Copiar a la fototeca», las fotos se copian como antes.
func TestUnaCarpetaSeEnlazaYLoMovidoSeRecoloca(t *testing.T) {
	repo, cli := fototecaDePrueba(t)
	dir := t.TempDir()
	escribir := func(rel string, datos []byte) string {
		t.Helper()
		r := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(r), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(r, datos, 0o644); err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b := jpegDe(t, 10), jpegDe(t, 200)
	rutaA := escribir("playa.jpg", a)
	escribir("2024/montaña.jpg", b)
	webm, err := os.ReadFile("../../video/testdata/dos-segundos.webm")
	if err != nil {
		t.Fatal(err)
	}
	escribir("vídeos/cumple.webm", webm)
	escribir("notas.txt", []byte("hola"))

	carpetas := func() []string { return []string{dir} }
	g := soloImportables{GaleriaSistema: galeriaCarpetas{carpetas: carpetas}, videos: true}
	d := nuevoDestinoLocal(repo, cli)
	d.carpetas, d.medir = carpetas, medirVideo
	p, err := photos.NuevaCopia(g, "").Ronda(t.Context(), "local", d, nil)
	if err != nil || p.Subidas != 3 || p.Fallidas != 0 {
		t.Fatalf("ronda: %+v %v", p, err)
	}
	linea, _, err := cli.Timeline(t.Context(), "", 100, false, "")
	if err != nil || len(linea) != 3 {
		t.Fatal("la galería debe tener las dos fotos y el vídeo:", len(linea), err)
	}
	var video api.Photo
	for _, f := range linea {
		if f.Externo == "" {
			t.Fatalf("no está enlazada: %+v", f)
		}
		if f.EsVideo() {
			video = f
		}
	}
	if video.ID == "" || video.DurationMs < 1500 || video.DurationMs > 2500 || video.Width == 0 {
		t.Fatalf("el vídeo, con su duración y sus lados: %+v", video)
	}
	// Su miniatura sale del fotograma, no del póster neutro (un gris liso).
	r, err := cli.Variant(t.Context(), video.ID, api.PhotoThumb)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(r); err != nil {
		t.Fatal("la miniatura del vídeo:", err)
	}
	_ = r.Close()

	// Se mueve una foto dentro de la carpeta: se recoloca por su huella.
	movida := filepath.Join(dir, "2025", "playa (renombrada).jpg")
	if err := os.MkdirAll(filepath.Dir(movida), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(rutaA, movida); err != nil {
		t.Fatal(err)
	}
	lista, err := (galeriaCarpetas{carpetas: carpetas}).Listar(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rutas := make([]string, 0, len(lista))
	for _, e := range lista {
		rutas = append(rutas, e.ID)
	}
	if n, err := repo.Recolocar(t.Context(), rutas); err != nil || n != 1 {
		t.Fatal("recolocar:", n, err)
	}
	if f, ok, err := repo.FotoPorHuella(t.Context(), huella(a)); err != nil || !ok || f.Externo != movida || f.EstadoExterno != "" {
		t.Fatal("la movida:", f, ok, err)
	}

	// Con «Copiar a la fototeca», una foto nueva se copia.
	c := jpegDe(t, 90)
	escribir("copiada.jpg", c)
	d = nuevoDestinoLocal(repo, cli)
	d.carpetas, d.copiar, d.medir = carpetas, true, medirVideo
	if p, err := photos.NuevaCopia(g, "").Ronda(t.Context(), "local", d, nil); err != nil || p.Subidas != 1 {
		t.Fatalf("ronda copiando: %+v %v", p, err)
	}
	if f, ok, err := repo.FotoPorHuella(t.Context(), huella(c)); err != nil || !ok || f.Externo != "" {
		t.Fatal("con copiar, la foto debe estar copiada:", f, ok, err)
	}
}

// medirVideo lee en Go la duración y los lados de un WebM y saca su fotograma.
func TestMedirVideoDeUnWebM(t *testing.T) {
	m := medirVideo(t.Context(), "../../video/testdata/dos-segundos.webm")
	if m.duracion < 1500*time.Millisecond || m.duracion > 2500*time.Millisecond || m.ancho == 0 || m.alto == 0 {
		t.Fatalf("medida: %v %dx%d", m.duracion, m.ancho, m.alto)
	}
	img, _, err := image.Decode(bytes.NewReader(m.poster))
	if err != nil || img.Bounds().Dx() != m.ancho {
		t.Fatal("el póster:", err)
	}
}

func huella(datos []byte) string {
	s := sha256.Sum256(datos)
	return hex.EncodeToString(s[:])
}

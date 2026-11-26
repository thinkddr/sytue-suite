// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// HU-07: lo que no tiene fecha de captura entra con la de subida que se diga (la de
// Sytue, que ordenaba así la línea de tiempo): enlazado, foto o vídeo, al enlazarlo; y
// una foto copiada (una HEIC pasada a JPEG), después, con PonerFechas, que no pisa la
// de captura que ya tenga.
func TestLoTraidoSinFechaDeCapturaTomaLaDeSubida(t *testing.T) {
	ctx := t.Context()
	fuera := t.TempDir()
	repo, _, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	subida := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tomada := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)

	foto := filepath.Join(fuera, "nube.jpg")
	if err := os.WriteFile(foto, jpegConColor(t, 40, 30, color.RGBA{90, 90, 200, 255}), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: foto, SubidoEn: &subida})
	if err != nil {
		t.Fatal(err)
	}
	if f.TakenFromEXIF || !f.TakenAt.Equal(subida) {
		t.Errorf("la foto enlazada no va a su fecha de subida: %v (%v)", f.TakenAt, f.TakenFromEXIF)
	}
	video := filepath.Join(fuera, "paseo.webm")
	if err := os.WriteFile(video, []byte("\x1a\x45\xdf\xa3 no es un webm de verdad"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: video, Tipo: "video/webm", Duracion: 2 * time.Second, SubidoEn: &subida})
	if err != nil {
		t.Fatal(err)
	}
	if v.TakenFromEXIF || !v.TakenAt.Equal(subida) || v.DurationMs != 2000 {
		t.Errorf("el vídeo enlazado no va a su fecha de subida: %+v", v)
	}

	datos := jpegConColor(t, 30, 20, color.RGBA{20, 160, 20, 255})
	c, err := repo.ImportarFoto(ctx, uuid.NewString(), "IMG_0001.jpg", "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PonerFechas(ctx, c.ID, &tomada, &subida); err != nil {
		t.Fatal(err)
	}
	if c, err = repo.Photo(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if !c.TakenFromEXIF || !c.TakenAt.Equal(tomada) {
		t.Errorf("la copiada no tiene la fecha de captura dicha: %v (%v)", c.TakenAt, c.TakenFromEXIF)
	}
	otra := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.PonerFechas(ctx, c.ID, &otra, nil); err != nil {
		t.Fatal(err)
	}
	if c, _ = repo.Photo(ctx, c.ID); !c.TakenAt.Equal(tomada) {
		t.Errorf("PonerFechas ha pisado la fecha de captura que ya tenía: %v", c.TakenAt)
	}
	if err := repo.PonerFechas(ctx, c.ID, nil, nil); err != nil {
		t.Errorf("sin fechas no hay nada que poner: %v", err)
	}
}

// HU-07: un álbum traído guarda su origen, que sobrevive a renombrarlo y a reabrir la
// fototeca; dos con el mismo título son dos; uno borrado se sabe borrado.
func TestUnAlbumTraidoSeReconocePorSuOrigen(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, _, cerrar := abrirLocal(t, dir)
	a, err := repo.CrearAlbumDe(ctx, "Verano 2023", "sytue:a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CrearAlbumDe(ctx, "Verano 2023", "sytue:b")
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.CrearAlbumDe(ctx, "Invierno", "sytue:c")
	if err != nil {
		t.Fatal(err)
	}
	propio, err := repo.CreateAlbum(ctx, "Verano 2023")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RenameAlbum(ctx, a.ID, "Playa"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAlbum(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CrearAlbumDe(ctx, "Sin origen", " "); err == nil {
		t.Error("un origen vacío no es un origen")
	}
	cerrar()

	repo, _, cerrar = abrirLocal(t, dir)
	defer cerrar()
	por, err := repo.AlbumesPorOrigen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(por) != 3 || por["sytue:a"].ID != a.ID || por["sytue:b"].ID != b.ID || por["sytue:a"].Borrado || !por["sytue:c"].Borrado {
		t.Errorf("por origen: %+v", por)
	}
	for _, x := range por {
		if x.ID == propio.ID {
			t.Error("el álbum hecho aquí no tiene origen")
		}
	}
	albumes, err := repo.Albums(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(albumes) != 3 {
		t.Errorf("álbumes: %+v", albumes)
	}
	for _, x := range albumes {
		if x.ID == a.ID && x.Title != "Playa" {
			t.Errorf("el renombrado: %+v", x)
		}
	}
}

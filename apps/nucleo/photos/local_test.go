// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// abrirLocal monta Photos sobre el almacén en claro de Sytue Office, como
// `cmd-local/photos`: un solo repositorio, sin Drive ni servidor.
func abrirLocal(t *testing.T, dir string) (*photos.Repository, *api.Photos, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := photos.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		repo.Close()
		rec.Cerrar()
		t.Fatal("preparar:", err)
	}
	fachada, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, fachada, func() { repo.Close(); rec.Cerrar() }
}

func jpegDePrueba(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Una foto importada del disco sobrevive a cerrar y volver a abrir el almacén: sale en
// la línea de tiempo, su original vuelve byte a byte, su miniatura se puede leer y el
// original está en claro en la carpeta de objetos.
func TestPhotosImportaYSobreviveAReabrirElAlmacenEnClaro(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	foto := jpegDePrueba(t)
	suma := sha256.Sum256(foto)
	huella := hex.EncodeToString(suma[:])

	repo, fachada, cerrar := abrirLocal(t, dir)
	if !fachada.CanImportPrivatePhotos() {
		t.Fatal("la fachada local no sabe importar")
	}
	if _, ok, err := repo.FotoPorHuella(ctx, huella); err != nil || ok {
		t.Fatal("antes de importar no hay ninguna con esa huella", ok, err)
	}
	importada, err := fachada.ImportarFoto(ctx, uuid.NewString(), "playa.jpg", "image/jpeg", bytes.NewReader(foto), int64(len(foto)))
	if err != nil {
		t.Fatal("importar:", err)
	}
	if importada.Width != 64 || importada.Height != 48 {
		t.Fatalf("medidas %dx%d", importada.Width, importada.Height)
	}
	if err := fachada.SetFavorite(ctx, importada.ID, true); err != nil {
		t.Fatal(err)
	}
	cerrar()

	repo2, fachada2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	linea, _, err := fachada2.Timeline(ctx, "", 100, false, "")
	if err != nil || len(linea) != 1 || linea[0].ID != importada.ID || linea[0].Name != "playa.jpg" || !linea[0].Favorite {
		t.Fatal("línea de tiempo tras reabrir:", linea, err)
	}
	var original bytes.Buffer
	if err := repo2.Original(ctx, importada.ID, &original); err != nil || !bytes.Equal(original.Bytes(), foto) {
		t.Fatal("el original no vuelve igual:", err)
	}
	r, err := fachada2.Variant(ctx, importada.ID, api.PhotoThumb)
	if err != nil {
		t.Fatal("miniatura:", err)
	}
	mini, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(mini)); err != nil {
		t.Fatal("la miniatura no es una imagen:", err)
	}
	// La segunda vez que se abre el mismo fichero se encuentra por su huella.
	otra, ok, err := repo2.FotoPorHuella(ctx, huella)
	if err != nil || !ok || otra.ID != importada.ID {
		t.Fatal("no se encuentra por huella:", otra, ok, err)
	}
	// En claro: el original está tal cual, con su huella por nombre.
	if !hayFicheroIgual(t, filepath.Join(dir, "objetos"), foto) {
		t.Fatal("el original no está en claro en la carpeta de objetos")
	}
	// En la papelera ya no cuenta.
	if err := fachada2.Trash(ctx, importada.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo2.FotoPorHuella(ctx, huella); err != nil || ok {
		t.Fatal("una foto en la papelera no se reabre por huella", ok, err)
	}
	// Pero sigue siendo conocida: «Importar del carrete» no la vuelve a traer.
	if hs, err := repo2.HuellasConocidas(ctx); err != nil || len(hs) != 1 || !hs[huella] {
		t.Fatal("huellas conocidas", hs, err)
	}
}

func hayFicheroIgual(t *testing.T, raiz string, datos []byte) bool {
	t.Helper()
	hallado := false
	_ = filepath.WalkDir(raiz, func(ruta string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || hallado {
			return err
		}
		if b, err := os.ReadFile(ruta); err == nil && bytes.Equal(b, datos) {
			hallado = true
		}
		return nil
	})
	return hallado
}

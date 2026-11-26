// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// fototecaConAlmacen es abrirLocal con el almacén a mano (para compactar y mirar sus
// objetos) y LiberarAlPurgar, como `cmd-local/photos`.
func fototecaConAlmacen(t *testing.T, dir string) (*almacen.Recursos, *photos.Repository, *api.Photos, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := photos.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	repo.LiberarAlPurgar()
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	fachada, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	return rec, repo, fachada, func() { repo.Close(); rec.Cerrar() }
}

// Un vídeo COPIADO a la fototeca (FO-09: el del carrete del teléfono, el de «Añadir fotos»
// o el que se suelta en la ventana): su original queda en el almacén tal cual (en claro,
// con su huella por nombre, y RutaEnClaro lo encuentra para el reproductor), la miniatura
// sale del póster, tiene su duración, se reconoce por su huella, no se edita, sobrevive a
// reabrir, se exporta y, borrado para siempre, se va del disco al compactar.
func TestUnVideoCopiadoEntraEnLaFototeca(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	rec, repo, fachada, cerrar := fototecaConAlmacen(t, dir)
	defer func() { cerrar() }() //nolint:gocritic // cerrar se reasigna más abajo: el diferido tiene que cerrar el último que se abrió
	video, err := os.ReadFile("../local/conversor/testdata/corto.mp4")
	if err != nil {
		t.Fatal(err)
	}
	poster := jpegConColor(t, 320, 180, color.RGBA{30, 200, 30, 255})
	grabado := time.Date(2022, 12, 24, 20, 15, 0, 0, time.UTC)
	v, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{
		Nombre: "VID_0001.mp4", Tipo: "video/MP4; codecs=avc1",
		Datos: bytes.NewReader(video), Tamano: int64(len(video)), Poster: poster, Duracion: 2500 * time.Millisecond, TomadoEn: &grabado,
	})
	if err != nil {
		t.Fatal("copiar el vídeo:", err)
	}
	if !v.EsVideo() || v.ContentType != "video/mp4" || v.DurationMs != 2500 || v.Width != 320 || v.Height != 180 ||
		v.SizeBytes != int64(len(video)) || !v.TakenAt.Equal(grabado) || v.Externo != "" || v.Name != "VID_0001.mp4" {
		t.Fatalf("el vídeo: %+v", v)
	}
	if img := leerVariante(t, fachada, v.ID); img.Bounds().Dx() != 320 {
		t.Fatal("la miniatura no sale del póster:", img.Bounds())
	}
	var original bytes.Buffer
	if err := repo.Original(ctx, v.ID, &original); err != nil || !bytes.Equal(original.Bytes(), video) {
		t.Fatal("el original del vídeo:", err)
	}
	huella, err := repo.HuellaOriginal(ctx, v.ID)
	if err != nil || huella != huellaDe(video) {
		t.Fatal("la huella del original:", huella, err)
	}
	ruta, ok := rec.Puerto.RutaEnClaro(huella)
	if !ok {
		t.Fatal("el original del vídeo no está en claro en el almacén")
	}
	if en, err := os.ReadFile(ruta); err != nil || !bytes.Equal(en, video) {
		t.Fatal("lo que hay en su ruta no es el vídeo:", err)
	}
	if f, ok, err := repo.FotoPorHuella(ctx, huella); err != nil || !ok || f.ID != v.ID {
		t.Fatal("por su huella:", f, ok, err)
	}
	receta := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}}}}
	if _, err := repo.SaveRecipe(ctx, v.ID, receta, ""); err == nil {
		t.Fatal("un vídeo copiado no se edita")
	}

	// Sin póster, uno neutro con los lados que se dicen; y lo que no es un vídeo no entra
	// por aquí, ni un vídeo por ImportarFoto (lo de siempre).
	otro := []byte("otro vídeo, este sin fotograma")
	w, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{
		Nombre: "sin póster.webm", Tipo: "video/webm",
		Datos: bytes.NewReader(otro), Tamano: int64(len(otro)), Ancho: 1920, Alto: 1080,
	})
	if err != nil || w.Width != 1920 || w.Height != 1080 {
		t.Fatal("sin póster:", w, err)
	}
	leerVariante(t, fachada, w.ID)
	if _, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{
		Nombre: "foto.jpg", Tipo: "image/jpeg",
		Datos: bytes.NewReader(poster), Tamano: int64(len(poster)),
	}); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("una foto por ImportarVideo:", err)
	}
	if _, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{
		Nombre: "corto.mp4", Tipo: "video/mp4",
		Datos: bytes.NewReader(video[:10]), Tamano: int64(len(video)),
	}); err == nil {
		t.Fatal("un vídeo que no llega entero ha entrado")
	}
	if _, err := fachada.ImportarFoto(ctx, uuid.NewString(), "corto.mp4", "video/mp4", bytes.NewReader(video), int64(len(video))); err == nil {
		t.Fatal("ImportarFoto no mide vídeos")
	}
	if pendientes, err := repo.ReanudarImportaciones(ctx); err != nil || len(pendientes) != 0 {
		t.Fatal("no debe quedar nada a medias:", pendientes, err)
	}

	// Tras reabrir sigue ahí, con su duración, y se exporta tal cual a su año y mes.
	cerrar()
	rec, repo, fachada, cerrar = fototecaConAlmacen(t, dir)
	linea, _, err := fachada.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 2 {
		t.Fatal("tras reabrir:", linea, err)
	}
	for _, f := range linea {
		if !f.EsVideo() || f.Externo != "" || f.ID == v.ID && f.DurationMs != 2500 {
			t.Fatalf("tras reabrir: %+v", f)
		}
	}
	exportada := t.TempDir()
	if res, err := repo.ExportarFototeca(ctx, exportada, nil); err != nil || res.Copiadas != 2 || len(res.Fallidas) != 0 {
		t.Fatal("exportar:", res, err)
	}
	if en, err := os.ReadFile(filepath.Join(exportada, "2022", "12", "VID_0001.mp4")); err != nil || !bytes.Equal(en, video) {
		t.Fatal("el vídeo exportado:", err)
	}
	if res, err := repo.ExportarFototeca(ctx, exportada, nil); err != nil || res.YaEstaban != 2 {
		t.Fatal("exportar dos veces no duplica:", res, err)
	}

	// A la papelera y borrado para siempre: compactar lo quita del disco, y su huella
	// sigue conocida (el carrete no lo vuelve a traer).
	if err := fachada.Trash(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := fachada.Purgar(ctx, ""); err != nil || n != 1 {
		t.Fatal("purgar:", n, err)
	}
	if _, err := rec.Compactar(ctx); err != nil {
		t.Fatal("compactar:", err)
	}
	if _, ok := rec.Puerto.RutaEnClaro(huella); ok {
		t.Fatal("el vídeo borrado para siempre sigue en el almacén")
	}
	if hs, err := repo.HuellasConocidas(ctx); err != nil || !hs[huella] {
		t.Fatal("lo purgado debe seguir conocido:", err)
	}
}

// repetido da n bytes de un patrón sin tenerlos todos en memoria.
type repetido struct{ quedan int64 }

func (r *repetido) Read(p []byte) (int, error) {
	if r.quedan <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.quedan)
	for i := range p[:n] {
		p[i] = byte((r.quedan - int64(i)) % 251)
	}
	r.quedan -= n
	return int(n), nil
}

// Un vídeo copiado puede pasar de lo que la ingesta sabe derivar de una foto
// (photorender.MaxOriginal): nunca se descodifica. Una foto así sigue sin entrar.
func TestUnVideoCopiadoPuedePasarDelTopeDeUnaFoto(t *testing.T) {
	if testing.Short() {
		t.Skip("escribe un vídeo de más de 96 MB")
	}
	ctx := t.Context()
	_, repo, fachada, cerrar := fototecaConAlmacen(t, t.TempDir())
	defer cerrar()
	tam := int64(photorender.MaxOriginal + 1<<20)
	h := sha256.New()
	if _, err := io.Copy(h, &repetido{tam}); err != nil {
		t.Fatal(err)
	}
	v, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{Nombre: "largo.mov", Tipo: "video/quicktime", Datos: &repetido{tam}, Tamano: tam})
	if err != nil || v.SizeBytes != tam {
		t.Fatal("un vídeo grande:", v, err)
	}
	if huella, err := repo.HuellaOriginal(ctx, v.ID); err != nil || huella != hex.EncodeToString(h.Sum(nil)) {
		t.Fatal("su huella:", err)
	}
	if _, err := fachada.ImportarFoto(ctx, uuid.NewString(), "grande.jpg", "image/jpeg", &repetido{tam}, tam); err == nil {
		t.Fatal("una foto de más de MaxOriginal ha entrado")
	}
}

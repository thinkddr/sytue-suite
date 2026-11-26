// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
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
)

func huellaDe(datos []byte) string {
	s := sha256.Sum256(datos)
	return hex.EncodeToString(s[:])
}

func jpegConColor(t *testing.T, ancho, alto int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := range alto {
		for x := range ancho {
			img.Set(x, y, color.RGBA{c.R + uint8(x), c.G + uint8(y), c.B, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func leerVariante(t *testing.T, f *api.Photos, id string) image.Image {
	t.Helper()
	r, err := f.Variant(t.Context(), id, api.PhotoThumb)
	if err != nil {
		t.Fatal("miniatura:", err)
	}
	defer r.Close()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal("la miniatura no es una imagen:", err)
	}
	return img
}

// Una foto de una carpeta del equipo se ENLAZA (FO-09): su original no se copia al
// almacén (solo sus derivadas), se lee del fichero, sobrevive a reabrir; si el fichero
// desaparece la galería lo dice sin romperse, si se mueve se recoloca por su huella y,
// si cambia, lo dice. Y se puede editar.
func TestUnOriginalExternoSeEnlazaSinCopiar(t *testing.T) {
	dir, fuera := t.TempDir(), t.TempDir()
	ctx := t.Context()
	foto := jpegConColor(t, 80, 60, color.RGBA{10, 20, 200, 255})
	ruta := filepath.Join(fuera, "playa.jpg")
	if err := os.WriteFile(ruta, foto, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, fachada, cerrar := abrirLocal(t, dir)
	enlazada, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: ruta})
	if err != nil {
		t.Fatal("enlazar:", err)
	}
	if enlazada.Externo != ruta || enlazada.EstadoExterno != "" || enlazada.Width != 80 || enlazada.ContentType != "image/jpeg" || enlazada.Name != "playa.jpg" {
		t.Fatalf("la enlazada: %+v", enlazada)
	}
	if hayFicheroIgual(t, filepath.Join(dir, "objetos"), foto) {
		t.Fatal("el original se ha copiado al almacén")
	}
	leerVariante(t, fachada, enlazada.ID)
	if hs, err := repo.HuellasConocidas(ctx); err != nil || !hs[huellaDe(foto)] {
		t.Fatal("la huella del enlazado no se conoce:", hs, err)
	}
	if otra, ok, err := repo.FotoPorHuella(ctx, huellaDe(foto)); err != nil || !ok || otra.ID != enlazada.ID {
		t.Fatal("no se encuentra por huella:", otra, ok, err)
	}
	// Dos veces el mismo trabajo no duplica.
	if otra, err := repo.ImportarExterno(ctx, enlazada.ID, photos.OrigenExterno{Ruta: ruta}); err != nil || otra.ID != enlazada.ID {
		t.Fatal("repetir el trabajo:", otra, err)
	}
	cerrar()

	repo, fachada, cerrar = abrirLocal(t, dir)
	defer cerrar()
	linea, _, err := fachada.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 1 || linea[0].Externo != ruta || linea[0].EstadoExterno != "" {
		t.Fatal("tras reabrir:", linea, err)
	}
	var original bytes.Buffer
	if err := repo.Original(ctx, enlazada.ID, &original); err != nil || !bytes.Equal(original.Bytes(), foto) {
		t.Fatal("el original no se lee del fichero:", err)
	}
	// Se edita como cualquier otra: la receta lee el original del fichero.
	receta := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}}}}
	if _, err := repo.SaveRecipe(ctx, enlazada.ID, receta, ""); err != nil {
		t.Fatal("editar la enlazada:", err)
	}

	// El fichero se va: la galería sigue, con su miniatura, y dice que falta.
	movida := filepath.Join(fuera, "2024", "playa renombrada.jpg")
	if err := os.MkdirAll(filepath.Dir(movida), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(ruta, movida); err != nil {
		t.Fatal(err)
	}
	linea, _, err = fachada.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 1 || linea[0].EstadoExterno != api.ExternoFalta {
		t.Fatal("sin fichero:", linea, err)
	}
	leerVariante(t, fachada, enlazada.ID)
	if err := repo.Original(ctx, enlazada.ID, io.Discard); !errors.Is(err, photos.ErrOriginalExterno) {
		t.Fatal("sin fichero, el original debe decirlo:", err)
	}
	// Vuelve a aparecer en otro sitio: se recoloca por su huella (otra con el mismo
	// tamaño y otros bytes no confunde).
	impostora := filepath.Join(fuera, "otra.jpg")
	falsa := bytes.Clone(foto)
	falsa[len(falsa)-3] ^= 0xff
	if err := os.WriteFile(impostora, falsa, 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := repo.Recolocar(ctx, []string{impostora, movida})
	if err != nil || n != 1 {
		t.Fatal("recolocar:", n, err)
	}
	f, err := fachada.Photo(ctx, enlazada.ID)
	if err != nil || f.Externo != movida || f.EstadoExterno != "" {
		t.Fatal("tras recolocar:", f, err)
	}
	if err := repo.Original(ctx, enlazada.ID, io.Discard); err != nil {
		t.Fatal("tras recolocar, el original:", err)
	}
	// Cambia por fuera: lo dice.
	if err := os.WriteFile(movida, append(bytes.Clone(foto), 0, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := fachada.Photo(ctx, enlazada.ID); err != nil || f.EstadoExterno != api.ExternoCambiado {
		t.Fatal("cambiado:", f, err)
	}
	if err := repo.Original(ctx, enlazada.ID, io.Discard); !errors.Is(err, photos.ErrOriginalExterno) {
		t.Fatal("cambiado, el original debe decirlo:", err)
	}
}

// Un vídeo entra en la galería como original externo, con su duración y su póster
// (FO-09): la miniatura sale del fotograma, el original es el fichero y no se edita.
// Sin fotograma, un póster neutro.
func TestUnVideoEntraEnLaGaleriaConSuPoster(t *testing.T) {
	dir, fuera := t.TempDir(), t.TempDir()
	ctx := t.Context()
	repo, fachada, cerrar := abrirLocal(t, dir)
	defer func() { cerrar() }() //nolint:gocritic // cerrar se reasigna más abajo: el diferido tiene que cerrar el último que se abrió
	video := bytes.Repeat([]byte("no es un mp4 de verdad "), 4096)
	ruta := filepath.Join(fuera, "cumple.mp4")
	if err := os.WriteFile(ruta, video, 0o600); err != nil {
		t.Fatal(err)
	}
	poster := jpegConColor(t, 320, 180, color.RGBA{200, 30, 30, 255})
	grabado := time.Date(2023, 7, 14, 18, 30, 0, 0, time.UTC)
	v, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: ruta, Tipo: "video/mp4", Poster: poster, Duracion: 83 * time.Second, TomadoEn: &grabado})
	if err != nil {
		t.Fatal("importar el vídeo:", err)
	}
	if !v.EsVideo() || v.DurationMs != 83000 || v.Width != 320 || v.Height != 180 || v.SizeBytes != int64(len(video)) || !v.TakenAt.Equal(grabado) {
		t.Fatalf("el vídeo: %+v", v)
	}
	if img := leerVariante(t, fachada, v.ID); img.Bounds().Dx() != 320 {
		t.Fatal("la miniatura no sale del póster:", img.Bounds())
	}
	var original bytes.Buffer
	if err := repo.Original(ctx, v.ID, &original); err != nil || !bytes.Equal(original.Bytes(), video) {
		t.Fatal("el original del vídeo:", err)
	}
	if ruta2, estado, ok, err := repo.RutaExterna(ctx, v.ID); err != nil || !ok || ruta2 != ruta || estado != "" {
		t.Fatal("ruta externa:", ruta2, estado, ok, err)
	}
	receta := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}}}}
	if _, err := repo.SaveRecipe(ctx, v.ID, receta, ""); err == nil {
		t.Fatal("un vídeo no se edita")
	}
	sinPoster := filepath.Join(fuera, "sin poster.webm")
	if err := os.WriteFile(sinPoster, []byte("otro vídeo"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: sinPoster, Tipo: "video/webm", Ancho: 1920, Alto: 1080})
	if err != nil {
		t.Fatal("sin póster:", err)
	}
	leerVariante(t, fachada, w.ID)
	cerrar()
	_, fachada, cerrar = abrirLocal(t, dir)
	linea, _, err := fachada.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 2 {
		t.Fatal("tras reabrir:", linea, err)
	}
	for _, f := range linea {
		if !f.EsVideo() || f.Externo == "" {
			t.Fatalf("tras reabrir: %+v", f)
		}
	}
}

// Con LiberarAlPurgar, vaciar la papelera suelta los originales y compactar el almacén
// los quita del disco: el de una importada desaparece de `objetos/` (el de un fichero
// enlazado NO se toca, es de la persona), lo vivo se queda y lo borrado no se vuelve a
// importar (su huella sigue conocida).
func TestPurgarYCompactarLiberaLosOriginales(t *testing.T) {
	dir, fuera := t.TempDir(), t.TempDir()
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := photos.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	repo.LiberarAlPurgar()
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	fachada, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	a, b := jpegConColor(t, 60, 40, color.RGBA{1, 2, 3, 255}), jpegConColor(t, 60, 40, color.RGBA{90, 2, 3, 255})
	c := jpegConColor(t, 60, 40, color.RGBA{1, 90, 3, 255})
	fa, err := fachada.ImportarFoto(ctx, uuid.NewString(), "a.jpg", "image/jpeg", bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	fb, err := fachada.ImportarFoto(ctx, uuid.NewString(), "b.jpg", "image/jpeg", bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	rutaC := filepath.Join(fuera, "c.jpg")
	if err := os.WriteFile(rutaC, c, 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: rutaC})
	if err != nil {
		t.Fatal(err)
	}
	// Un álbum con las tres (sus miembros van en páginas retenidas que cita su índice,
	// también retenido) y una edición de B (sus derivadas, también retenidas): compactar
	// las tiene que seguir por sus referencias.
	album, err := fachada.CreateAlbum(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := fachada.AddToAlbum(ctx, album.ID, []string{fa.ID, fb.ID, fc.ID}); err != nil || n != 3 {
		t.Fatal("álbum:", n, err)
	}
	receta := model.Receta{Version: 1, Pasos: []model.Paso{{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}}}}
	huellaReceta, err := repo.SaveRecipe(ctx, fb.ID, receta, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{fa.ID, fc.ID} {
		if err := fachada.Trash(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	antes := contarObjetos(t, dir)
	if n, err := fachada.Purgar(ctx, ""); err != nil || n != 2 {
		t.Fatal("purgar:", n, err)
	}
	res, err := rec.Compactar(ctx)
	if err != nil {
		t.Fatal("compactar:", err)
	}
	// Se van el original de A y las derivadas de A y de C (en fotos tan pequeñas las tres
	// derivadas son los mismos bytes: un objeto por foto) y los índices viejos del álbum;
	// quedan el original de B, sus derivadas (la de antes y la editada) y el álbum.
	if res.Objetos < 3 || contarObjetos(t, dir) != antes-res.Objetos {
		t.Fatalf("no liberó lo de las purgadas: %+v (antes %d)", res, antes)
	}
	if hayFicheroIgual(t, filepath.Join(dir, "objetos"), a) {
		t.Fatal("el original de la purgada sigue en el almacén")
	}
	if !hayFicheroIgual(t, filepath.Join(dir, "objetos"), b) {
		t.Fatal("se ha ido el original de una viva")
	}
	if _, err := os.Stat(rutaC); err != nil {
		t.Fatal("compactar ha tocado un fichero enlazado de la persona:", err)
	}
	// En la misma sesión, tras refrescar el motor, todo se lee.
	if err := repo.RefreshPrivateData(ctx); err != nil {
		t.Fatal("refrescar:", err)
	}
	leerVariante(t, fachada, fb.ID)
	repo.Close()
	rec.Cerrar()

	repo2, fachada2, cerrar2 := abrirLocal(t, dir)
	defer cerrar2()
	linea, _, err := fachada2.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 1 || linea[0].ID != fb.ID {
		t.Fatal("tras compactar y reabrir:", linea, err)
	}
	var original bytes.Buffer
	if err := repo2.Original(ctx, fb.ID, &original); err != nil || !bytes.Equal(original.Bytes(), b) {
		t.Fatal("el original vivo:", err)
	}
	enAlbum, err := fachada2.AlbumPhotos(ctx, album.ID)
	if err != nil || len(enAlbum) != 1 || enAlbum[0].ID != fb.ID {
		t.Fatal("el álbum tras compactar y reabrir:", enAlbum, err)
	}
	r, err := fachada2.VariantDe(ctx, fb.ID, api.PhotoThumb, huellaReceta)
	if err != nil {
		t.Fatal("la derivada editada tras compactar:", err)
	}
	editada, err := io.ReadAll(r)
	_ = r.Close()
	if _, _, derr := image.Decode(bytes.NewReader(editada)); err != nil || derr != nil {
		t.Fatal("la derivada editada tras compactar no se lee:", err, derr)
	}
	hs, err := repo2.HuellasConocidas(ctx)
	if err != nil || !hs[huellaDe(a)] || !hs[huellaDe(c)] || !hs[huellaDe(b)] {
		t.Fatal("lo purgado debe seguir conocido (no se vuelve a importar):", hs, err)
	}
	if papelera, err := fachada2.Papelera(ctx); err != nil || len(papelera.Items) != 0 {
		t.Fatal("papelera:", papelera, err)
	}
}

func contarObjetos(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(filepath.Join(dir, "objetos"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// Una foto enlazada sin fecha en su cabecera toma la que se diga (la que sabía Sytue, al
// traerla de una copia de «Tus datos»); sin decirla, no tiene ninguna, como siempre.
func TestUnaFotoEnlazadaSinFechaTomaLaQueSeDiga(t *testing.T) {
	ctx := t.Context()
	fuera := t.TempDir()
	repo, _, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	hecha := time.Date(2019, 8, 15, 19, 45, 0, 0, time.FixedZone("CEST", 2*3600))
	conFecha := filepath.Join(fuera, "atardecer.jpg")
	sinFecha := filepath.Join(fuera, "nube.jpg")
	if err := os.WriteFile(conFecha, jpegConColor(t, 40, 30, color.RGBA{200, 90, 20, 255}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sinFecha, jpegConColor(t, 40, 30, color.RGBA{90, 90, 200, 255}), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: conFecha, TomadoEn: &hecha})
	if err != nil {
		t.Fatal(err)
	}
	if !f.TakenFromEXIF || !f.TakenAt.Equal(hecha) {
		t.Errorf("la fecha dicha no se ha puesto: %v (%v)", f.TakenAt, f.TakenFromEXIF)
	}
	g, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: sinFecha})
	if err != nil {
		t.Fatal(err)
	}
	if g.TakenFromEXIF {
		t.Errorf("sin decirla, una foto sin EXIF no tiene fecha de cuando se hizo: %v", g.TakenAt)
	}
}

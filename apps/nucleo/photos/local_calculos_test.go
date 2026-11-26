// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

func jpegDeColor(t *testing.T, tono uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := range 24 {
		for x := range 32 {
			img.Set(x, y, color.RGBA{tono, uint8(x * 8), uint8(y * 10), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Lo que el servicio calcula y en Sytue Office calcula el repositorio (FO-02, FO-05),
// por la fachada que usan las pantallas: años y salto a un año, repetidas por huella,
// portada de álbum, borrar para siempre y vaciar la papelera, y exportar la fototeca
// por año y mes sin duplicar al repetir.
func TestLaFototecaLocalCalculaLoQueHaciaElServicio(t *testing.T) {
	ctx := t.Context()
	repo, f, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	importar := func(nombre string, datos []byte) api.Photo {
		t.Helper()
		foto, err := f.ImportarFoto(ctx, uuid.NewString(), nombre, "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
		if err != nil {
			t.Fatal("importar", nombre, err)
		}
		return foto
	}
	roja, verde := jpegDeColor(t, 250), jpegDeColor(t, 10)
	a := importar("roja.jpg", roja)
	b := importar("verde.jpg", verde)
	c := importar("roja-copia.jpg", roja)

	anyo := time.Now().UTC().Year()
	anyos, err := f.TimelineYears(ctx, false)
	if err != nil || len(anyos) != 1 || anyos[0] != (api.TimelineYear{Year: anyo, Count: 3}) {
		t.Fatal("años:", anyos, err)
	}
	desde, _, err := f.TimelineDesde(ctx, strconv.Itoa(anyo), 60, false)
	if err != nil || len(desde) != 3 {
		t.Fatal("desde este año:", len(desde), err)
	}
	if antes, _, err := f.TimelineDesde(ctx, strconv.Itoa(anyo-1), 60, false); err != nil || len(antes) != 0 {
		t.Fatal("desde el año pasado:", len(antes), err)
	}

	grupos, err := f.Duplicates(ctx, 50)
	if err != nil || len(grupos) != 1 || len(grupos[0]) != 2 || grupos[0][0].ID != a.ID || grupos[0][1].ID != c.ID {
		t.Fatal("repetidas:", grupos, err)
	}
	if cerca, err := f.Nearby(ctx, a.ID, 10); err != nil || len(cerca) != 0 {
		t.Fatal("sin GPS no hay nada cerca:", cerca, err)
	}
	if _, err := f.Memories(ctx); err != nil {
		t.Fatal("recuerdos:", err)
	}

	album, err := f.CreateAlbum(ctx, "Colores")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.AddToAlbum(ctx, album.ID, []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.PonerPortada(ctx, album.ID, "Colores", c.ID); err == nil {
		t.Fatal("una foto que no está en el álbum ha sido portada")
	}
	if err := f.PonerPortada(ctx, album.ID, "Colores", b.ID); err != nil {
		t.Fatal("portada:", err)
	}
	if got, err := f.Album(ctx, album.ID); err != nil || got.CoverAssetID != b.ID {
		t.Fatal("la portada no se queda:", got, err)
	}

	// Exportar: originales tal cual en AAAA/MM; la copia repetida con otro nombre va
	// aparte, y repetir la exportación no escribe nada nuevo.
	dir := t.TempDir()
	resumen, err := repo.ExportarFototeca(ctx, dir, nil)
	if err != nil || resumen.Copiadas != 3 || resumen.YaEstaban != 0 || len(resumen.Fallidas) != 0 {
		t.Fatal("exportar:", resumen, err)
	}
	mes := filepath.Join(dir, strconv.Itoa(anyo), time.Now().UTC().Format("01"))
	if datos, err := os.ReadFile(filepath.Join(mes, "roja.jpg")); err != nil || !bytes.Equal(datos, roja) {
		t.Fatal("el original exportado no es el mismo:", err)
	}
	if otra, err := repo.ExportarFototeca(ctx, dir, nil); err != nil || otra.Copiadas != 0 || otra.YaEstaban != 3 {
		t.Fatal("exportar dos veces:", otra, err)
	}
	if err := os.WriteFile(filepath.Join(mes, "verde.jpg"), []byte("otra cosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if otra, err := repo.ExportarFototeca(ctx, dir, nil); err != nil || otra.Copiadas != 1 {
		t.Fatal("con un fichero distinto del mismo nombre:", otra, err)
	}
	if datos, err := os.ReadFile(filepath.Join(mes, "verde (2).jpg")); err != nil || !bytes.Equal(datos, verde) {
		t.Fatal("no se escribió aparte sin pisar:", err)
	}

	// Borrar para siempre una, y luego vaciar la papelera.
	for _, id := range []string{a.ID, b.ID, c.ID} {
		if err := f.Trash(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := f.Purgar(ctx, a.ID); err != nil || n != 1 {
		t.Fatal("purgar una:", n, err)
	}
	if err := f.Restaurar(ctx, a.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("una purgada se ha restaurado")
	}
	papelera, err := f.Papelera(ctx)
	if err != nil || len(papelera.Items) != 2 {
		t.Fatal("papelera tras purgar una:", papelera.Items, err)
	}
	if n, err := f.Purgar(ctx, ""); err != nil || n != 2 {
		t.Fatal("vaciar:", n, err)
	}
	if papelera, err := f.Papelera(ctx); err != nil || len(papelera.Items) != 0 {
		t.Fatal("la papelera no está vacía:", papelera.Items, err)
	}
	if grupos, err := f.Duplicates(ctx, 50); err != nil || len(grupos) != 0 {
		t.Fatal("repetidas tras vaciar:", grupos, err)
	}
	if album, err := f.Album(ctx, album.ID); err != nil || album.Count != 0 {
		t.Fatal("el álbum sigue contando purgadas:", album, err)
	}
}

// WebP entra como cualquier foto cuando se admite (FO-03).
func TestLaFototecaLocalImportaWebP(t *testing.T) {
	raw, err := os.ReadFile("testdata/foto.webp")
	if err != nil {
		t.Fatal(err)
	}
	repo, f, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	if _, err := f.ImportarFoto(t.Context(), uuid.NewString(), "a.webp", "image/webp", bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("sin admitirlo, un WebP ha entrado")
	}
	repo.AdmitirFormatos("webp")
	foto, err := f.ImportarFoto(t.Context(), uuid.NewString(), "a.webp", "image/webp", bytes.NewReader(raw), int64(len(raw)))
	if err != nil || foto.Width != 96 || foto.Height != 64 {
		t.Fatal("webp:", foto, err)
	}
	if _, err := f.Variant(t.Context(), foto.ID, api.PhotoLarge); err != nil {
		t.Fatal("variante:", err)
	}
}

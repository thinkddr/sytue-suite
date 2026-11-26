// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestEtiquetasLibresSiguenAlFicheroYSeBuscan(t *testing.T) {
	a, base := abrirPrueba(t)
	ctx := t.Context()
	r := filepath.Join(a.Raiz(), "Viajes", "billete.pdf")
	escribirPrueba(t, r, "%PDF")
	id := idDe(r)
	if err := a.PonerEtiquetas(ctx, id, []string{"  viaje  2026 ", "Facturas", "facturas", ""}); err != nil {
		t.Fatal(err)
	}
	if ls, _ := a.Etiquetas(ctx, id); !slices.Equal(ls, []string{"Facturas", "viaje 2026"}) {
		t.Fatal("etiquetas limpias y ordenadas", ls)
	}
	// Se buscan por etiqueta aunque el nombre no coincida.
	res, err := a.Search(ctx, "factur")
	if err != nil || len(res.Files) != 1 || res.Files[0].Path != r {
		t.Fatal("buscar por etiqueta", res, err)
	}
	// Renombrar desde aquí se las lleva.
	if err := a.Rename(ctx, api.DriveFile, id, "ida.pdf"); err != nil {
		t.Fatal(err)
	}
	nuevo := filepath.Join(a.Raiz(), "Viajes", "ida.pdf")
	if ls, _ := a.Etiquetas(ctx, idDe(nuevo)); len(ls) != 2 {
		t.Fatal("el renombrado perdió las etiquetas", ls)
	}
	// Mover la carpeta, también.
	destino := filepath.Join(a.Raiz(), "Archivo")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := a.Move(ctx, api.DriveFolder, idDe(filepath.Dir(nuevo)), idDe(destino)); err != nil {
		t.Fatal(err)
	}
	movido := filepath.Join(destino, "Viajes", "ida.pdf")
	if ls, _ := a.Etiquetas(ctx, idDe(movido)); len(ls) != 2 {
		t.Fatal("mover la carpeta perdió las etiquetas", ls)
	}
	if todas := a.TodasLasEtiquetas(ctx); !slices.Equal(todas, []string{"Facturas", "viaje 2026"}) {
		t.Fatal("todas", todas)
	}
	// A la papelera se van con él, y vuelven al restaurar.
	if err := a.Delete(ctx, api.DriveFile, idDe(movido)); err != nil {
		t.Fatal(err)
	}
	if todas := a.TodasLasEtiquetas(ctx); len(todas) != 0 {
		t.Fatal("lo tirado no debe ofrecer sus etiquetas", todas)
	}
	escribirPrueba(t, movido, "otro fichero con el mismo nombre")
	if ls, _ := a.Etiquetas(ctx, idDe(movido)); len(ls) != 0 {
		t.Fatal("un fichero nuevo en la misma ruta heredó etiquetas", ls)
	}
	if err := os.Remove(movido); err != nil {
		t.Fatal(err)
	}
	if err := a.Restore(ctx, api.DriveFile, idDe(movido)); err != nil {
		t.Fatal(err)
	}
	if ls, _ := a.Etiquetas(ctx, idDe(movido)); len(ls) != 2 {
		t.Fatal("restaurar no devolvió las etiquetas", ls)
	}
	// Quitarlas todas las borra del índice, que sobrevive a reabrir.
	if err := a.PonerEtiquetas(ctx, idDe(movido), []string{"solo una"}); err != nil {
		t.Fatal(err)
	}
	b, err := Abrir(filepath.Join(base, "Drive"), Opciones{Instalacion: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	if ls, _ := b.Etiquetas(ctx, idDe(movido)); !slices.Equal(ls, []string{"solo una"}) {
		t.Fatal("el índice no guardó las etiquetas", ls)
	}
	if err := b.PonerEtiquetas(ctx, idDe(movido), nil); err != nil {
		t.Fatal(err)
	}
	if len(b.ind.Etiquetas) != 0 {
		t.Fatal("quitar todas debe vaciar la entrada", b.ind.Etiquetas)
	}
}

func TestMiniaturasEnCacheDeDisco(t *testing.T) {
	base := t.TempDir()
	cache := filepath.Join(base, "cache")
	a, err := Abrir(filepath.Join(base, "Drive"), Opciones{Instalacion: "i", Miniaturas: cache})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 640, 480))); err != nil {
		t.Fatal(err)
	}
	r := filepath.Join(a.Raiz(), "foto.png")
	escribirPrueba(t, r, b.String())
	_ = leer(a.Thumbnail(t.Context(), idDe(r)))(t)
	var guardadas []string
	_ = filepath.WalkDir(cache, func(p string, e os.DirEntry, _ error) error {
		if e != nil && !e.IsDir() {
			guardadas = append(guardadas, p)
		}
		return nil
	})
	if len(guardadas) != 1 {
		t.Fatal("la miniatura no quedó en la caché", guardadas)
	}
	// La segunda sale de la caché: se pisa el fichero de la caché y se comprueba que lo
	// que vuelve es eso.
	if err := os.WriteFile(guardadas[0], []byte("de la caché"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := leer(a.Thumbnail(t.Context(), idDe(r)))(t); got != "de la caché" {
		t.Fatal("no se leyó de la caché", got)
	}
	// Si el fichero cambia, la clave cambia y se hace de nuevo.
	futuro := time.Now().Add(time.Hour)
	if err := os.Chtimes(r, futuro, futuro); err != nil {
		t.Fatal(err)
	}
	if got := leer(a.Thumbnail(t.Context(), idDe(r)))(t); got == "de la caché" {
		t.Fatal("un fichero cambiado no debe servir la miniatura vieja")
	}
}

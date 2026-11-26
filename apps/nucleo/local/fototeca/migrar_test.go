// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/contrato"
)

// visto es lo que la pantalla ve de una fototeca: para comparar antes y después.
type visto struct {
	linea, papelera []api.Photo
	albumes         []api.Album
	enAlbum         map[string][]string
	conocidas       map[string]bool
	porOrigen       map[string]photos.AlbumDeOrigen
	recetas         map[string]string
}

func ver(t *testing.T, f contrato.Fototeca) visto {
	t.Helper()
	ctx := t.Context()
	var v visto
	for cursor := ""; ; {
		p, siguiente, err := f.Timeline(ctx, cursor, 7, false, "")
		if err != nil {
			t.Fatal(err)
		}
		v.linea = append(v.linea, p...)
		if siguiente == "" {
			break
		}
		cursor = siguiente
	}
	var err error
	if v.papelera, err = f.Papelera(ctx); err != nil {
		t.Fatal(err)
	}
	if v.albumes, err = f.Albums(ctx); err != nil {
		t.Fatal(err)
	}
	v.enAlbum = map[string][]string{}
	for _, a := range v.albumes {
		fs, err := f.AlbumPhotos(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range fs {
			v.enAlbum[a.ID] = append(v.enAlbum[a.ID], x.ID)
		}
	}
	if v.conocidas, err = f.HuellasConocidas(ctx); err != nil {
		t.Fatal(err)
	}
	if v.porOrigen, err = f.AlbumesPorOrigen(ctx); err != nil {
		t.Fatal(err)
	}
	v.recetas = map[string]string{}
	for _, x := range v.linea {
		r, err := f.LeerReceta(ctx, x.ID)
		if err != nil {
			t.Fatal(err)
		}
		v.recetas[x.ID] = r.Huella()
	}
	return v
}

func compararVisto(t *testing.T, antes, despues visto) {
	t.Helper()
	mismas := func(a, b []api.Photo) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			x, y := a[i], b[i]
			if x.ID != y.ID || x.Name != y.Name || !x.TakenAt.Equal(y.TakenAt) || x.TakenFromEXIF != y.TakenFromEXIF || x.Favorite != y.Favorite ||
				x.Description != y.Description || !slices.Equal(x.Tags, y.Tags) || x.Width != y.Width || x.Height != y.Height || x.ContentType != y.ContentType ||
				x.SizeBytes != y.SizeBytes || x.DurationMs != y.DurationMs || x.Externo != y.Externo || x.EstadoExterno != y.EstadoExterno || x.Camera != y.Camera ||
				(x.Location == nil) != (y.Location == nil) || x.Location != nil && *x.Location != *y.Location {
				t.Errorf("foto %d:\n antes   %+v\n después %+v", i, x, y)
				return false
			}
		}
		return true
	}
	if !mismas(antes.linea, despues.linea) {
		t.Error("la línea de tiempo no es la misma")
	}
	if !mismas(antes.papelera, despues.papelera) {
		t.Error("la papelera no es la misma")
	}
	if !slices.Equal(antes.albumes, despues.albumes) {
		t.Errorf("álbumes:\n antes   %+v\n después %+v", antes.albumes, despues.albumes)
	}
	for id, fs := range antes.enAlbum {
		if !slices.Equal(fs, despues.enAlbum[id]) {
			t.Errorf("el álbum %s: %v y %v", id, fs, despues.enAlbum[id])
		}
	}
	if len(antes.conocidas) != len(despues.conocidas) {
		t.Errorf("huellas conocidas: %d y %d", len(antes.conocidas), len(despues.conocidas))
	}
	for h := range antes.conocidas {
		if !despues.conocidas[h] {
			t.Errorf("se ha perdido la huella %s", h)
		}
	}
	if len(antes.porOrigen) != len(despues.porOrigen) {
		t.Errorf("por origen: %v y %v", antes.porOrigen, despues.porOrigen)
	}
	for k, a := range antes.porOrigen {
		if despues.porOrigen[k] != a {
			t.Errorf("origen %s: %v y %v", k, a, despues.porOrigen[k])
		}
	}
	for id, h := range antes.recetas {
		if despues.recetas[id] != h {
			t.Errorf("la receta de %s: %s y %s", id, h, despues.recetas[id])
		}
	}
}

// almacenDeAntes llena dir con una fototeca del almacén de registros (como la dejaba
// Photos de Sytue Office hasta ahora) con de todo, y devuelve lo que se veía y los
// originales de unas cuantas.
type deAntes struct {
	visto
	editada, enlazada, video, purgada string
	originales                        map[string][]byte
}

func almacenDeAntes(t *testing.T, dir string) deAntes {
	t.Helper()
	ctx := t.Context()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := photos.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	repo.AdmitirFormatos("webp", "avif", "bmp")
	repo.LiberarAlPurgar()
	if err := almacen.Preparar(ctx, repo); err != nil {
		t.Fatal(err)
	}
	f, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	d := deAntes{originales: map[string][]byte{}}
	importar := func(nombre string, datos []byte) api.Photo {
		t.Helper()
		x, err := f.ImportarFoto(ctx, uuid.NewString(), nombre, "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
		if err != nil {
			t.Fatal(err)
		}
		d.originales[x.ID] = datos
		return x
	}
	madrid := contrato.Lugar{Lat: 40.4168, Lon: -3.7038}
	fotos := make([]api.Photo, 0, 9)
	for i := range 9 {
		datos := contrato.JPEG(40, 30, uint8(i*20))
		if i%3 == 0 {
			datos = contrato.ConExif(datos, time.Date(2018+i, 6, 1+i, 10, 0, 0, 0, time.UTC), 6, &madrid)
		}
		fotos = append(fotos, importar("IMG_"+string(rune('A'+i))+".jpg", datos))
	}
	repetida := importar("repetida.jpg", d.originales[fotos[1].ID])
	if err := f.SetFavorite(ctx, fotos[2].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := f.SetDescription(ctx, fotos[3].ID, "Una playa", []string{"mar"}); err != nil {
		t.Fatal(err)
	}
	cuando := time.Date(2015, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := repo.PonerFechas(ctx, fotos[4].ID, &cuando, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Rotar(ctx, fotos[5].ID); err != nil {
		t.Fatal(err)
	}
	d.editada = fotos[5].ID
	fuera := t.TempDir()
	ruta := filepath.Join(fuera, "enlazada.jpg")
	if err := os.WriteFile(ruta, contrato.JPEG(50, 20, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	enlazada, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: ruta})
	if err != nil {
		t.Fatal(err)
	}
	d.enlazada = enlazada.ID
	video := bytes.Repeat([]byte("vídeo "), 3000)
	v, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{Nombre: "VID.mp4", Tipo: "video/mp4", Datos: bytes.NewReader(video), Tamano: int64(len(video)), Duracion: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	d.video, d.originales[v.ID] = v.ID, video
	viaje, err := f.CreateAlbum(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.AddToAlbum(ctx, viaje.ID, []string{fotos[6].ID, fotos[0].ID, enlazada.ID, fotos[7].ID, v.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.PonerPortada(ctx, viaje.ID, "Viaje", fotos[7].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.RemoveFromAlbum(ctx, viaje.ID, fotos[0].ID); err != nil {
		t.Fatal(err)
	}
	traido, err := repo.CrearAlbumDe(ctx, "Traído", "sytue:1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.AddToAlbum(ctx, traido.ID, []string{fotos[8].ID, repetida.ID}); err != nil {
		t.Fatal(err)
	}
	borrado, err := repo.CrearAlbumDe(ctx, "Borrado", "sytue:2")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteAlbum(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}
	// La papelera: una que está en un álbum (vuelve con ella) y una purgada.
	for _, id := range []string{fotos[7].ID, fotos[8].ID, repetida.ID} {
		if err := f.Trash(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.Purgar(ctx, repetida.ID); err != nil {
		t.Fatal(err)
	}
	d.purgada = repetida.ID
	d.visto = ver(t, repo)
	repo.Close()
	rec.Cerrar()
	return d
}

// La fototeca del almacén de registros pasa a SQLite al abrirla: se ve igual (la línea,
// la papelera, los álbumes con su orden y su portada, las recetas, las huellas conocidas
// y los orígenes), los originales y las derivadas se leen, el diario de antes se
// conserva con otro nombre, no se migra dos veces y lo de después funciona.
func TestLaFototecaDeAntesPasaASQLite(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	antes := almacenDeAntes(t, dir)
	if !fototeca.HayQueMigrar(dir) {
		t.Fatal("hay que migrar")
	}
	var fases []string
	f, err := fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{Formatos: []string{"webp"}, Avance: func(a fototeca.Avance) {
		if len(fases) == 0 || fases[len(fases)-1] != a.Fase {
			fases = append(fases, a.Fase)
		}
	}})
	if err != nil {
		t.Fatal("migrar:", err)
	}
	if !slices.Equal(fases, []string{fototeca.FaseAbrir, fototeca.FaseCopiar, fototeca.FaseComprobar}) {
		t.Error("fases:", fases)
	}
	compararVisto(t, antes.visto, ver(t, f))
	for id, datos := range antes.originales {
		if id == antes.purgada {
			continue
		}
		var b bytes.Buffer
		if err := f.Original(ctx, id, &b); err != nil || !bytes.Equal(b.Bytes(), datos) {
			t.Error("el original de", id, err)
		}
	}
	for _, x := range append(slices.Clone(antes.linea), antes.papelera...) {
		r, err := f.VariantDe(ctx, x.ID, api.PhotoThumb, "")
		if err != nil {
			t.Fatal("la miniatura de", x.Name, err)
		}
		_ = r.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); !os.IsNotExist(err) {
		t.Error("el diario de antes sigue con su nombre:", err)
	}
	for _, conservado := range []string{fototeca.DiarioConservado, "diario", "origen.json"} {
		if _, err := os.Stat(filepath.Join(dir, conservado)); err != nil {
			t.Error("no se ha conservado", conservado, err)
		}
	}
	if fototeca.HayQueMigrar(dir) {
		t.Error("ya no hay que migrar")
	}
	// Lo de después: purgar una migrada suelta su original del disco.
	if err := f.Trash(ctx, antes.video); err != nil {
		t.Fatal(err)
	}
	if n, err := f.Purgar(ctx, ""); err != nil || n != 3 {
		t.Fatal("vaciar la papelera:", n, err)
	}
	if contrato.HayFicheroIgual(filepath.Join(dir, "objetos"), antes.originales[antes.video]) {
		t.Error("el vídeo purgado sigue en el disco")
	}
	f.Close()

	// Reabrir no migra otra vez.
	fases = nil
	f, err = fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{Avance: func(a fototeca.Avance) { fases = append(fases, a.Fase) }})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(fases) != 0 {
		t.Error("ha migrado dos veces:", fases)
	}
	if l, _, err := f.Timeline(ctx, "", 100, false, ""); err != nil || len(l) != len(antes.linea)-1 {
		t.Error("tras reabrir:", len(l), err)
	}
}

// Un corte o un fallo a mitad no deja nada a medias: si falta un fichero que se cita, no
// se migra, el almacén sigue como estaba y se vuelve a intentar al abrir; si el corte
// llega tras confirmar, la próxima apertura solo aparta el diario.
func TestLaMigracionSeRepiteEnteraTrasUnFallo(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	antes := almacenDeAntes(t, dir)
	// Quitar el original de una foto: el almacén ya no abre (lo valida) y no se migra.
	huella := contrato.Huella(antes.originales[antes.editada])
	objeto := filepath.Join(dir, "objetos", huella[:2], huella)
	guardado, err := os.ReadFile(objeto)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(objeto); err != nil {
		t.Fatal(err)
	}
	if _, err := fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{}); err == nil {
		t.Fatal("ha migrado sin un original")
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); err != nil {
		t.Fatal("el diario de antes se ha tocado:", err)
	}
	if !fototeca.HayQueMigrar(dir) {
		t.Fatal("sigue habiendo que migrar")
	}
	if l, err := fototeca.LeerSinAbrir(dir); err != nil || l.Migrada || len(l.Fotos) != 0 {
		t.Fatal("se ha quedado algo a medias:", l, err)
	}
	if err := os.WriteFile(objeto, guardado, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{})
	if err != nil {
		t.Fatal("la segunda vez:", err)
	}
	compararVisto(t, antes.visto, ver(t, f))
	f.Close()

	// Un corte entre confirmar y apartar el diario: se aparta al abrir, sin migrar otra
	// vez (ni duplicar nada).
	if err := os.Rename(filepath.Join(dir, fototeca.DiarioConservado), filepath.Join(dir, "registros.jsonl")); err != nil {
		t.Fatal(err)
	}
	var fases []string
	f, err = fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{Avance: func(a fototeca.Avance) { fases = append(fases, a.Fase) }})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(fases) != 0 {
		t.Error("ha migrado otra vez:", fases)
	}
	if _, err := os.Stat(filepath.Join(dir, "registros.jsonl")); !os.IsNotExist(err) {
		t.Error("no ha apartado el diario:", err)
	}
	compararVisto(t, antes.visto, ver(t, f))
}

// Con la fototeca de antes abierta en otra app (el almacén con su cerrojo), no se migra
// ni se toca nada: ErrEnUso. Y si dos abren a la vez, migra uno y el otro la encuentra
// hecha.
func TestNoSeMigraConElAlmacenAbierto(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	antes := almacenDeAntes(t, dir)
	rec, err := almacen.Abrir(context.Background(), dir, "photos", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{}); !errors.Is(err, fototeca.ErrEnUso) {
		t.Fatal("con el almacén abierto:", err)
	}
	rec.Cerrar()
	if !fototeca.HayQueMigrar(dir) {
		t.Fatal("sigue habiendo que migrar")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for range 2 {
		wg.Go(func() {
			f, err := fototeca.Abrir(ctx, dir, "instalacion-1", fototeca.Opciones{})
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			if err == nil {
				f.Close()
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal("dos a la vez:", err)
		}
	}
	f := abrirEn(t, dir)
	defer f.Close()
	compararVisto(t, antes.visto, ver(t, f))
	// La miniatura de la enlazada y la del vídeo, y la receta, siguen.
	for _, id := range []string{antes.enlazada, antes.video, antes.editada} {
		r, err := f.VariantDe(ctx, id, api.PhotoPreview, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()
	}
}

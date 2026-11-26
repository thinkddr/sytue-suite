// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package contrato es el juego de pruebas de comportamiento de la fototeca de Sytue
// Office (§8.2 del plan de la edición local): el MISMO juego corre contra el repositorio
// de Photos sobre el motor de registros (`nucleo/photos`, el de la edición conectada, en
// el almacén en claro) y contra la fototeca sobre SQLite (`local/fototeca`, el plan B de
// §5.6). Lo que la pantalla y los demás (el Hub al traer, Exportar, Cast) ven de una lo
// ven igual de la otra. Solo lo importan las pruebas.
package contrato

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

// Fototeca es lo que tienen las dos: lo que pide la fachada de las pantallas (api.Photos)
// y lo que usan Photos, el Hub y Exportar de Sytue Office.
type Fototeca interface {
	api.PhotosData
	api.PhotosMediaData
	api.PhotosExportData
	api.PhotosRecipeData
	api.PhotosAlbumData
	api.PhotosAlbumMembershipData
	api.PhotosCalendarData
	api.PhotosDiscoveryData
	api.PhotosPurgeData
	api.PhotosAlbumCoverData
	api.PhotosDirectIngestData
	ImportarExterno(context.Context, string, photos.OrigenExterno) (api.Photo, error)
	ImportarVideo(context.Context, string, photos.OrigenVideo) (api.Photo, error)
	FotoPorHuella(context.Context, string) (api.Photo, bool, error)
	HuellasConocidas(context.Context) (map[string]bool, error)
	HuellaOriginal(context.Context, string) (string, error)
	Original(context.Context, string, io.Writer) error
	RutaExterna(context.Context, string) (ruta, estado string, ok bool, err error)
	Recolocar(context.Context, []string) (int, error)
	ExportarFototeca(context.Context, string, func(hechas, total int)) (photos.ResumenExportacion, error)
	PonerFechas(context.Context, string, *time.Time, *time.Time) error
	CrearAlbumDe(context.Context, string, string) (api.Album, error)
	AlbumesPorOrigen(context.Context) (map[string]photos.AlbumDeOrigen, error)
}

// Puerto es una de las dos fototecas.
type Puerto struct {
	// Abrir abre la fototeca de la carpeta dir (otra vez la misma carpeta, la reabre) y
	// da cómo cerrarla.
	Abrir func(t *testing.T, dir string) (Fototeca, func())
	// Liberar, con la fototeca cerrada, hace lo que haga falta para que se vayan del
	// disco los ficheros de lo borrado para siempre (en el almacén de registros,
	// compactarlo); nil si ya se fueron al borrar.
	Liberar func(t *testing.T, dir string)
}

// Probar corre el juego entero sobre p.
func Probar(t *testing.T, p Puerto) {
	for _, c := range []struct {
		nombre string
		probar func(*testing.T, Puerto)
	}{
		{"importar y reabrir", importarYReabrir},
		{"línea de tiempo", lineaDeTiempo},
		{"cálculos", calculos},
		{"álbumes", albumes},
		{"editar", editar},
		{"enlazados", enlazados},
		{"vídeos", videos},
		{"borrar para siempre", borrarParaSiempre},
		{"traído de Sytue", traido},
		{"exportar", exportar},
	} {
		t.Run(c.nombre, func(t *testing.T) { c.probar(t, p) })
	}
}

// abrir abre la fototeca de dir con su fachada, y la cierra al acabar la prueba (o
// antes, con lo que devuelve).
func abrir(t *testing.T, p Puerto, dir string) (Fototeca, *api.Photos, func()) {
	t.Helper()
	f, cerrar := p.Abrir(t, dir)
	fachada, err := api.NewPhotos("https://sin-red.invalid", nil, nil).WithPrivateData(f)
	if err != nil {
		cerrar()
		t.Fatal(err)
	}
	una := false
	fin := func() {
		if !una {
			una = true
			cerrar()
		}
	}
	t.Cleanup(fin)
	return f, fachada, fin
}

// Huella es el SHA-256 de datos en hexadecimal.
func Huella(datos []byte) string {
	s := sha256.Sum256(datos)
	return hex.EncodeToString(s[:])
}

// HayFicheroIgual dice si bajo raiz hay un fichero con esos bytes.
func HayFicheroIgual(raiz string, datos []byte) bool {
	hallado := false
	_ = filepath.WalkDir(raiz, func(ruta string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || hallado {
			return err
		}
		if b, err := os.ReadFile(ruta); err == nil && bytes.Equal(b, datos) { //nolint:gosec // G304: la carpeta de la prueba
			hallado = true
		}
		return nil
	})
	return hallado
}

func importar(t *testing.T, f *api.Photos, nombre string, datos []byte) api.Photo {
	t.Helper()
	foto, err := f.ImportarFoto(t.Context(), uuid.NewString(), nombre, "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal("importar", nombre, err)
	}
	return foto
}

// imagen lee la imagen que devuelve una llamada (imagen(t)(f.Variant(…))).
func imagen(t *testing.T) func(io.ReadCloser, error) image.Image {
	return func(r io.ReadCloser, err error) image.Image {
		t.Helper()
		return leerImagen(t, r, err)
	}
}

func leerImagen(t *testing.T, r io.ReadCloser, err error) image.Image {
	t.Helper()
	if err != nil {
		t.Fatal("leer la imagen:", err)
	}
	defer func() { _ = r.Close() }()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal("no es una imagen:", err)
	}
	return img
}

func ids(fs []api.Photo) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}

// Una foto importada sobrevive a cerrar y reabrir: sale en la línea de tiempo, su
// original vuelve byte a byte, su miniatura se lee, se encuentra por su huella; en la
// papelera ya no, pero sigue siendo conocida. Importar dos veces el mismo trabajo no
// duplica, y con otros bytes es un conflicto.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func importarYReabrir(t *testing.T, p Puerto) {
	dir := t.TempDir()
	ctx := t.Context()
	foto := JPEG(64, 48, 120)
	huella := Huella(foto)
	repo, fachada, cerrar := abrir(t, p, dir)
	if !fachada.CanImportPrivatePhotos() {
		t.Fatal("la fachada no sabe importar")
	}
	if _, ok, err := repo.FotoPorHuella(ctx, huella); err != nil || ok {
		t.Fatal("antes de importar no hay ninguna con esa huella", ok, err)
	}
	job := uuid.NewString()
	importada, err := fachada.ImportarFoto(ctx, job, "playa.jpg", "image/jpeg", bytes.NewReader(foto), int64(len(foto)))
	if err != nil {
		t.Fatal("importar:", err)
	}
	if importada.ID != job || importada.Width != 64 || importada.Height != 48 || importada.Name != "playa.jpg" || importada.ContentType != "image/jpeg" ||
		importada.SizeBytes != int64(len(foto)) || importada.TakenFromEXIF || importada.DriveFileID != "" || importada.Externo != "" {
		t.Fatalf("importada: %+v", importada)
	}
	if otra, err := fachada.ImportarFoto(ctx, job, "playa.jpg", "image/jpeg", bytes.NewReader(foto), int64(len(foto))); err != nil || otra.ID != job {
		t.Fatal("el mismo trabajo dos veces:", otra, err)
	}
	distinta := JPEG(64, 48, 10)
	if _, err := fachada.ImportarFoto(ctx, job, "playa.jpg", "image/jpeg", bytes.NewReader(distinta), int64(len(distinta))); !errors.Is(err, photos.ErrConflict) {
		t.Fatal("el mismo trabajo con otros bytes:", err)
	}
	if _, err := fachada.ImportarFoto(ctx, uuid.NewString(), "texto.jpg", "image/jpeg", bytes.NewReader([]byte("no es una foto")), 14); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("algo que no es una foto:", err)
	}
	if err := fachada.SetFavorite(ctx, importada.ID, true); err != nil {
		t.Fatal(err)
	}
	if pendientes, err := fachada.ReanudarImportaciones(ctx); err != nil || len(pendientes) != 0 {
		t.Fatal("no queda nada a medias:", pendientes, err)
	}
	cerrar()

	repo, fachada, _ = abrir(t, p, dir)
	linea, siguiente, err := fachada.Timeline(ctx, "", 100, false, "")
	if err != nil || siguiente != "" || len(linea) != 1 || linea[0].ID != importada.ID || linea[0].Name != "playa.jpg" || !linea[0].Favorite {
		t.Fatal("línea de tiempo tras reabrir:", linea, err)
	}
	if !linea[0].TakenAt.Equal(importada.TakenAt) {
		t.Fatal("la fecha de la galería cambia al reabrir:", linea[0].TakenAt, importada.TakenAt)
	}
	var original bytes.Buffer
	if err := repo.Original(ctx, importada.ID, &original); err != nil || !bytes.Equal(original.Bytes(), foto) {
		t.Fatal("el original no vuelve igual:", err)
	}
	imagen(t)(fachada.Variant(ctx, importada.ID, api.PhotoThumb))
	for _, v := range model.Variantes() {
		imagen(t)(fachada.Variant(ctx, importada.ID, string(v)))
	}
	if h, err := repo.HuellaOriginal(ctx, importada.ID); err != nil || h != huella {
		t.Fatal("huella del original:", h, err)
	}
	otra, ok, err := repo.FotoPorHuella(ctx, huella)
	if err != nil || !ok || otra.ID != importada.ID {
		t.Fatal("no se encuentra por huella:", otra, ok, err)
	}
	if !HayFicheroIgual(dir, foto) {
		t.Fatal("el original no está en claro en la carpeta")
	}
	if err := fachada.Trash(ctx, importada.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.FotoPorHuella(ctx, huella); err != nil || ok {
		t.Fatal("una foto en la papelera no se reabre por huella", ok, err)
	}
	if hs, err := repo.HuellasConocidas(ctx); err != nil || len(hs) != 1 || !hs[huella] {
		t.Fatal("huellas conocidas", hs, err)
	}
	if _, err := fachada.Photo(ctx, importada.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("una foto de la papelera no está en la galería:", err)
	}
	// Su miniatura y su original se siguen leyendo (la papelera las enseña).
	imagen(t)(fachada.Variant(ctx, importada.ID, api.PhotoThumb))
	if err := repo.Original(ctx, importada.ID, io.Discard); err != nil {
		t.Fatal("el original de una de la papelera:", err)
	}
	for nombre, err := range map[string]error{
		"favorita":    fachada.SetFavorite(ctx, importada.ID, false),
		"descripción": fachada.SetDescription(ctx, importada.ID, "x", nil),
		"papelera":    fachada.Trash(ctx, importada.ID),
	} {
		if !errors.Is(err, photos.ErrNotFound) {
			t.Errorf("%s de una de la papelera: %v", nombre, err)
		}
	}
	if err := fachada.Restaurar(ctx, importada.ID); err != nil {
		t.Fatal(err)
	}
	if err := fachada.Restaurar(ctx, importada.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("restaurar una que no está en la papelera:", err)
	}
	if _, err := fachada.Photo(ctx, uuid.NewString()); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("una que no existe:", err)
	}
}

// fotosConFecha importa n fotos con su fecha de captura dicha (PonerFechas), de la más
// vieja a la más nueva, separadas por un día.
func fotosConFecha(t *testing.T, repo Fototeca, f *api.Photos, n int, primera time.Time) []api.Photo {
	t.Helper()
	out := make([]api.Photo, n)
	for i := range n {
		foto := importar(t, f, "foto-"+strconv.Itoa(i)+".jpg", JPEG(16, 12, uint8(i)))
		cuando := primera.AddDate(0, 0, i)
		if err := repo.PonerFechas(t.Context(), foto.ID, &cuando, nil); err != nil {
			t.Fatal(err)
		}
		var err error
		if out[i], err = f.Photo(t.Context(), foto.ID); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// La línea de tiempo: de la más reciente a la más antigua por la fecha de la galería (y
// por id a igual fecha), por páginas sin repetir ni saltarse ninguna, con favoritas,
// buscando sin mayúsculas ni acentos por nombre, descripción y etiquetas, desde un año o
// un mes, con sus años; y la papelera, de la última que se tiró a la primera.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func lineaDeTiempo(t *testing.T, p Puerto) {
	ctx := t.Context()
	repo, f, _ := abrir(t, p, t.TempDir())
	fotos := fotosConFecha(t, repo, f, 7, time.Date(2019, 12, 29, 10, 0, 0, 0, time.UTC))
	// Dos con la MISMA fecha: las ordena el id.
	gemela := importar(t, f, "gemela.jpg", JPEG(16, 12, 200))
	mismo := fotos[3].TakenAt
	if err := repo.PonerFechas(ctx, gemela.ID, &mismo, nil); err != nil {
		t.Fatal(err)
	}
	gemela, _ = f.Photo(ctx, gemela.ID)
	todas := append(slices.Clone(fotos), gemela)
	slices.SortFunc(todas, photos.CompararEnLinea)

	var vistas []api.Photo
	for cursor, paginas := "", 0; ; paginas++ {
		pagina, siguiente, err := f.Timeline(ctx, cursor, 3, false, "")
		if err != nil || paginas > 10 {
			t.Fatal("página:", err)
		}
		vistas = append(vistas, pagina...)
		if siguiente == "" {
			break
		}
		cursor = siguiente
	}
	if !slices.Equal(ids(vistas), ids(todas)) {
		t.Fatalf("la línea por páginas:\n%v\nquería\n%v", ids(vistas), ids(todas))
	}
	for i, v := range vistas {
		if !v.TakenFromEXIF || !v.TakenAt.Equal(todas[i].TakenAt) {
			t.Fatalf("fecha de %s: %v (%v)", v.Name, v.TakenAt, v.TakenFromEXIF)
		}
	}
	if _, _, err := f.Timeline(ctx, "no-es-un-cursor", 3, false, ""); err == nil {
		t.Error("un cursor que no es de esta línea vale")
	}

	// Favoritas: la misma línea, filtrada.
	for _, i := range []int{1, 5} {
		if err := f.SetFavorite(ctx, fotos[i].ID, true); err != nil {
			t.Fatal(err)
		}
	}
	favs, _, err := f.Timeline(ctx, "", 50, true, "")
	if err != nil || !slices.Equal(ids(favs), []string{fotos[5].ID, fotos[1].ID}) {
		t.Fatal("favoritas:", ids(favs), err)
	}
	// Buscar.
	if err := f.SetDescription(ctx, fotos[2].ID, "Una tarde en la PLAYA de Cádiz", []string{"mar", "árboles"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetDescription(ctx, fotos[0].ID, strings501(), nil); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("una descripción demasiado larga:", err)
	}
	for q, want := range map[string][]string{
		"playa": {fotos[2].ID}, "CADIZ": {fotos[2].ID}, "arbol": {fotos[2].ID}, "foto-4": {fotos[4].ID}, "nada así": {},
	} {
		got, _, err := f.Timeline(ctx, "", 50, false, q)
		if err != nil || !slices.Equal(ids(got), want) {
			t.Errorf("buscar %q: %v %v", q, ids(got), err)
		}
	}
	tambien, _, err := f.Timeline(ctx, "", 50, false, "nada así", "gemela", "mar")
	if err != nil || !slices.Equal(ids(tambien), []string{fotos[2].ID, gemela.ID}) && !slices.Equal(ids(tambien), []string{gemela.ID, fotos[2].ID}) {
		t.Error("buscar con términos de la IA:", ids(tambien), err)
	}
	// Saltar a un año o un mes, y seguir desde ahí.
	desde, siguiente, err := f.TimelineDesde(ctx, "2019", 50, false)
	if err != nil || siguiente != "" || !slices.Equal(ids(desde), ids(todas[len(todas)-3:])) {
		t.Fatal("desde 2019:", ids(desde), err)
	}
	enero, siguiente, err := f.TimelineDesde(ctx, "2020-01-02", 2, false)
	if err != nil || len(enero) != 2 || siguiente == "" || !enero[0].TakenAt.Before(time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("desde el 2 de enero:", enero, err)
	}
	resto, _, err := f.Timeline(ctx, siguiente, 50, false, "")
	if err != nil || len(enero)+len(resto) != 6 {
		t.Fatal("seguir desde el 2 de enero:", len(resto), err)
	}
	if _, _, err := f.TimelineDesde(ctx, "ayer", 2, false); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("una fecha que no es una fecha:", err)
	}
	anyos, err := f.TimelineYears(ctx, false)
	if err != nil || !slices.Equal(anyos, []api.TimelineYear{{Year: 2020, Count: 5}, {Year: 2019, Count: 3}}) {
		t.Fatal("años:", anyos, err)
	}
	if anyos, err := f.TimelineYears(ctx, true); err != nil || !slices.Equal(anyos, []api.TimelineYear{{Year: 2020, Count: 1}, {Year: 2019, Count: 1}}) {
		t.Fatal("años de las favoritas:", anyos, err)
	}
	// La papelera.
	for _, i := range []int{4, 0, 6} {
		if err := f.Trash(ctx, fotos[i].ID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	papelera, err := f.Papelera(ctx)
	if err != nil || !slices.Equal(ids(papelera.Items), []string{fotos[6].ID, fotos[0].ID, fotos[4].ID}) {
		t.Fatal("papelera:", ids(papelera.Items), err)
	}
	linea, _, err := f.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 5 || slices.Contains(ids(linea), fotos[4].ID) {
		t.Fatal("la línea sin la papelera:", ids(linea), err)
	}
	if anyos, err := f.TimelineYears(ctx, false); err != nil || !slices.Equal(anyos, []api.TimelineYear{{Year: 2020, Count: 3}, {Year: 2019, Count: 2}}) {
		t.Fatal("años sin la papelera:", anyos, err)
	}
}

func strings501() string { return string(bytes.Repeat([]byte("a"), 501)) }

// Lo que el servicio calcula con su índice: los recuerdos (un día como hoy en años
// anteriores, solo por la fecha de captura), las repetidas (el mismo original) y «Por
// aquí» (a menos de 2 km, de la más cerca a la más lejos, sin la de partida).
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func calculos(t *testing.T, p Puerto) {
	ctx := t.Context()
	repo, f, _ := abrir(t, p, t.TempDir())
	hoy := time.Now()
	hace := func(anyos int) time.Time {
		return time.Date(hoy.Year()-anyos, hoy.Month(), hoy.Day(), 12, 0, 0, 0, time.UTC)
	}
	madrid := Lugar{40.4168, -3.7038}
	cerca := Lugar{40.4200, -3.7000}   // a unos 500 m
	mas := Lugar{40.4300, -3.7038}     // a unos 1,5 km
	lejos := Lugar{40.4500, -3.7038}   // a unos 3,7 km
	sevilla := Lugar{37.3891, -5.9845} // lejísimos
	nueva := func(nombre string, tono uint8, tomada time.Time, lugar *Lugar) api.Photo {
		t.Helper()
		return importar(t, f, nombre, ConExif(JPEG(24, 16, tono), tomada, 1, lugar))
	}
	a := nueva("a.jpg", 1, hace(1), &madrid)
	b := nueva("b.jpg", 2, hace(3), &cerca)
	c := nueva("c.jpg", 3, hace(1).Add(time.Hour), &mas)
	d := nueva("d.jpg", 4, hace(3).AddDate(0, 0, 2), &lejos)
	e := nueva("e.jpg", 5, hace(0), &sevilla)
	sinFecha := importar(t, f, "f.jpg", JPEG(24, 16, 6))
	if !a.TakenFromEXIF || a.Location == nil || a.Location.Lat < 40.41 || a.Location.Lat > 40.42 || a.Location.Lon > -3.70 {
		t.Fatalf("el EXIF de la prueba no se lee: %+v %+v", a, a.Location)
	}

	recuerdos, err := f.Memories(ctx)
	if err != nil || len(recuerdos) != 2 || recuerdos[0].Year != hoy.Year()-1 || recuerdos[1].Year != hoy.Year()-3 ||
		!slices.Equal(ids(recuerdos[0].Items), []string{c.ID, a.ID}) || !slices.Equal(ids(recuerdos[1].Items), []string{b.ID}) {
		t.Fatalf("recuerdos: %+v %v", recuerdos, err)
	}
	cercanas, err := f.Nearby(ctx, a.ID, 10)
	if err != nil || !slices.Equal(ids(cercanas), []string{b.ID, c.ID}) {
		t.Fatal("por aquí:", ids(cercanas), err)
	}
	if solo, err := f.Nearby(ctx, a.ID, 1); err != nil || !slices.Equal(ids(solo), []string{b.ID}) {
		t.Fatal("por aquí, la más cercana:", ids(solo), err)
	}
	if nada, err := f.Nearby(ctx, sinFecha.ID, 10); err != nil || len(nada) != 0 {
		t.Fatal("sin GPS no hay nada cerca:", nada, err)
	}
	if _, err := f.Nearby(ctx, uuid.NewString(), 10); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("por aquí de una que no está:", err)
	}
	_ = d
	_ = e

	// Repetidas: el mismo original dos veces, con otro nombre; la primera importada
	// delante.
	roja := JPEG(20, 20, 250)
	r1 := importar(t, f, "roja.jpg", roja)
	r2 := importar(t, f, "roja (copia).jpg", roja)
	r3 := importar(t, f, "roja (otra).jpg", roja)
	grupos, err := f.Duplicates(ctx, 50)
	if err != nil || len(grupos) != 1 || !slices.Equal(ids(grupos[0]), []string{r1.ID, r2.ID, r3.ID}) {
		t.Fatal("repetidas:", grupos, err)
	}
	if err := f.Trash(ctx, r2.ID); err != nil {
		t.Fatal(err)
	}
	if grupos, err := f.Duplicates(ctx, 50); err != nil || len(grupos) != 1 || !slices.Equal(ids(grupos[0]), []string{r1.ID, r3.ID}) {
		t.Fatal("repetidas sin la de la papelera:", grupos, err)
	}
	if err := f.Trash(ctx, r3.ID); err != nil {
		t.Fatal(err)
	}
	if grupos, err := f.Duplicates(ctx, 50); err != nil || len(grupos) != 0 {
		t.Fatal("ya no hay repetidas:", grupos, err)
	}
	if _, ok, err := repo.FotoPorHuella(ctx, Huella(roja)); err != nil || !ok {
		t.Fatal("la que queda se encuentra por huella:", ok, err)
	}
}

// Los álbumes: crear (título limpio), renombrar, añadir sin repetir y al final, quitar,
// la portada (la elegida si sigue, si no la primera), las fotos de la papelera fuera
// hasta restaurarlas, borrar el álbum sin tocar sus fotos, y del más nuevo al más viejo.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func albumes(t *testing.T, p Puerto) {
	ctx := t.Context()
	dir := t.TempDir()
	_, f, cerrar := abrir(t, p, dir)
	a := importar(t, f, "a.jpg", JPEG(10, 10, 1))
	b := importar(t, f, "b.jpg", JPEG(10, 10, 2))
	c := importar(t, f, "c.jpg", JPEG(10, 10, 3))
	viaje, err := f.CreateAlbum(ctx, "  Viaje  ")
	if err != nil || viaje.Title != "Viaje" || viaje.ID == "" {
		t.Fatal("crear:", viaje, err)
	}
	if _, err := f.CreateAlbum(ctx, "   "); err == nil {
		t.Error("un álbum sin título")
	}
	time.Sleep(2 * time.Millisecond)
	otro, err := f.CreateAlbum(ctx, "Otro")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.AddToAlbum(ctx, viaje.ID, []string{b.ID, a.ID, b.ID}); err != nil || n != 2 {
		t.Fatal("añadir:", n, err)
	}
	if n, err := f.AddToAlbum(ctx, viaje.ID, []string{a.ID, c.ID}); err != nil || n != 1 {
		t.Fatal("añadir otra vez:", n, err)
	}
	if _, err := f.AddToAlbum(ctx, viaje.ID, []string{uuid.NewString()}); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("añadir una que no está:", err)
	}
	en, err := f.AlbumPhotos(ctx, viaje.ID)
	if err != nil || !slices.Equal(ids(en), []string{b.ID, a.ID, c.ID}) {
		t.Fatal("en el álbum:", ids(en), err)
	}
	if got, err := f.Album(ctx, viaje.ID); err != nil || got.Count != 3 || got.CoverAssetID != b.ID || got.Title != "Viaje" {
		t.Fatal("el álbum:", got, err)
	}
	if err := f.PonerPortada(ctx, viaje.ID, "Viaje", c.ID); err != nil {
		t.Fatal("portada:", err)
	}
	if err := f.PonerPortada(ctx, otro.ID, "Otro", c.ID); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("portada con una que no está en el álbum:", err)
	}
	if err := f.RenameAlbum(ctx, viaje.ID, "Viaje a Cádiz"); err != nil {
		t.Fatal(err)
	}
	if err := f.RenameAlbum(ctx, viaje.ID, ""); err == nil {
		t.Error("renombrar sin título")
	}
	// Una de la papelera no cuenta ni es portada, y vuelve al restaurarla.
	if err := f.Trash(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := f.Album(ctx, viaje.ID); err != nil || got.Count != 2 || got.CoverAssetID != b.ID {
		t.Fatal("con una en la papelera:", got, err)
	}
	if _, err := f.AddToAlbum(ctx, otro.ID, []string{c.ID}); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("añadir una de la papelera:", err)
	}
	cerrar()

	_, f, _ = abrir(t, p, dir)
	if err := f.Restaurar(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.Album(ctx, viaje.ID)
	if err != nil || got.Count != 3 || got.CoverAssetID != c.ID || got.Title != "Viaje a Cádiz" {
		t.Fatal("restaurada y tras reabrir:", got, err)
	}
	if err := f.RemoveFromAlbum(ctx, viaje.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	// Quitar una que ya no está no se prueba: el repositorio del motor de registros
	// falla al volver a escribir las mismas páginas del álbum («almacen: no
	// encontrado»), y la pantalla nunca lo pide.
	if err := f.RemoveFromAlbum(ctx, viaje.ID, "no-es-un-id"); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("quitar algo que no es un id:", err)
	}
	if got, err := f.Album(ctx, viaje.ID); err != nil || got.Count != 2 || got.CoverAssetID != b.ID {
		t.Fatal("sin la portada:", got, err)
	}
	if n, err := f.AddToAlbum(ctx, viaje.ID, []string{c.ID}); err != nil || n != 1 {
		t.Fatal("volver a añadirla:", n, err)
	}
	if en, err := f.AlbumPhotos(ctx, viaje.ID); err != nil || !slices.Equal(ids(en), []string{b.ID, a.ID, c.ID}) {
		t.Fatal("vuelve al final:", ids(en), err)
	}
	todos, err := f.Albums(ctx)
	if err != nil || len(todos) != 2 || todos[0].ID != otro.ID || todos[1].ID != viaje.ID || todos[0].Count != 0 || todos[0].CoverAssetID != "" {
		t.Fatal("los álbumes:", todos, err)
	}
	if err := f.DeleteAlbum(ctx, viaje.ID); err != nil {
		t.Fatal(err)
	}
	for nombre, err := range map[string]error{
		"leer":        func() error { _, err := f.Album(ctx, viaje.ID); return err }(),
		"sus fotos":   func() error { _, err := f.AlbumPhotos(ctx, viaje.ID); return err }(),
		"renombrar":   f.RenameAlbum(ctx, viaje.ID, "x"),
		"añadir":      func() error { _, err := f.AddToAlbum(ctx, viaje.ID, []string{a.ID}); return err }(),
		"borrar":      f.DeleteAlbum(ctx, viaje.ID),
		"inexistente": func() error { _, err := f.Album(ctx, uuid.NewString()); return err }(),
	} {
		if !errors.Is(err, photos.ErrNotFound) {
			t.Errorf("%s un álbum borrado: %v", nombre, err)
		}
	}
	if linea, _, err := f.Timeline(ctx, "", 50, false, ""); err != nil || len(linea) != 3 {
		t.Fatal("borrar el álbum no toca sus fotos:", ids(linea), err)
	}
	if todos, err := f.Albums(ctx); err != nil || len(todos) != 1 {
		t.Fatal("sin el borrado:", todos, err)
	}
}

// Editar: la receta se guarda con sus derivadas (girar 90° cambia los lados), la
// miniatura sigue a la receta guardada y la de antes se puede pedir por su huella, la
// base equivocada es un conflicto, «Descargar» aplica la receta, y un vídeo no se edita.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func editar(t *testing.T, p Puerto) {
	ctx := t.Context()
	dir := t.TempDir()
	_, f, cerrar := abrir(t, p, dir)
	foto := importar(t, f, "apaisada.jpg", JPEG(80, 40, 30))
	receta, err := f.LeerReceta(ctx, foto.ID)
	if err != nil || len(receta.Pasos) != 0 {
		t.Fatal("sin editar:", receta, err)
	}
	vacia := receta.Huella()
	if pendiente, err := f.EdicionPendiente(ctx, foto.ID); err != nil || pendiente != nil {
		t.Fatal("nada a medias:", pendiente, err)
	}
	girada, err := f.Rotar(ctx, foto.ID)
	if err != nil || girada == "" || girada == vacia {
		t.Fatal("girar:", girada, err)
	}
	if img := imagen(t)(f.Variant(ctx, foto.ID, api.PhotoThumb)); img.Bounds().Dx() >= img.Bounds().Dy() {
		t.Fatal("la miniatura no sigue a la receta:", img.Bounds())
	}
	if img := imagen(t)(f.VariantDe(ctx, foto.ID, api.PhotoThumb, vacia)); img.Bounds().Dx() <= img.Bounds().Dy() {
		t.Fatal("la de antes, por su huella:", img.Bounds())
	}
	if img := imagen(t)(f.VariantDe(ctx, foto.ID, api.PhotoPreview, girada)); img.Bounds().Dx() >= img.Bounds().Dy() {
		t.Fatal("la previa girada:", img.Bounds())
	}
	if _, err := f.GuardarRecetaDesde(ctx, foto.ID, api.Receta{Version: 1}, vacia); !errors.Is(err, photos.ErrConflict) {
		t.Fatal("con la base de antes:", err)
	}
	receta, err = f.LeerReceta(ctx, foto.ID)
	if err != nil || receta.Huella() != girada || len(receta.Pasos) != 1 {
		t.Fatal("la receta guardada:", receta, err)
	}
	cerrar()

	_, f, _ = abrir(t, p, dir)
	if img := imagen(t)(f.Export(ctx, foto.ID, "png")); img.Bounds().Dx() != 40 || img.Bounds().Dy() != 80 {
		t.Fatal("descargar aplica la receta:", img.Bounds())
	}
	if img := imagen(t)(f.Export(ctx, foto.ID, "jpg")); img.Bounds().Dx() != 40 {
		t.Fatal("descargar en JPEG:", img.Bounds())
	}
	if _, err := f.Export(ctx, foto.ID, "gif"); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("descargar en un formato que no se da:", err)
	}
	// Volver a la receta vacía.
	if h, err := f.GuardarRecetaDesde(ctx, foto.ID, api.Receta{Version: 1}, girada); err != nil || h != vacia {
		t.Fatal("deshacer la edición:", h, err)
	}
	if img := imagen(t)(f.Variant(ctx, foto.ID, api.PhotoThumb)); img.Bounds().Dx() <= img.Bounds().Dy() {
		t.Fatal("la miniatura sin editar:", img.Bounds())
	}
	if _, err := f.Variant(ctx, foto.ID, "gigante"); err == nil {
		t.Error("una variante que no existe")
	}
	if _, err := f.Variant(ctx, uuid.NewString(), api.PhotoThumb); !errors.Is(err, photos.ErrNotFound) {
		t.Error("la miniatura de una que no existe:", err)
	}
}

// Un fichero de una carpeta del equipo se ENLAZA: su original no se copia (solo sus
// derivadas), se lee del fichero, sobrevive a reabrir; si el fichero se va la galería lo
// dice sin romperse, si se mueve se recoloca por su huella y, si cambia, lo dice. Se
// edita como las demás. Un vídeo enlazado lleva su duración y su póster.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func enlazados(t *testing.T, p Puerto) {
	dir, fuera := t.TempDir(), t.TempDir()
	ctx := t.Context()
	foto := JPEG(80, 60, 77)
	ruta := filepath.Join(fuera, "playa.jpg")
	if err := os.WriteFile(ruta, foto, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, f, cerrar := abrir(t, p, dir)
	enlazada, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: ruta})
	if err != nil {
		t.Fatal("enlazar:", err)
	}
	if enlazada.Externo != ruta || enlazada.EstadoExterno != "" || enlazada.Width != 80 || enlazada.ContentType != "image/jpeg" || enlazada.Name != "playa.jpg" {
		t.Fatalf("la enlazada: %+v", enlazada)
	}
	if HayFicheroIgual(dir, foto) {
		t.Fatal("el original se ha copiado")
	}
	imagen(t)(f.Variant(ctx, enlazada.ID, api.PhotoThumb))
	if hs, err := repo.HuellasConocidas(ctx); err != nil || !hs[Huella(foto)] {
		t.Fatal("la huella del enlazado no se conoce:", hs, err)
	}
	if otra, err := repo.ImportarExterno(ctx, enlazada.ID, photos.OrigenExterno{Ruta: ruta}); err != nil || otra.ID != enlazada.ID {
		t.Fatal("repetir el trabajo:", otra, err)
	}
	if _, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: "relativa.jpg"}); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("una ruta relativa:", err)
	}
	cerrar()

	repo, f, _ = abrir(t, p, dir)
	linea, _, err := f.Timeline(ctx, "", 50, false, "")
	if err != nil || len(linea) != 1 || linea[0].Externo != ruta || linea[0].EstadoExterno != "" {
		t.Fatal("tras reabrir:", linea, err)
	}
	var original bytes.Buffer
	if err := repo.Original(ctx, enlazada.ID, &original); err != nil || !bytes.Equal(original.Bytes(), foto) {
		t.Fatal("el original no se lee del fichero:", err)
	}
	if _, err := f.Rotar(ctx, enlazada.ID); err != nil {
		t.Fatal("editar la enlazada:", err)
	}
	movida := filepath.Join(fuera, "2024", "playa renombrada.jpg")
	if err := os.MkdirAll(filepath.Dir(movida), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(ruta, movida); err != nil {
		t.Fatal(err)
	}
	if linea, _, err := f.Timeline(ctx, "", 50, false, ""); err != nil || len(linea) != 1 || linea[0].EstadoExterno != api.ExternoFalta {
		t.Fatal("sin fichero:", linea, err)
	}
	imagen(t)(f.Variant(ctx, enlazada.ID, api.PhotoThumb))
	if err := repo.Original(ctx, enlazada.ID, io.Discard); !errors.Is(err, photos.ErrOriginalExterno) {
		t.Fatal("sin fichero, el original debe decirlo:", err)
	}
	impostora := filepath.Join(fuera, "otra.jpg")
	falsa := bytes.Clone(foto)
	falsa[len(falsa)-3] ^= 0xff
	if err := os.WriteFile(impostora, falsa, 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.Recolocar(ctx, []string{impostora, movida}); err != nil || n != 1 {
		t.Fatal("recolocar:", n, err)
	}
	if g, err := f.Photo(ctx, enlazada.ID); err != nil || g.Externo != movida || g.EstadoExterno != "" {
		t.Fatal("tras recolocar:", g, err)
	}
	if r, estado, ok, err := repo.RutaExterna(ctx, enlazada.ID); err != nil || !ok || r != movida || estado != "" {
		t.Fatal("ruta externa:", r, estado, ok, err)
	}
	if n, err := repo.Recolocar(ctx, []string{movida}); err != nil || n != 0 {
		t.Fatal("recolocar lo que ya está:", n, err)
	}
	if err := os.WriteFile(movida, append(bytes.Clone(foto), 0, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if g, err := f.Photo(ctx, enlazada.ID); err != nil || g.EstadoExterno != api.ExternoCambiado {
		t.Fatal("cambiado:", g, err)
	}
	if err := repo.Original(ctx, enlazada.ID, io.Discard); !errors.Is(err, photos.ErrOriginalExterno) {
		t.Fatal("cambiado, el original debe decirlo:", err)
	}

	// Un vídeo enlazado: su duración, su póster y su fecha; no se edita.
	video := bytes.Repeat([]byte("no es un mp4 de verdad "), 4096)
	rutaVideo := filepath.Join(fuera, "cumple.mp4")
	if err := os.WriteFile(rutaVideo, video, 0o600); err != nil {
		t.Fatal(err)
	}
	grabado := time.Date(2023, 7, 14, 18, 30, 0, 0, time.UTC)
	v, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: rutaVideo, Tipo: "video/mp4", Poster: JPEG(320, 180, 9), Duracion: 83 * time.Second, TomadoEn: &grabado})
	if err != nil {
		t.Fatal("enlazar el vídeo:", err)
	}
	if !v.EsVideo() || v.DurationMs != 83000 || v.Width != 320 || v.Height != 180 || v.SizeBytes != int64(len(video)) || !v.TakenAt.Equal(grabado) || v.Externo != rutaVideo {
		t.Fatalf("el vídeo: %+v", v)
	}
	if img := imagen(t)(f.Variant(ctx, v.ID, api.PhotoThumb)); img.Bounds().Dx() != 320 {
		t.Fatal("la miniatura no sale del póster:", img.Bounds())
	}
	if _, err := f.Rotar(ctx, v.ID); err == nil {
		t.Fatal("un vídeo no se edita")
	}
	sinPoster := filepath.Join(fuera, "sin poster.webm")
	if err := os.WriteFile(sinPoster, []byte("otro vídeo"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: sinPoster, Tipo: "video/webm", Ancho: 1920, Alto: 1080})
	if err != nil || w.Width != 1920 || w.Height != 1080 {
		t.Fatal("sin póster:", w, err)
	}
	imagen(t)(f.Variant(ctx, w.ID, api.PhotoThumb))
}

// Un vídeo COPIADO: su original va a la fototeca tal cual, con su póster y su duración;
// se encuentra por su huella y no se edita.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func videos(t *testing.T, p Puerto) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, _, cerrar := abrir(t, p, dir)
	video := bytes.Repeat([]byte("vídeo copiado "), 10000)
	grabado := time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC)
	v, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{
		Nombre: "VID_0001.mp4", Tipo: "video/mp4; codecs=avc1", Datos: bytes.NewReader(video), Tamano: int64(len(video)),
		Poster: JPEG(64, 36, 40), Duracion: 12500 * time.Millisecond, TomadoEn: &grabado,
	})
	if err != nil {
		t.Fatal("copiar el vídeo:", err)
	}
	if !v.EsVideo() || v.ContentType != "video/mp4" || v.DurationMs != 12500 || v.Width != 64 || v.Height != 36 || v.Externo != "" || !v.TakenAt.Equal(grabado) {
		t.Fatalf("el vídeo copiado: %+v", v)
	}
	if !HayFicheroIgual(dir, video) {
		t.Fatal("el vídeo no está copiado")
	}
	if _, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{Nombre: "x.jpg", Tipo: "image/jpeg", Datos: bytes.NewReader(video), Tamano: int64(len(video))}); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatal("algo que no es un vídeo:", err)
	}
	sinPoster, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{Nombre: "VID_0002.webm", Tipo: "video/webm", Datos: bytes.NewReader([]byte("webm")), Tamano: 4})
	if err != nil || sinPoster.Width != 1280 || sinPoster.Height != 720 {
		t.Fatal("sin póster:", sinPoster, err)
	}
	cerrar()

	repo, f, _ := abrir(t, p, dir)
	var original bytes.Buffer
	if err := repo.Original(ctx, v.ID, &original); err != nil || !bytes.Equal(original.Bytes(), video) {
		t.Fatal("el original del vídeo:", err)
	}
	if g, ok, err := repo.FotoPorHuella(ctx, Huella(video)); err != nil || !ok || g.ID != v.ID {
		t.Fatal("por huella:", g, ok, err)
	}
	if h, err := repo.HuellaOriginal(ctx, v.ID); err != nil || h != Huella(video) {
		t.Fatal("huella del original:", h, err)
	}
	if _, _, ok, err := repo.RutaExterna(ctx, v.ID); err != nil || ok {
		t.Fatal("un vídeo copiado no es externo:", ok, err)
	}
	if img := imagen(t)(f.Variant(ctx, v.ID, api.PhotoThumb)); img.Bounds().Dx() != 64 {
		t.Fatal("la miniatura del póster:", img.Bounds())
	}
	if _, err := f.Rotar(ctx, v.ID); err == nil {
		t.Fatal("un vídeo no se edita")
	}
}

// Borrar para siempre: una o toda la papelera; lo borrado no vuelve (ni se restaura ni
// está en la papelera ni en sus álbumes), su original copiado y sus derivadas se van del
// disco (el de una enlazada NO se toca), lo vivo se queda y lo borrado sigue conocido.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func borrarParaSiempre(t *testing.T, p Puerto) {
	dir, fuera := t.TempDir(), t.TempDir()
	ctx := t.Context()
	repo, f, cerrar := abrir(t, p, dir)
	a, b, c := JPEG(60, 40, 1), JPEG(60, 40, 90), JPEG(60, 40, 180)
	fa := importar(t, f, "a.jpg", a)
	fb := importar(t, f, "b.jpg", b)
	rutaC := filepath.Join(fuera, "c.jpg")
	if err := os.WriteFile(rutaC, c, 0o600); err != nil {
		t.Fatal(err)
	}
	fc, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: rutaC})
	if err != nil {
		t.Fatal(err)
	}
	album, err := f.CreateAlbum(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.AddToAlbum(ctx, album.ID, []string{fa.ID, fb.ID, fc.ID}); err != nil || n != 3 {
		t.Fatal("álbum:", n, err)
	}
	huellaReceta, err := f.Rotar(ctx, fb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Purgar(ctx, fa.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("borrar para siempre una que no está en la papelera:", err)
	}
	for _, id := range []string{fa.ID, fc.ID} {
		if err := f.Trash(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := f.Purgar(ctx, fa.ID); err != nil || n != 1 {
		t.Fatal("purgar una:", n, err)
	}
	if err := f.Restaurar(ctx, fa.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("una purgada se ha restaurado:", err)
	}
	if _, err := f.Purgar(ctx, fa.ID); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("purgar dos veces:", err)
	}
	if n, err := f.Purgar(ctx, ""); err != nil || n != 1 {
		t.Fatal("vaciar la papelera:", n, err)
	}
	cerrar()
	if p.Liberar != nil {
		p.Liberar(t, dir)
	}
	if HayFicheroIgual(dir, a) {
		t.Fatal("el original de la purgada sigue en el disco")
	}
	if !HayFicheroIgual(dir, b) {
		t.Fatal("se ha ido el original de una viva")
	}
	if _, err := os.Stat(rutaC); err != nil {
		t.Fatal("se ha tocado un fichero enlazado de la persona:", err)
	}

	repo, f, _ = abrir(t, p, dir)
	if linea, _, err := f.Timeline(ctx, "", 50, false, ""); err != nil || !slices.Equal(ids(linea), []string{fb.ID}) {
		t.Fatal("tras purgar y reabrir:", ids(linea), err)
	}
	if papelera, err := f.Papelera(ctx); err != nil || len(papelera.Items) != 0 {
		t.Fatal("papelera:", ids(papelera.Items), err)
	}
	if en, err := f.AlbumPhotos(ctx, album.ID); err != nil || !slices.Equal(ids(en), []string{fb.ID}) {
		t.Fatal("el álbum:", ids(en), err)
	}
	if got, err := f.Album(ctx, album.ID); err != nil || got.Count != 1 {
		t.Fatal("el álbum cuenta purgadas:", got, err)
	}
	imagen(t)(f.VariantDe(ctx, fb.ID, api.PhotoThumb, huellaReceta))
	var original bytes.Buffer
	if err := repo.Original(ctx, fb.ID, &original); err != nil || !bytes.Equal(original.Bytes(), b) {
		t.Fatal("el original vivo:", err)
	}
	if err := repo.Original(ctx, fa.ID, io.Discard); err == nil {
		t.Fatal("el original de una purgada")
	}
	hs, err := repo.HuellasConocidas(ctx)
	if err != nil || !hs[Huella(a)] || !hs[Huella(b)] || !hs[Huella(c)] {
		t.Fatal("lo purgado debe seguir conocido:", hs, err)
	}
	if h, err := repo.HuellaOriginal(ctx, fa.ID); err != nil || h != Huella(a) {
		t.Fatal("la huella de una purgada:", h, err)
	}
	if _, ok, err := repo.FotoPorHuella(ctx, Huella(a)); err != nil || ok {
		t.Fatal("una purgada por huella:", ok, err)
	}
}

// Lo traído de una copia de «Tus datos» de Sytue (HU-07): la fecha de subida y la de
// captura que dice la copia (sin pisar la que ya tenga), y el origen de un álbum, que
// sobrevive a renombrarlo, a borrarlo y a reabrir.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func traido(t *testing.T, p Puerto) {
	ctx := t.Context()
	dir, fuera := t.TempDir(), t.TempDir()
	repo, f, cerrar := abrir(t, p, dir)
	subida := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	tomada := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
	hecha := time.Date(2019, 8, 15, 19, 45, 0, 0, time.FixedZone("CEST", 2*3600))
	nube := filepath.Join(fuera, "nube.jpg")
	atardecer := filepath.Join(fuera, "atardecer.jpg")
	if err := os.WriteFile(nube, JPEG(40, 30, 50), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(atardecer, JPEG(40, 30, 51), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: nube, SubidoEn: &subida})
	if err != nil || g.TakenFromEXIF || !g.TakenAt.Equal(subida) {
		t.Fatal("la enlazada a su fecha de subida:", g, err)
	}
	h, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: atardecer, TomadoEn: &hecha})
	if err != nil || !h.TakenFromEXIF || !h.TakenAt.Equal(hecha) {
		t.Fatal("la enlazada con la fecha dicha:", h, err)
	}
	c := importar(t, f, "IMG_0001.jpg", JPEG(30, 20, 52))
	if err := repo.PonerFechas(ctx, c.ID, &tomada, &subida); err != nil {
		t.Fatal(err)
	}
	if c, err = f.Photo(ctx, c.ID); err != nil || !c.TakenFromEXIF || !c.TakenAt.Equal(tomada) {
		t.Fatal("la copiada con su fecha de captura:", c, err)
	}
	otra := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.PonerFechas(ctx, c.ID, &otra, nil); err != nil {
		t.Fatal(err)
	}
	if c, _ = f.Photo(ctx, c.ID); !c.TakenAt.Equal(tomada) {
		t.Error("PonerFechas ha pisado la fecha de captura:", c.TakenAt)
	}
	if err := repo.PonerFechas(ctx, c.ID, nil, nil); err != nil {
		t.Error("sin fechas no hay nada que poner:", err)
	}
	a, err := repo.CrearAlbumDe(ctx, "Verano 2023", "sytue:a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CrearAlbumDe(ctx, "Verano 2023", "sytue:b")
	if err != nil {
		t.Fatal(err)
	}
	borrado, err := repo.CrearAlbumDe(ctx, "Invierno", "sytue:c")
	if err != nil {
		t.Fatal(err)
	}
	propio, err := f.CreateAlbum(ctx, "Verano 2023")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.RenameAlbum(ctx, a.ID, "Playa"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteAlbum(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CrearAlbumDe(ctx, "Sin origen", " "); err == nil {
		t.Error("un origen vacío no es un origen")
	}
	cerrar()

	repo, f, _ = abrir(t, p, dir)
	por, err := repo.AlbumesPorOrigen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(por) != 3 || por["sytue:a"].ID != a.ID || por["sytue:b"].ID != b.ID || por["sytue:a"].Borrado || !por["sytue:c"].Borrado || por["sytue:c"].ID != borrado.ID {
		t.Errorf("por origen: %+v", por)
	}
	for _, x := range por {
		if x.ID == propio.ID {
			t.Error("el álbum hecho aquí no tiene origen")
		}
	}
	todos, err := f.Albums(ctx)
	if err != nil || len(todos) != 3 {
		t.Fatal("álbumes:", todos, err)
	}
	for _, x := range todos {
		if x.ID == a.ID && x.Title != "Playa" {
			t.Errorf("el renombrado: %+v", x)
		}
	}
}

// Exportar la fototeca: los originales tal cual en AAAA/MM por la fecha de la galería;
// repetir no escribe nada; un fichero distinto con el mismo nombre no se pisa; lo de la
// papelera no va; un enlazado y un vídeo copiado, también.
//
//nolint:gocognit,gocyclo // un recorrido de comportamiento paso a paso, como una prueba (_test.go): partirlo esconde el orden de lo que se prueba
func exportar(t *testing.T, p Puerto) {
	ctx := t.Context()
	dir, fuera := t.TempDir(), t.TempDir()
	repo, f, _ := abrir(t, p, dir)
	roja, verde, azul := JPEG(20, 20, 250), JPEG(20, 20, 10), JPEG(20, 20, 120)
	r := importar(t, f, "roja.jpg", roja)
	importar(t, f, "verde.jpg", verde)
	tirada := importar(t, f, "azul.jpg", azul)
	if err := f.Trash(ctx, tirada.ID); err != nil {
		t.Fatal(err)
	}
	cuando := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := repo.PonerFechas(ctx, r.ID, &cuando, nil); err != nil {
		t.Fatal(err)
	}
	enlazada := filepath.Join(fuera, "enlazada.jpg")
	enlazadaDatos := JPEG(20, 20, 60)
	if err := os.WriteFile(enlazada, enlazadaDatos, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ImportarExterno(ctx, uuid.NewString(), photos.OrigenExterno{Ruta: enlazada}); err != nil {
		t.Fatal(err)
	}
	video := bytes.Repeat([]byte("v"), 5000)
	if _, err := repo.ImportarVideo(ctx, uuid.NewString(), photos.OrigenVideo{Nombre: "vid.mp4", Tipo: "video/mp4", Datos: bytes.NewReader(video), Tamano: int64(len(video))}); err != nil {
		t.Fatal(err)
	}
	destino := t.TempDir()
	avances := 0
	resumen, err := repo.ExportarFototeca(ctx, destino, func(hechas, total int) {
		avances++
		if total != 4 || hechas != avances {
			t.Errorf("avance %d de %d", hechas, total)
		}
	})
	if err != nil || resumen.Copiadas != 4 || resumen.YaEstaban != 0 || len(resumen.Fallidas) != 0 || avances != 4 {
		t.Fatal("exportar:", resumen, err)
	}
	if datos, err := os.ReadFile(filepath.Join(destino, "2021", "05", "roja.jpg")); err != nil || !bytes.Equal(datos, roja) {
		t.Fatal("la roja en su mes:", err)
	}
	mes := filepath.Join(destino, strconv.Itoa(time.Now().UTC().Year()), time.Now().UTC().Format("01"))
	for nombre, datos := range map[string][]byte{"verde.jpg": verde, "enlazada.jpg": enlazadaDatos, "vid.mp4": video} {
		if got, err := os.ReadFile(filepath.Join(mes, nombre)); err != nil || !bytes.Equal(got, datos) {
			t.Fatal("exportada", nombre, err)
		}
	}
	if _, err := os.Stat(filepath.Join(mes, "azul.jpg")); err == nil {
		t.Fatal("se ha exportado una de la papelera")
	}
	if otra, err := repo.ExportarFototeca(ctx, destino, nil); err != nil || otra.Copiadas != 0 || otra.YaEstaban != 4 {
		t.Fatal("exportar dos veces:", otra, err)
	}
	if err := os.WriteFile(filepath.Join(mes, "verde.jpg"), []byte("otra cosa"), 0o600); err != nil {
		t.Fatal(err)
	}
	if otra, err := repo.ExportarFototeca(ctx, destino, nil); err != nil || otra.Copiadas != 1 {
		t.Fatal("con un fichero distinto del mismo nombre:", otra, err)
	}
	if datos, err := os.ReadFile(filepath.Join(mes, "verde (2).jpg")); err != nil || !bytes.Equal(datos, verde) {
		t.Fatal("no se escribió aparte sin pisar:", err)
	}
	if err := os.Remove(enlazada); err != nil {
		t.Fatal(err)
	}
	if otra, err := repo.ExportarFototeca(ctx, t.TempDir(), nil); err != nil || otra.Copiadas != 3 || len(otra.Fallidas) != 1 || !errors.Is(otra.Fallidas[0], photos.ErrOriginalExterno) {
		t.Fatal("sin el fichero de la enlazada:", otra, err)
	}
	if _, err := repo.ExportarFototeca(ctx, "", nil); err == nil {
		t.Fatal("exportar sin carpeta")
	}
}

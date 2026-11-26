// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	sqlfotos "git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// fototeca abre la fototeca de Sytue Office de la carpeta de datos, como Photos: la de
// SQLite (`local/fototeca`).
func fototeca(t *testing.T, datos string) (*sqlfotos.Fototeca, func()) {
	t.Helper()
	repo, err := sqlfotos.Abrir(context.Background(), filepath.Join(datos, "Datos", "photos"), "instalacion", sqlfotos.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	return repo, repo.Close
}

// notepads abre los cuadernos de Sytue Office de la carpeta de datos, como Notepads.
func notepads(t *testing.T, datos string) (*notes.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), filepath.Join(datos, "Datos", "notes"), "notes", "instalacion")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.NewLocal(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

// HU-07: «Traer» lleva las fotos y los vídeos a Photos —enlazados a sus originales, que
// se quedan en Drive una sola vez; una HEIC, como el JPEG que saca el conversor— con su
// favorita, su descripción, sus etiquetas, su fecha (la de subida si no tienen de captura)
// y sus álbumes (dos con el mismo título son dos), y las notas a Notepads desde el HTML
// de cada cuaderno —con su árbol, fechas, tareas, etiquetas, adjuntos e imágenes—, sea
// cual sea el formato en que se pidieron las tablas (JSON, CSV o XML). Lo que Photos no
// sabe abrir aquí (la AVIF, sin su decodificador en este proceso) se queda en Drive y se
// cuenta. Traer otra vez la misma copia no repite ni una foto, ni una nota, ni un álbum.
func TestTraerLlevaFotosAPhotosYNotasANotepadsEnCadaFormato(t *testing.T) {
	for _, f := range []string{"json", "csv", "xml"} {
		t.Run(f, func(t *testing.T) {
			ctx := t.Context()
			zipRuta := tusDatos(t, completo(t, f))
			datos := t.TempDir()
			if _, err := traer.Traer(ctx, zipRuta, destino(datos)); err != nil {
				t.Fatal(err)
			}
			drive := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24", "Drive")
			comprobarFotos(t, datos, drive)
			comprobarNotas(t, datos)

			otra, err := traer.Traer(ctx, zipRuta, destino(datos))
			if err != nil {
				t.Fatal(err)
			}
			if p := parte(otra, "photos"); p.Importados != 0 || p.Repetidos != 6 || p.SinAbrir != 1 {
				t.Errorf("la segunda vez, fotos: %+v", p)
			}
			if p := parte(otra, "notes"); p.Importados != 0 || p.Repetidos != 4 {
				t.Errorf("la segunda vez, notas: %+v", p)
			}
			comprobarFotos(t, datos, drive)
			comprobarNotas(t, datos)
		})
	}
}

func comprobarFotos(t *testing.T, datos, drive string) {
	t.Helper()
	ctx := t.Context()
	repo, cerrar := fototeca(t, datos)
	defer cerrar()
	linea, _, err := repo.Timeline(ctx, "", 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	porNombre := map[string]api.Photo{}
	for _, p := range linea {
		porNombre[p.Name] = p
	}
	if len(linea) != 6 || len(porNombre) != 6 {
		t.Fatalf("fotos en Photos: %+v", linea)
	}
	playa, gato, nieve := porNombre["playa.jpg"], porNombre["gato.png"], porNombre["nieve.jpg"]
	clip, paseo, heic := porNombre["clip.mp4"], porNombre["paseo.webm"], porNombre["IMG_0001.jpg"]
	// Enlazadas a su original en Drive (el de Fotos/, no la copia de Copias/), sin copiarlo.
	for nombre, quiero := range map[string]string{
		"playa.jpg": "Fotos/playa.jpg", "gato.png": "Fotos/gato.png", "nieve.jpg": "Viajes/nieve.jpg",
		"clip.mp4": "Fotos/clip.mp4", "paseo.webm": "Fotos/paseo.webm",
	} {
		if p := porNombre[nombre]; p.Externo != filepath.Join(drive, filepath.FromSlash(quiero)) || p.EstadoExterno != "" {
			t.Errorf("%s no está enlazada a su original en Drive: %q %q", nombre, p.Externo, p.EstadoExterno)
		}
	}
	tomada := time.Date(2023, 8, 15, 17, 45, 0, 0, time.UTC)
	subida := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	if !playa.Favorite || playa.Description != "Atardecer en la playa" || !slices.Equal(playa.Tags, []string{"playa", "verano"}) ||
		!playa.TakenFromEXIF || !playa.TakenAt.Equal(tomada) {
		t.Errorf("playa: %+v", playa)
	}
	// Sin fecha de captura, en la de subida (como en Sytue), y dicho.
	if gato.Favorite || gato.Description != "=SUMA(1) gatos" || len(gato.Tags) != 0 || gato.TakenFromEXIF || !gato.TakenAt.Equal(subida) {
		t.Errorf("gato: %+v", gato)
	}
	if nieve.Favorite || !slices.Equal(nieve.Tags, []string{"nieve"}) || !nieve.TakenAt.Equal(time.Date(2022, 1, 10, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("nieve: %+v", nieve)
	}
	// Los vídeos: el que se sabe medir, con su duración y sus lados; el que no, con los
	// de la tabla.
	if paseo.ContentType != "video/webm" || paseo.DurationMs != 2008 || paseo.Width != 320 || paseo.Height != 180 || !paseo.Favorite ||
		!slices.Equal(paseo.Tags, []string{"paseo"}) || paseo.TakenFromEXIF || !paseo.TakenAt.Equal(time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)) {
		t.Errorf("paseo: %+v", paseo)
	}
	if clip.ContentType != "video/mp4" || clip.Width != 1280 || clip.Height != 720 || !clip.TakenAt.Equal(subida) {
		t.Errorf("clip: %+v", clip)
	}
	// La HEIC, como el JPEG del conversor, copiado a la fototeca, con su fecha y su
	// favorita; su original se queda en Drive.
	if heic.ContentType != "image/jpeg" || heic.Externo != "" || !heic.Favorite || !heic.TakenFromEXIF ||
		!heic.TakenAt.Equal(time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)) {
		t.Errorf("la HEIC: %+v", heic)
	}
	// Lo que Photos no sabe abrir en este proceso (la AVIF) se queda en Drive.
	for _, f := range []string{"Fotos/IMG_0001.heic", "Fotos/flor.avif"} {
		if _, err := os.Stat(filepath.Join(drive, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s no se ha quedado en Drive: %v", f, err)
		}
	}
	if got := albumesPorTitulo(t, repo); !slices.Equal(got, []string{
		"Flores: IMG_0001.jpg", "Invierno: clip.mp4, nieve.jpg", "Verano 2023: gato.png, playa.jpg", "Verano 2023: paseo.webm",
	}) {
		t.Errorf("álbumes: %q", got)
	}
}

// albumesPorTitulo son los álbumes de la fototeca como «título: fotos», en orden.
func albumesPorTitulo(t *testing.T, repo *sqlfotos.Fototeca) []string {
	t.Helper()
	albumes, err := repo.Albums(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(albumes))
	for _, a := range albumes {
		fotos, err := repo.AlbumPhotos(t.Context(), a.ID)
		if err != nil {
			t.Fatal(err)
		}
		nombres := make([]string, 0, len(fotos))
		for _, f := range fotos {
			nombres = append(nombres, f.Name)
		}
		slices.Sort(nombres)
		out = append(out, a.Title+": "+strings.Join(nombres, ", "))
	}
	slices.Sort(out)
	return out
}

func comprobarNotas(t *testing.T, datos string) {
	t.Helper()
	ctx := t.Context()
	repo, cerrar := notepads(t, datos)
	defer cerrar()
	libros, err := repo.ListNotebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libros) != 2 {
		t.Fatalf("cuadernos (el cifrado no entra): %+v", libros)
	}
	i := slices.IndexFunc(libros, func(n api.Notebook) bool { return n.Title == "Casa" })
	j := slices.IndexFunc(libros, func(n api.Notebook) bool { return n.Title == "Presupuestos" })
	if i < 0 || j < 0 {
		t.Fatalf("cuadernos: %+v", libros)
	}
	casa, hijo := libros[i], libros[j]
	if hijo.ParentID == nil || *hijo.ParentID != casa.ID || casa.Emoji != "🏠" || !casa.UpdatedAt.Equal(cambio) {
		t.Errorf("el árbol o el cuaderno: %+v / %+v", casa, hijo)
	}
	ns, err := repo.ListNotes(ctx, casa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 3 {
		t.Fatalf("notas de Casa (la de Drive no entra): %+v", ns)
	}
	por := map[string]api.Note{}
	for _, n := range ns {
		por[n.Title] = n
	}
	texto, tarea, plano := por["Llamar al fontanero"], por["Comprar tejas"], por["Plano"]
	if !strings.Contains(texto.Content, "<b>reforma</b>") || !texto.CreatedAt.Equal(creada) || !texto.UpdatedAt.Equal(cambio) {
		t.Errorf("la nota de texto: %+v", texto)
	}
	if !strings.Contains(string(texto.Data), "pixel.png") || !strings.Contains(string(texto.Data), pixel) {
		t.Errorf("el adjunto no ha venido: %s", texto.Data)
	}
	if len(texto.States) != 1 || texto.States[0].Name != "Alta" || texto.States[0].Emoji != "🔥" {
		t.Errorf("la etiqueta: %+v", texto.States)
	}
	if tarea.Kind != "todo" || !tarea.Checked || tarea.CheckedAt == nil || !tarea.CheckedAt.Equal(hecha) {
		t.Errorf("la tarea: %+v", tarea)
	}
	if plano.Kind != "image" || !strings.Contains(string(plano.Data), "https://ejemplo.invalid/plano.png") {
		t.Errorf("la nota de imagen: %+v", plano)
	}
	if hs, _ := repo.ListNotes(ctx, hijo.ID); len(hs) != 1 || hs[0].Title != "Tejado" {
		t.Errorf("notas del hijo: %+v", hs)
	}
	if tags, _ := repo.ListTags(ctx); len(tags) != 1 || tags[0].Name != "Prioridad" || len(tags[0].States) != 1 {
		t.Errorf("etiquetas: %+v", tags)
	}
}

// Traer otra vez la misma copia vuelve a dejar en Drive el original de una foto enlazada
// que ya no estaba, y Photos lo recoloca; la que se tiró a la papelera de Photos no
// vuelve. Una cuyo original cambió la persona en Drive se queda con su cambio y Photos
// enlaza otra copia, la de Sytue, dejada al lado.
func TestTraerOtraVezRecolocaLoQueFaltaYNoDevuelveLoTirado(t *testing.T) {
	ctx := t.Context()
	zipRuta := tusDatos(t, completo(t, "csv"))
	datos := t.TempDir()
	if _, err := traer.Traer(ctx, zipRuta, destino(datos)); err != nil {
		t.Fatal(err)
	}
	drive := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24", "Drive")
	playa := filepath.Join(drive, "Fotos", "playa.jpg")
	if err := os.Remove(playa); err != nil {
		t.Fatal(err)
	}
	nieve := filepath.Join(drive, "Viajes", "nieve.jpg")
	cambiada := jpegDePrueba(10, 10, 200)
	if err := os.WriteFile(nieve, cambiada, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, cerrar := fototeca(t, datos)
	linea, _, err := repo.Timeline(ctx, "", 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range linea {
		switch p.Name {
		case "gato.png":
			if err := repo.Trash(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		case "playa.jpg":
			if p.EstadoExterno == "" {
				t.Fatal("sin su fichero, la playa no dice que le falta")
			}
		}
	}
	cerrar()

	otra, err := traer.Traer(ctx, zipRuta, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	if p := parte(otra, "photos"); p.Importados != 0 || p.Repetidos != 6 {
		t.Errorf("la segunda vez: %+v", p)
	}
	repo, cerrar = fototeca(t, datos)
	defer cerrar()
	linea, _, err = repo.Timeline(ctx, "", 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	nombres := make([]string, 0, len(linea))
	for _, p := range linea {
		nombres = append(nombres, p.Name)
		if p.Name == "playa.jpg" && (p.Externo != playa || p.EstadoExterno != "") {
			t.Errorf("la playa no se ha recolocado: %q %q", p.Externo, p.EstadoExterno)
		}
		if p.Name == "nieve.jpg" && (p.Externo != filepath.Join(drive, "Viajes", "nieve (2).jpg") || p.EstadoExterno != "") {
			t.Errorf("la nieve cambiada no se ha recolocado en otra copia: %q %q", p.Externo, p.EstadoExterno)
		}
	}
	if raw, _ := os.ReadFile(nieve); !bytes.Equal(raw, cambiada) {
		t.Error("se ha pisado el cambio de la persona")
	}
	slices.Sort(nombres)
	if !slices.Equal(nombres, []string{"IMG_0001.jpg", "clip.mp4", "nieve.jpg", "paseo.webm", "playa.jpg"}) {
		t.Errorf("fotos: %v", nombres)
	}
	if _, err := os.Stat(filepath.Join(drive, "Fotos", "gato (2).png")); err == nil {
		t.Error("el original de la tirada se ha vuelto a copiar")
	}
}

// conOtraFoto es la copia completa en JSON con una foto más, ola.jpg, en el álbum de
// Sytue 0a («Verano 2023»): la de una copia posterior.
func conOtraFoto(t *testing.T) map[string][]byte {
	t.Helper()
	const r = "sytue-one-acme-2026-11-20/"
	out := completo(t, "json")
	ola := jpegDePrueba(24, 16, 77)
	out[r+"drive/files/Fotos/ola.jpg"] = ola
	var fotos, albumes []map[string]any //nolint:prealloc // los llena json.Unmarshal
	if err := json.Unmarshal(out[r+"photos/photos.json"], &fotos); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out[r+"photos/albums.json"], &albumes); err != nil {
		t.Fatal(err)
	}
	const id = "5f070500-0000-0000-0000-000000000009"
	fotos = append(fotos, map[string]any{
		"id": id, "name": "ola.jpg", "content_type": "image/jpeg", "size_bytes": len(ola),
		"taken_at": nil, "uploaded_at": "2025-07-08T09:10:11Z", "favorite": false, "description": "", "tags": nil,
	})
	for _, a := range albumes {
		if a["id"] == "5f070500-0000-0000-0000-00000000000a" {
			a["photos"] = append(a["photos"].([]any), id)
		}
	}
	var err error
	if out[r+"photos/photos.json"], err = json.Marshal(fotos); err != nil {
		t.Fatal(err)
	}
	if out[r+"photos/albums.json"], err = json.Marshal(albumes); err != nil {
		t.Fatal(err)
	}
	return out
}

// HU-07: un álbum se reconoce por su identificador en Sytue, no por su título: una
// copia posterior con una foto más en un álbum que aquí se ha renombrado la pone en el
// renombrado (sin crear otro «Verano 2023» ni juntarlo con el otro que se llama así), y
// un álbum que la persona borró no vuelve.
func TestUnAlbumSeReconocePorSuIdentificador(t *testing.T) {
	ctx := t.Context()
	datos := t.TempDir()
	if _, err := traer.Traer(ctx, tusDatos(t, completo(t, "json")), destino(datos)); err != nil {
		t.Fatal(err)
	}
	repo, cerrar := fototeca(t, datos)
	por, err := repo.AlbumesPorOrigen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	verano, flores := por["sytue:5f070500-0000-0000-0000-00000000000a"], por["sytue:5f070500-0000-0000-0000-00000000000e"]
	if verano.ID == "" || flores.ID == "" || len(por) != 4 {
		t.Fatalf("los álbumes no guardan su identificador de Sytue: %+v", por)
	}
	if err := repo.RenameAlbum(ctx, verano.ID, "Playa"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAlbum(ctx, flores.ID); err != nil {
		t.Fatal(err)
	}
	cerrar()

	inf, err := traer.Traer(ctx, tusDatos(t, conOtraFoto(t)), destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	if p := parte(inf, "photos"); p.Importados != 1 || p.Repetidos != 6 {
		t.Errorf("la copia posterior: %+v", p)
	}
	repo, cerrar = fototeca(t, datos)
	defer cerrar()
	if got := albumesPorTitulo(t, repo); !slices.Equal(got, []string{
		"Invierno: clip.mp4, nieve.jpg", "Playa: gato.png, ola.jpg, playa.jpg", "Verano 2023: paseo.webm",
	}) {
		t.Errorf("álbumes: %q", got)
	}
}

// HU-07: en un equipo sin con qué convertir las HEIC, la HEIC se queda en Drive y se
// dice; traída otra vez con conversor, entra (sin copiarla otra vez a Drive); y luego
// no se repite, tampoco si se pierde lo que se recordaba de ella (se reconoce por la
// huella del JPEG).
func TestUnaHEICSinConversorSeQuedaEnDriveHastaQueLoHay(t *testing.T) {
	ctx := t.Context()
	zipRuta := tusDatos(t, completo(t, "csv"))
	datos := t.TempDir()
	inf, err := traer.Traer(ctx, zipRuta, destinoSinConversor(datos))
	if err != nil {
		t.Fatal(err)
	}
	if sinAbrir, sinConversor := inf.SinAbrir(); sinAbrir != 2 || sinConversor != 1 || parte(inf, "photos").Importados != 5 {
		t.Errorf("sin conversor: %+v", parte(inf, "photos"))
	}
	drive := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24", "Drive")
	if _, err := os.Stat(filepath.Join(drive, "Fotos", "IMG_0001.heic")); err != nil {
		t.Fatal("la HEIC no se ha quedado en Drive:", err)
	}
	// Las tablas de Fotos se dejan en Drive: lo que dicen de la HEIC no se pierde.
	if _, err := os.Stat(filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24", "photos", "photos.csv")); err != nil {
		t.Error("la tabla de Fotos no se ha dejado:", err)
	}

	inf, err = traer.Traer(ctx, zipRuta, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	if p := parte(inf, "photos"); p.Importados != 1 || p.Repetidos != 5 || p.SinAbrir != 1 || p.SinConversor != 0 {
		t.Errorf("con conversor: %+v", p)
	}
	if repetidos := copiasRepetidas(t, filepath.Join(datos, "Drive")); len(repetidos) > 0 {
		t.Errorf("se ha copiado otra vez a Drive: %v", repetidos)
	}
	for _, sinMemoria := range []bool{false, true} {
		if sinMemoria {
			if err := os.Remove(filepath.Join(datos, "Drive", ".traido-de-sytue.json")); err != nil {
				t.Fatal(err)
			}
		}
		inf, err = traer.Traer(ctx, zipRuta, destino(datos))
		if err != nil {
			t.Fatal(err)
		}
		if p := parte(inf, "photos"); p.Importados != 0 || p.Repetidos != 6 {
			t.Errorf("otra vez (sin memoria: %v): %+v", sinMemoria, p)
		}
	}
	repo, cerrar := fototeca(t, datos)
	defer cerrar()
	linea, _, err := repo.Timeline(ctx, "", 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(linea) != 6 {
		t.Errorf("fotos: %+v", linea)
	}
	if got := albumesPorTitulo(t, repo); !slices.Contains(got, "Flores: IMG_0001.jpg") {
		t.Errorf("la HEIC no está en su álbum: %q", got)
	}
}

// HU-07: un cuaderno del diario de la copia entra en Notepads como el diario de su día,
// no como un cuaderno normal; traerlo otra vez no lo repite.
func TestUnCuadernoDelDiarioEntraComoDiario(t *testing.T) {
	ctx := t.Context()
	idDiario := uuid.MustParse("0a000000-0000-4000-8000-000000000009")
	diario := model.Notebook{ID: idDiario, Title: "Lunes, 2 de junio", Color: "#86198f", Layout: model.LayoutList, DailyDate: "2025-06-02", CreatedAt: creada, UpdatedAt: creada}
	nota := model.Note{ID: nota(9), NotebookID: idDiario, Kind: model.KindText, Title: "Lo del lunes", Content: "<p>lunes</p>", CreatedAt: creada, UpdatedAt: creada}
	zipRuta := tusDatos(t, map[string][]byte{
		"sytue-one-2026-11-20/notes/notebooks/Lunes, 2 de junio.html": []byte(model.NotebookAHTML(diario, []model.Note{nota})),
	})
	datos := t.TempDir()
	for vuelta := range 2 {
		inf, err := traer.Traer(ctx, zipRuta, destino(datos))
		if err != nil {
			t.Fatal(err)
		}
		if p := parte(inf, "notes"); p.Importados+p.Repetidos != 1 || (vuelta == 1) != (p.Repetidos == 1) {
			t.Errorf("vuelta %d: %+v", vuelta, p)
		}
	}
	repo, cerrar := notepads(t, datos)
	defer cerrar()
	libros, err := repo.ListNotebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libros) != 1 || libros[0].DailyDate != "2025-06-02" || libros[0].Title != "Lunes, 2 de junio" {
		t.Fatalf("el diario: %+v", libros)
	}
	dia, err := repo.DailyNotebook(ctx, "2025-06-02", "Lunes", false)
	if err != nil || dia.ID != libros[0].ID {
		t.Errorf("Notepads no lo abre como el diario del día: %+v %v", dia, err)
	}
}

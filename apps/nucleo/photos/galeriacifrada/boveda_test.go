// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
)

var scopeBoveda = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "drive"}

// driveDeBoveda es un Drive mínimo cuyos ficheros van por la bóveda de verdad
// (`vaultfiles` sobre el servidor falso de `pqvaulttest`): cifrados en bloques que el
// servidor solo ve opacos. El catálogo (nombres, carpetas, versiones) vive aquí en
// memoria, como en el dispositivo; cada versión es una revisión nueva del mismo objeto.
// Reemplazar exige la versión vista, como `drive.Repository.ReplaceFile`.
type driveDeBoveda struct {
	t       *testing.T
	fake    *pqvaulttest.Fake
	gateway *api.Drive
	store   *vaultfiles.Store
	journal *offline.Store

	mu        sync.Mutex
	n         int
	carpetas  map[string]api.Folder
	ficheros  map[string]api.File
	versiones map[string][]versionCifrada
}

// versionCifrada es dónde está, cifrada, una versión de un fichero.
type versionCifrada struct {
	ref      storage.Reference
	revision string
}

func nuevoDriveDeBoveda(t *testing.T) *driveDeBoveda {
	t.Helper()
	fake, gateway, account := pqvaulttest.New(t, scopeBoveda)
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scopeBoveda, Server: gateway, Registry: account, UnlockKey: bytes.Repeat([]byte{7}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := vaultfiles.New(vault)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	return &driveDeBoveda{
		t: t, fake: fake, gateway: gateway, store: store, journal: journal,
		carpetas: map[string]api.Folder{"raiz": {ID: "raiz", Name: "Mi unidad", IsRoot: true}},
		ficheros: map[string]api.File{}, versiones: map[string][]versionCifrada{},
	}
}

func (d *driveDeBoveda) Root(context.Context) (api.Folder, error) { return d.carpetas["raiz"], nil }

func (d *driveDeBoveda) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var h api.Children
	for _, c := range d.carpetas {
		if c.ParentID != nil && *c.ParentID == carpeta {
			h.Folders = append(h.Folders, c)
		}
	}
	for _, f := range d.ficheros {
		if f.FolderID == carpeta {
			h.Files = append(h.Files, f)
		}
	}
	return h, nil
}

func (d *driveDeBoveda) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.n++
	p := padre
	c := api.Folder{ID: fmt.Sprintf("carpeta-%d", d.n), ParentID: &p, Name: nombre}
	d.carpetas[c.ID] = c
	return c, nil
}

func (d *driveDeBoveda) Upload(ctx context.Context, carpeta, nombre, tipo string, r io.Reader, _ int64) (api.File, error) {
	d.mu.Lock()
	var previo *api.File
	for _, f := range d.ficheros {
		if f.FolderID == carpeta && f.Name == nombre {
			previo = &f
		}
	}
	d.mu.Unlock()
	return d.subir(ctx, carpeta, nombre, tipo, r, previo)
}

// ReplaceFile es `api.DriveFileReplacer`: rechaza una versión que ya no es la vista.
func (d *driveDeBoveda) ReplaceFile(ctx context.Context, original api.File, tipo string, r io.Reader, _ int64) (api.File, error) {
	d.mu.Lock()
	actual, ok := d.ficheros[original.ID]
	d.mu.Unlock()
	if !ok || actual.Version != original.Version || actual.Checksum != original.Checksum {
		return api.File{}, errors.New("la foto cambió en otro dispositivo")
	}
	return d.subir(ctx, actual.FolderID, actual.Name, tipo, r, &actual)
}

func (d *driveDeBoveda) subir(ctx context.Context, carpeta, nombre, tipo string, r io.Reader, previo *api.File) (api.File, error) {
	datos, err := io.ReadAll(r)
	if err != nil {
		return api.File{}, err
	}
	d.mu.Lock()
	d.n++
	n := d.n
	f := api.File{ID: fmt.Sprintf("file_%d", n), FolderID: carpeta, Name: nombre, ContentType: tipo, Version: 1}
	if previo != nil {
		f = *previo
		f.Version++
		f.ContentType = tipo
	}
	d.mu.Unlock()
	revision := fmt.Sprintf("rev_%d", n)
	request := storage.FileRequest{ObjectID: f.ID, RevisionID: revision, SizeBytes: int64(len(datos)), Metadata: []byte(`{}`)}
	ref, err := d.store.UploadStorageFileAt(ctx, d.gateway, d.journal, fmt.Sprintf("job_%d", n), d.store.PublicationTarget(), request, bytes.NewReader(datos))
	if err != nil {
		return api.File{}, err
	}
	ahora := time.Date(2026, 11, 23, 10, 0, n, 0, time.UTC)
	if previo == nil {
		f.CreatedAt = ahora
	}
	f.UpdatedAt, f.SizeBytes, f.Checksum = ahora, int64(len(datos)), revision
	d.mu.Lock()
	d.ficheros[f.ID] = f
	d.versiones[f.ID] = append(d.versiones[f.ID], versionCifrada{ref: ref, revision: revision})
	d.mu.Unlock()
	return f, nil
}

func (d *driveDeBoveda) Download(ctx context.Context, id string) (io.ReadCloser, error) {
	return d.descargarVersion(ctx, id, -1)
}

// descargarVersion baja la versión `v` (desde 0); -1, la última.
func (d *driveDeBoveda) descargarVersion(ctx context.Context, id string, v int) (io.ReadCloser, error) {
	d.mu.Lock()
	versiones := d.versiones[id]
	d.mu.Unlock()
	if len(versiones) == 0 {
		return nil, errors.New("no está")
	}
	if v < 0 {
		v = len(versiones) - 1
	}
	var out bytes.Buffer
	if _, err := d.store.DownloadStorageFile(ctx, d.gateway, versiones[v].ref, id, versiones[v].revision, &out); err != nil {
		return nil, err
	}
	return io.NopCloser(&out), nil
}

func fotoDePrueba(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Sobre la bóveda de verdad: la edición es una versión nueva del mismo fichero, la
// anterior sigue descifrándose de su historial, la entrada se rehace sin perder fecha,
// favorita ni álbum, y al servidor no llega en claro ni la foto ni el índice.
func TestEditarSobreLaBovedaGuardaVersionCifrada(t *testing.T) {
	ctx := t.Context()
	d := nuevoDriveDeBoveda(t)
	g := galeriacifrada.Nueva(d, nil)
	raiz, _ := d.Root(ctx)
	carpeta, err := d.CreateFolder(ctx, raiz.ID, galeriacifrada.CarpetaFotos)
	if err != nil {
		t.Fatal(err)
	}
	original := fotoDePrueba(t, 48, 24)
	f, err := d.Upload(ctx, carpeta.ID, "IMG_0042.jpg", "image/jpeg", bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Indexar(ctx, f, original); err != nil {
		t.Fatal(err)
	}
	album, err := g.CrearAlbum(ctx, "Álbum secreto")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.PonerEnAlbum(ctx, f, album.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Actualizar(ctx, f, func(e *galeriacifrada.Entrada) { e.Favorita = true }); err != nil {
		t.Fatal(err)
	}
	indice, err := g.Leer(ctx)
	if err != nil || len(indice.Fotos) != 1 {
		t.Fatalf("índice = %+v, %v", indice, err)
	}
	foto := indice.Fotos[0]
	tomada := foto.Entrada.Tomada

	receta := model.Receta{Version: model.VersionReceta, Pasos: []model.Paso{
		{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}},
		{Tipo: model.PasoExposicion, Params: map[string]float64{"valor": 0.5}},
	}}
	hecha, err := g.Editar(ctx, foto, receta)
	if err != nil {
		t.Fatal(err)
	}
	e := hecha.Entrada
	if e.Ancho != 24 || e.Alto != 48 || !e.Favorita || len(e.Albumes) != 1 || !e.Tomada.Equal(tomada) {
		t.Fatalf("entrada rehecha = %+v", e)
	}
	d.mu.Lock()
	versiones, actual := len(d.versiones[f.ID]), d.ficheros[f.ID]
	d.mu.Unlock()
	if versiones != 2 || actual.Version != 2 || e.Suma != actual.Checksum {
		t.Fatalf("versiones = %d, fichero = %+v", versiones, actual)
	}
	// La versión anterior sigue en el historial, igual que se subió.
	r, err := d.descargarVersion(ctx, f.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	antes, _ := io.ReadAll(r)
	if !bytes.Equal(antes, original) {
		t.Fatal("la versión anterior no vuelve igual")
	}
	r, err = d.descargarVersion(ctx, f.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	nueva, _ := io.ReadAll(r)
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(nueva)); err != nil || cfg.Width != 24 || cfg.Height != 48 {
		t.Fatalf("la versión nueva mide %+v (%v)", cfg, err)
	}
	// Nada en claro en el servidor: ni la foto, ni el título del álbum, ni el índice.
	for _, blob := range d.fake.Blobs.Data {
		for _, claro := range [][]byte{original[:64], nueva[:64], []byte("Álbum secreto"), []byte("IMG_0042"), []byte(galeriacifrada.TipoFoto)} {
			if bytes.Contains(blob, claro) {
				t.Fatalf("el servidor ve %q en claro", claro)
			}
		}
	}

	// Una edición hecha con la foto ya cambiada en otro dispositivo no la pisa; con
	// la versión que devolvió la edición, sí se guarda otra.
	if _, err := g.Editar(ctx, foto, receta); err == nil {
		t.Fatal("una edición sobre una versión vieja no se guarda")
	}
	d.mu.Lock()
	versiones = len(d.versiones[f.ID])
	d.mu.Unlock()
	if versiones != 2 {
		t.Fatal("se escribió sobre una versión vieja", versiones)
	}
	if _, err := g.Editar(ctx, hecha, receta); err != nil {
		t.Fatal("la versión devuelta vale para la siguiente edición", err)
	}
	d.mu.Lock()
	versiones = len(d.versiones[f.ID])
	d.mu.Unlock()
	if versiones != 3 {
		t.Fatal("se escribió sobre una versión vieja", versiones)
	}
}

// Sobre la bóveda de verdad: el visor y «Descargar» descifran el original solo en
// memoria. La vista grande y la foto entera salen orientadas, la exportación solo va al
// escritor que se le da, y nada de eso escribe en el Drive ni deja nada en claro en el
// servidor.
func TestVerYExportarSobreLaBovedaNoEscribeNada(t *testing.T) {
	ctx := t.Context()
	d := nuevoDriveDeBoveda(t)
	g := galeriacifrada.Nueva(d, nil)
	raiz, _ := d.Root(ctx)
	carpeta, err := d.CreateFolder(ctx, raiz.ID, galeriacifrada.CarpetaFotos)
	if err != nil {
		t.Fatal(err)
	}
	original := fotoDePrueba(t, 60, 30)
	f, err := d.Upload(ctx, carpeta.ID, "IMG_0077.jpg", "image/jpeg", bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	blobsAntes, ficherosAntes := len(d.fake.Blobs.Data), len(d.ficheros)
	d.mu.Unlock()

	vista, err := g.Vista(ctx, f, 40)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(vista)); err != nil || cfg.Width != 40 || cfg.Height != 20 {
		t.Fatalf("la vista mide %+v (%v)", cfg, err)
	}
	img, err := g.Imagen(ctx, f)
	if err != nil || img.Bounds().Dx() != 60 || img.Bounds().Dy() != 30 {
		t.Fatalf("la foto entera = %v (%v)", img, err)
	}
	for _, formato := range []string{"jpeg", "png"} {
		var out bytes.Buffer
		if err := g.Exportar(ctx, f, formato, &out); err != nil {
			t.Fatal(formato, err)
		}
		cfg, tipo, err := image.DecodeConfig(bytes.NewReader(out.Bytes()))
		if err != nil || tipo != formato || cfg.Width != 60 || cfg.Height != 30 {
			t.Fatalf("%s: %s %+v (%v)", formato, tipo, cfg, err)
		}
	}
	d.mu.Lock()
	blobsDespues, ficherosDespues, versiones := len(d.fake.Blobs.Data), len(d.ficheros), len(d.versiones[f.ID])
	d.mu.Unlock()
	if blobsDespues != blobsAntes || ficherosDespues != ficherosAntes || versiones != 1 {
		t.Fatalf("ver o exportar escribió: blobs %d→%d, ficheros %d→%d, versiones %d", blobsAntes, blobsDespues, ficherosAntes, ficherosDespues, versiones)
	}
	for _, blob := range d.fake.Blobs.Data {
		if bytes.Contains(blob, original[:64]) || bytes.Contains(blob, vista[:64]) {
			t.Fatal("el servidor ve la foto en claro")
		}
	}
	if _, err := g.Vista(ctx, api.File{ID: "no/vale"}, 0); !errors.Is(err, galeriacifrada.ErrSinFoto) {
		t.Fatal("un id que no es de la galería no se lee", err)
	}
}

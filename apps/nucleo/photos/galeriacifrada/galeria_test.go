// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// almacenEnMemoria es un Drive mínimo: carpetas, ficheros con versiones (subir con
// el mismo nombre crea una versión nueva del mismo id, como el Drive de la bóveda).
type almacenEnMemoria struct {
	mu        sync.Mutex
	n         int
	carpetas  map[string]api.Folder
	ficheros  map[string]api.File
	contenido map[string][]byte
	versiones map[string]int
	subidas   int
	fallar    map[string]bool // ids cuya descarga falla
}

func nuevoAlmacen() *almacenEnMemoria {
	a := &almacenEnMemoria{
		carpetas:  map[string]api.Folder{"raiz": {ID: "raiz", Name: "Mi unidad", IsRoot: true}},
		ficheros:  map[string]api.File{},
		contenido: map[string][]byte{},
		versiones: map[string]int{},
		fallar:    map[string]bool{},
	}
	return a
}

func (a *almacenEnMemoria) id() string {
	a.n++
	return fmt.Sprintf("id-%04d", a.n)
}

func (a *almacenEnMemoria) Root(context.Context) (api.Folder, error) {
	return a.carpetas["raiz"], nil
}

func (a *almacenEnMemoria) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var h api.Children
	for _, c := range a.carpetas {
		if c.ParentID != nil && *c.ParentID == carpeta {
			h.Folders = append(h.Folders, c)
		}
	}
	for _, f := range a.ficheros {
		if f.FolderID == carpeta {
			h.Files = append(h.Files, f)
		}
	}
	return h, nil
}

func (a *almacenEnMemoria) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := padre
	c := api.Folder{ID: a.id(), ParentID: &p, Name: nombre}
	a.carpetas[c.ID] = c
	return c, nil
}

func (a *almacenEnMemoria) Upload(_ context.Context, carpeta, nombre, tipo string, r io.Reader, _ int64) (api.File, error) {
	datos, err := io.ReadAll(r)
	if err != nil {
		return api.File{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.subidas++
	ahora := time.Date(2026, 11, 23, 10, 0, a.subidas, 0, time.UTC)
	for id, f := range a.ficheros {
		if f.FolderID != carpeta || f.Name != nombre {
			continue
		}
		f.Version++
		f.UpdatedAt = ahora
		f.SizeBytes = int64(len(datos))
		f.Checksum = fmt.Sprintf("suma-%d", a.subidas)
		a.ficheros[id] = f
		a.contenido[id] = datos
		a.versiones[id]++
		return f, nil
	}
	f := api.File{ID: a.id(), FolderID: carpeta, Name: nombre, ContentType: tipo, Version: 1, SizeBytes: int64(len(datos)), CreatedAt: ahora, UpdatedAt: ahora, Checksum: fmt.Sprintf("suma-%d", a.subidas)}
	a.ficheros[f.ID] = f
	a.contenido[f.ID] = datos
	a.versiones[f.ID] = 1
	return f, nil
}

func (a *almacenEnMemoria) Download(_ context.Context, id string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallar[id] {
		return nil, errors.New("descarga rota")
	}
	datos, ok := a.contenido[id]
	if !ok {
		return nil, errors.New("no está")
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(datos))), nil
}

// fotoConExif es un JPEG de 40×20 con orientación 6 (girada) y fecha de captura.
func fotoConExif(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: 100, B: uint8(y * 10), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	tiff := make([]byte, 76)
	copy(tiff, "II")
	le.PutUint16(tiff[2:], 42)
	le.PutUint32(tiff[4:], 8)
	le.PutUint16(tiff[8:], 2)
	entrada := func(off int, tag, tipo uint16, n, v uint32) {
		le.PutUint16(tiff[off:], tag)
		le.PutUint16(tiff[off+2:], tipo)
		le.PutUint32(tiff[off+4:], n)
		le.PutUint32(tiff[off+8:], v)
	}
	entrada(10, 0x0112, 3, 1, 6)
	entrada(22, 0x8769, 4, 1, 38)
	le.PutUint16(tiff[38:], 1)
	entrada(40, 0x9003, 2, 20, 56)
	copy(tiff[56:], "2026:07:14 18:32:05\x00")
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(2+6+len(tiff)))
	app1 = append(append(app1, "Exif\x00\x00"...), tiff...)
	jpg := buf.Bytes()
	return append(append(append([]byte{}, jpg[:2]...), app1...), jpg[2:]...)
}

func subirFoto(t *testing.T, a *almacenEnMemoria, nombre string, datos []byte) api.File {
	t.Helper()
	ctx := context.Background()
	g := Nueva(a, nil)
	fotos, err := g.carpeta(ctx, "fotos", true)
	if err != nil {
		t.Fatal(err)
	}
	f, err := a.Upload(ctx, fotos, nombre, "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestIndexarYLeer(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	f := subirFoto(t, a, "IMG_0001.jpg", datos)
	g := Nueva(a, nil)
	if err := g.Indexar(ctx, f, datos); err != nil {
		t.Fatal(err)
	}
	i, err := Nueva(a, nil).Leer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Fotos) != 1 {
		t.Fatalf("fotos = %d", len(i.Fotos))
	}
	e := i.Fotos[0]
	if !e.ConEntrada || e.Pendiente || !e.Entrada.Exif {
		t.Fatalf("foto = %+v", e)
	}
	if want := time.Date(2026, 7, 14, 18, 32, 5, 0, time.UTC); !e.Entrada.Tomada.Equal(want) {
		t.Fatalf("tomada = %v", e.Entrada.Tomada)
	}
	// Orientación 6: las medidas son las de la foto ya girada.
	if e.Entrada.Ancho != 20 || e.Entrada.Alto != 40 {
		t.Fatalf("medidas = %dx%d", e.Entrada.Ancho, e.Entrada.Alto)
	}
	mini, err := g.LeerMiniatura(ctx, e.Entrada)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(mini)); err != nil || cfg.Width != 20 || cfg.Height != 40 {
		t.Fatalf("miniatura = %+v, %v", cfg, err)
	}
}

func TestIndexarEsIdempotenteYConservaFavoritaYAlbumes(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	f := subirFoto(t, a, "IMG_0001.jpg", datos)
	g := Nueva(a, nil)
	if err := g.Indexar(ctx, f, datos); err != nil {
		t.Fatal(err)
	}
	album, err := g.CrearAlbum(ctx, "  Verano  ")
	if err != nil {
		t.Fatal(err)
	}
	cambio := func(e *Entrada) { e.Favorita = true; e.Albumes = append(e.Albumes, album.ID) }
	if _, err := g.Actualizar(ctx, f, cambio); err != nil {
		t.Fatal(err)
	}
	antes := a.subidas
	// Repetir el mismo cambio o volver a indexar el mismo contenido no escribe nada.
	if _, err := g.Actualizar(ctx, f, cambio); err != nil {
		t.Fatal(err)
	}
	if err := g.Indexar(ctx, f, datos); err != nil {
		t.Fatal(err)
	}
	if a.subidas != antes {
		t.Fatalf("subidas de más: %d", a.subidas-antes)
	}
	// Una edición (otro contenido) rehace la entrada sin perder favorita ni álbum.
	editada, err := a.Upload(ctx, f.FolderID, f.Name, "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	if editada.ID != f.ID {
		t.Fatal("la edición tenía que ser una versión del mismo fichero")
	}
	i, err := Nueva(a, nil).Leer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !i.Fotos[0].Pendiente {
		t.Fatal("con otro contenido la entrada queda pendiente")
	}
	if err := g.Reconstruir(ctx, editada); err != nil {
		t.Fatal(err)
	}
	i, err = Nueva(a, nil).Leer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e := i.Fotos[0]
	if e.Pendiente || !e.Entrada.Favorita || len(e.Entrada.Albumes) != 1 || e.Entrada.Albumes[0] != album.ID {
		t.Fatalf("entrada = %+v", e)
	}
	if len(i.Albumes) != 1 || i.Albumes[0].Titulo != "Verano" || len(i.DelAlbum(album.ID)) != 1 || len(i.Favoritas()) != 1 {
		t.Fatalf("álbumes = %+v", i.Albumes)
	}
}

func TestLeerToleraEntradasQueFaltanORotas(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	sin := subirFoto(t, a, "sin-entrada.jpg", datos)
	rota := subirFoto(t, a, "rota.jpg", datos)
	ajena := subirFoto(t, a, "ajena.jpg", datos)
	g := Nueva(a, nil)
	indice, err := g.carpeta(ctx, CarpetaIndice, true)
	if err != nil {
		t.Fatal(err)
	}
	basura := []byte(`{"v":1,"tipo":"otra"}`)
	if _, err := a.Upload(ctx, indice, rota.ID+".json", "application/json", bytes.NewReader(basura), int64(len(basura))); err != nil {
		t.Fatal(err)
	}
	// Una entrada que dice ser de otra foto no se usa.
	if err := g.Indexar(ctx, sin, datos); err != nil {
		t.Fatal(err)
	}
	otra, _, _ := g.entradaActual(ctx, sin.ID)
	crudo := mustJSON(t, otra)
	if _, err := a.Upload(ctx, indice, ajena.ID+".json", "application/json", bytes.NewReader(crudo), int64(len(crudo))); err != nil {
		t.Fatal(err)
	}
	// Y una que no se puede descargar tampoco para el resto.
	for id, f := range a.ficheros {
		if f.Name == sin.ID+".json" {
			a.fallar[id] = true
		}
	}
	i, err := Nueva(a, nil).Leer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(i.Fotos) != 3 {
		t.Fatalf("fotos = %d", len(i.Fotos))
	}
	for _, f := range i.Fotos {
		if f.ConEntrada || !f.Pendiente || f.Entrada.Nombre != f.Fichero.Name {
			t.Fatalf("foto sin entrada válida = %+v", f)
		}
	}
	// Reconstruir desde el original vuelve a dejarla al día.
	if err := Nueva(a, nil).Reconstruir(ctx, rota); err != nil {
		t.Fatal(err)
	}
	i, _ = Nueva(a, nil).Leer(ctx)
	for _, f := range i.Fotos {
		if f.Fichero.ID == rota.ID && (!f.ConEntrada || f.Pendiente) {
			t.Fatalf("rota sin reconstruir: %+v", f)
		}
	}
}

func mustJSON(t *testing.T, e Entrada) []byte {
	t.Helper()
	datos, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return datos
}

// El formato es el mismo que escribe y lee la web (galeriaCifrada.test.ts usa los
// mismos ficheros).
func TestFormatoCompartidoConLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/entrada-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	e, err := DecodificarEntrada(datos)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Favorita || e.Ancho != 4032 || e.Tomada.Year() != 2026 || len(e.Albumes) != 1 {
		t.Fatalf("entrada = %+v", e)
	}
	datos, err = os.ReadFile("testdata/album-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if a, err := DecodificarAlbum(datos); err != nil || a.Titulo != "Verano" {
		t.Fatalf("álbum = %+v, %v", a, err)
	}
	for _, malo := range []string{
		`{"v":2,"tipo":"sytue.photos.cifrada.foto","foto":"a"}`,
		`{"v":1,"tipo":"sytue.photos.cifrada.foto","foto":"../x"}`,
		`{"v":1,"tipo":"sytue.photos.cifrada.foto","foto":"a","albumes":["a/b"]}`,
		`no es json`,
	} {
		if _, err := DecodificarEntrada([]byte(malo)); !errors.Is(err, ErrEntrada) {
			t.Errorf("%s: %v", malo, err)
		}
	}
}

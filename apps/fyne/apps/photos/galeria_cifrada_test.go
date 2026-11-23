// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
)

// TestSeccionCifradas: «En tu Drive cifrado» dice lo que no incluye a esas fotos,
// ofrece abrir el Drive cerrado sin abrirlo solo y, con el índice vacío, lo dice con el
// vacío compacto. Las fotos van en la línea de tiempo, no en la sección.
func TestSeccionCifradas(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, nil)
	nota := v.Traduce("gallery.encrypted.note")
	if nota == "gallery.encrypted.note" || !strings.Contains(nota, "Sytue AI") {
		t.Fatalf("falta la nota en el catálogo nativo: %q", nota)
	}
	cerrada := rotulosDeBotones(SeccionCifradas(v, DatosCifradas{Cerrada: true, AlMostrar: func() {}}))
	if !slices.Contains(cerrada, v.Traduce("gallery.encrypted.show")) {
		t.Fatalf("sin «Mostrar»: %q", cerrada)
	}
	vacia := SeccionCifradas(v, DatosCifradas{Vacia: true})
	if !slices.Contains(textosDePantalla(vacia), v.Traduce("gallery.encrypted.empty")) {
		t.Fatalf("sin el vacío: %q", textosDePantalla(vacia))
	}
	if !compacto(vacia) {
		t.Fatal("el vacío de la sección es el compacto")
	}
	abierta := textosDePantalla(SeccionCifradas(v, DatosCifradas{}))
	if !slices.Contains(abierta, nota) || slices.Contains(abierta, v.Traduce("gallery.encrypted.empty")) {
		t.Fatalf("abierta con fotos: %q", abierta)
	}

	a := &App{v: v, ruta: RutaFotos}
	a.GaleriaCifrada = func(context.Context, bool) (*api.Drive, error) { return nil, ErrDriveCifradoCerrado }
	a.cif = estadoCifradas{enBoveda: true, cerrada: true}
	if a.seccionCifradas() == nil {
		t.Fatal("con el Drive en la bóveda la galería tiene su sección")
	}
	a.g.texto = "playa"
	if a.seccionCifradas() == nil {
		t.Fatal("con una búsqueda la sección sigue: las cifradas también se buscan")
	}
	a.cif.enBoveda = false
	if a.seccionCifradas() != nil {
		t.Fatal("sin el Drive en la bóveda no hay sección")
	}
}

func compacto(o fyne.CanvasObject) bool {
	switch x := o.(type) {
	case *fyne.Container:
		return slices.ContainsFunc(x.Objects, compacto)
	case *ui.Vacio:
		return x.Compacto
	}
	return false
}

func cifradaDePrueba(id string, tomada time.Time, favorita bool, albumes ...string) galeriacifrada.Foto {
	return galeriacifrada.Foto{
		Fichero:    api.File{ID: id, Name: id + ".jpg"},
		Entrada:    galeriacifrada.Entrada{Foto: id, Nombre: id + ".jpg", Tomada: tomada, Exif: true, Ancho: 4, Alto: 3, Favorita: favorita, Albumes: albumes},
		ConEntrada: true,
	}
}

func idsDeLinea(fotos []api.Photo) string {
	out := make([]string, len(fotos))
	for i, f := range fotos {
		out[i] = f.ID
	}
	return strings.Join(out, " ")
}

// TestLineaMezcladaConCifradas: las cifradas entran en la línea por fecha cuando la del
// servidor ha llegado a la suya, en favoritas solo las marcadas, y con una búsqueda
// delante, por nombre; los meses las cuentan como a las demás.
func TestLineaMezcladaConCifradas(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, nil)
	dia := func(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 12, 0, 0, 0, time.UTC) }
	a := &App{v: v, ruta: RutaFotos}
	a.g.fotos = []api.Photo{{ID: "s1", Name: "s1", TakenAt: dia(9, 20)}, {ID: "s2", Name: "s2", TakenAt: dia(8, 1)}}
	if got := idsDeLinea(a.lineaMezclada()); got != "s1 s2" {
		t.Fatalf("sin galería cifrada leída, la del servidor: %q", got)
	}
	a.cif = estadoCifradas{enBoveda: true, galeria: galeriacifrada.Nueva(nil, nil), indice: galeriacifrada.Indice{Fotos: []galeriacifrada.Foto{
		cifradaDePrueba("nueva", dia(10, 1), true),
		cifradaDePrueba("playa", dia(8, 15), false),
		cifradaDePrueba("vieja", dia(3, 1), true),
	}}}
	if got := idsDeLinea(a.lineaMezclada()); got != "cifrada:nueva s1 cifrada:playa s2" {
		t.Fatalf("línea = %q", got)
	}
	a.g.fin = true
	linea := a.lineaMezclada()
	if got := idsDeLinea(linea); got != "cifrada:nueva s1 cifrada:playa s2 cifrada:vieja" {
		t.Fatalf("con la línea terminada = %q", got)
	}
	if !linea[0].Favorite || linea[0].Width != 4 || !linea[0].TakenFromEXIF || linea[0].Name != "nueva.jpg" {
		t.Fatalf("la cifrada en la línea = %+v", linea[0])
	}
	if meses := PorMes(linea, v.T); len(meses) != 4 || len(meses[1].Fotos) != 1 || len(meses[2].Fotos) != 2 {
		t.Fatalf("meses = %+v", meses)
	}
	a.ruta = RutaFavoritas
	if got := idsDeLinea(a.lineaMezclada()); got != "cifrada:nueva s1 s2 cifrada:vieja" {
		t.Fatalf("en favoritas = %q", got)
	}
	a.ruta = RutaFotos
	a.g.texto = "PLAYA"
	if got := idsDeLinea(a.lineaMezclada()); got != "cifrada:playa s1 s2" {
		t.Fatalf("con búsqueda = %q", got)
	}
	if f, ok := a.cifradaDeLinea("cifrada:playa"); !ok || f.Fichero.ID != "playa" {
		t.Fatal("el id de la línea lleva a su foto cifrada")
	}
	if _, ok := a.cifradaDeLinea("s1"); ok {
		t.Fatal("una del servidor no es cifrada")
	}
}

// TestTeselaCifradaNoVaEnLote: la tesela de una cifrada lleva el candado, se abre y se
// marca como favorita; en el modo selección no se marca y lo dice.
func TestTeselaCifradaNoVaEnLote(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, nil)
	f := FotoDeLinea(cifradaDePrueba("c1", time.Now(), false))
	var abiertas, alternadas []string
	d := Galeria{AlAbrir: func(p api.Photo) { abiertas = append(abiertas, p.ID) }, AlFavorita: func(api.Photo) {}, AlMantener: func(api.Photo) { t.Fatal("una cifrada no empieza una selección") }}
	m := teselaDe(v, f, d)
	if !m.Cifrada || m.AlFavorita == nil || m.AlMantener != nil || !strings.Contains(m.Rotulo, "c1.jpg") {
		t.Fatalf("tesela = %+v", m)
	}
	m.AlPulsar()
	if !slices.Equal(abiertas, []string{f.ID}) {
		t.Fatalf("abiertas = %q", abiertas)
	}
	d.EnSeleccion, d.Seleccion = true, map[string]bool{}
	d.AlAlternar = func(p api.Photo) { alternadas = append(alternadas, p.ID) }
	m = teselaDe(v, f, d)
	if m.Seleccionando || m.Rotulo != v.Traduce("gallery.encrypted.notSelectable") {
		t.Fatalf("en selección = %+v", m)
	}
	m.AlPulsar()
	if len(alternadas) != 1 {
		t.Fatal("pulsarla en selección lo dice (alternar)")
	}

	a := Nueva(nil, v, nil)
	a.g.seleccion = map[string]bool{}
	a.alternar(f)
	a.alternarMes([]api.Photo{f, {ID: "s1"}})
	if a.g.seleccion[f.ID] || !a.g.seleccion["s1"] || len(a.avisos.Avisos()) != 1 {
		t.Fatalf("selección = %v, avisos = %d", a.g.seleccion, len(a.avisos.Avisos()))
	}
}

// almacenCifrado es un Drive en memoria para la galería cifrada: subir con el mismo
// nombre es una versión nueva del mismo fichero, como el de la bóveda.
type almacenCifrado struct {
	mu        sync.Mutex
	n         int
	carpetas  map[string]api.Folder
	ficheros  map[string]api.File
	contenido map[string][]byte
}

func nuevoAlmacenCifrado() *almacenCifrado {
	return &almacenCifrado{carpetas: map[string]api.Folder{"raiz": {ID: "raiz", IsRoot: true}}, ficheros: map[string]api.File{}, contenido: map[string][]byte{}}
}

func (a *almacenCifrado) Root(context.Context) (api.Folder, error) { return a.carpetas["raiz"], nil }

func (a *almacenCifrado) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
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

func (a *almacenCifrado) CreateFolder(_ context.Context, padre, nombre string) (api.Folder, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.n++
	c := api.Folder{ID: fmt.Sprintf("c%d", a.n), ParentID: &padre, Name: nombre}
	a.carpetas[c.ID] = c
	return c, nil
}

func (a *almacenCifrado) Upload(_ context.Context, carpeta, nombre, tipo string, r io.Reader, _ int64) (api.File, error) {
	datos, err := io.ReadAll(r)
	if err != nil {
		return api.File{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.n++
	for id, f := range a.ficheros {
		if f.FolderID == carpeta && f.Name == nombre {
			f.Version++
			f.Checksum = fmt.Sprintf("suma-%d", a.n)
			a.ficheros[id], a.contenido[id] = f, datos
			return f, nil
		}
	}
	f := api.File{
		ID: fmt.Sprintf("f%d", a.n), FolderID: carpeta, Name: nombre, ContentType: tipo, Version: 1, SizeBytes: int64(len(datos)),
		CreatedAt: time.Date(2026, 6, 1, 0, 0, a.n, 0, time.UTC), Checksum: fmt.Sprintf("suma-%d", a.n),
	}
	a.ficheros[f.ID], a.contenido[f.ID] = f, datos
	return f, nil
}

func (a *almacenCifrado) Download(_ context.Context, id string) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	datos, ok := a.contenido[id]
	if !ok {
		return nil, errors.New("no está")
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(datos))), nil
}

func jpegDePrueba(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 9), G: uint8(y * 13), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// galeriaDePrueba deja una foto indexada en «Fotos» y el índice leído en la app.
func galeriaDePrueba(t *testing.T, a *App) (*almacenCifrado, *galeriacifrada.Galeria) {
	t.Helper()
	ctx := t.Context()
	almacen := nuevoAlmacenCifrado()
	g := galeriacifrada.Nueva(almacen, nil)
	fotos, err := almacen.CreateFolder(ctx, "raiz", galeriacifrada.CarpetaFotos)
	if err != nil {
		t.Fatal(err)
	}
	datos := jpegDePrueba(t, 30, 20)
	f, err := almacen.Upload(ctx, fotos.ID, "IMG_1.jpg", "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Indexar(ctx, f, datos); err != nil {
		t.Fatal(err)
	}
	indice, err := g.Leer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.cif = estadoCifradas{pedida: true, enBoveda: true, galeria: g, indice: indice}
	a.minisCif.poner(g, indice)
	return almacen, g
}

// TestAlbumesCifradosDesdeLaApp: la página de álbumes enseña los cifrados con su
// portada descifrada (nunca pedida al servidor); meter y sacar una foto escribe solo en
// el índice, y el álbum abierto lo enseña (vacío compacto sin fotos).
func TestAlbumesCifradosDesdeLaApp(t *testing.T) {
	test.NewTempApp(t)
	var alServidor []string
	teca := NuevaFototeca()
	teca.Pedir = func(_ context.Context, id, _ string) ([]byte, error) {
		alServidor = append(alServidor, id)
		return nil, errors.New("no")
	}
	a := Nueva(nil, vistaDePrueba(t, teca), nil)
	a.lanzar = func(f func()) { f() }
	a.GaleriaCifrada = func(context.Context, bool) (*api.Drive, error) { return nil, ErrDriveCifradoCerrado }
	a.desviarCifradas()
	almacen, g := galeriaDePrueba(t, a)
	foto := a.cif.indice.Fotos[0]
	album, err := g.CrearAlbum(t.Context(), "Viaje cifrado")
	if err != nil {
		t.Fatal(err)
	}
	a.cif.indice.Albumes = []galeriacifrada.Album{album}
	a.ruta = RutaAlbumes

	a.elegirAlbumCifrado(album.ID)
	seccion := a.seccionAlbumesCifrados()
	textos := textosDePantalla(seccion)
	if !slices.Contains(textos, "Viaje cifrado") || !slices.Contains(textos, a.v.Traduce("gallery.encrypted.albumEmpty")) || !compacto(seccion) {
		t.Fatalf("sección = %q", textos)
	}
	if !slices.Contains(rotulosDeBotones(seccion), a.v.Traduce("gallery.encrypted.newAlbum")) {
		t.Fatal("con el índice leído se ofrece crear un álbum cifrado")
	}

	a.albumCifrado(foto, album.ID, true)
	if got := a.cif.indice.DelAlbum(album.ID); len(got) != 1 {
		t.Fatalf("en el álbum = %d", len(got))
	}
	if versiones := almacen.ficheros[foto.Fichero.ID].Version; versiones != 1 {
		t.Fatal("meterla en un álbum no toca la foto", versiones)
	}
	textos = textosDePantalla(a.seccionAlbumesCifrados())
	if slices.Contains(textos, a.v.Traduce("gallery.encrypted.albumEmpty")) {
		t.Fatalf("el álbum ya tiene la foto: %q", textos)
	}
	// La portada y las teselas son de la foto cifrada, descifrada en el dispositivo.
	mini, err := teca.Pedir(t.Context(), idDeLinea(foto.Fichero.ID), VarianteMiniatura)
	if err != nil || len(mini) == 0 {
		t.Fatal("la miniatura cifrada no se descifró", err)
	}
	if len(alServidor) != 0 {
		t.Fatalf("se pidió al servidor %q", alServidor)
	}
	// Releída, el índice dice lo mismo.
	releido, err := galeriacifrada.Nueva(almacen, nil).Leer(t.Context())
	if err != nil || len(releido.DelAlbum(album.ID)) != 1 {
		t.Fatalf("releído = %+v, %v", releido, err)
	}

	a.albumCifrado(a.cif.indice.Fotos[0], album.ID, false)
	if got := a.cif.indice.DelAlbum(album.ID); len(got) != 0 {
		t.Fatalf("sigue en el álbum = %d", len(got))
	}
	a.favoritaCifrada(a.cif.indice.Fotos[0], true)
	if !a.cif.indice.Fotos[0].Entrada.Favorita {
		t.Fatal("la favorita no se apuntó")
	}
}

// TestEditorCifradoGuardaVersion: el editor de una cifrada no ofrece Sytue AI, guarda
// con «Guardar como versión nueva» y la foto queda como versión nueva del mismo
// fichero, con la entrada rehecha.
func TestEditorCifradoGuardaVersion(t *testing.T) {
	test.NewTempApp(t)
	a := Nueva(nil, vistaDePrueba(t, NuevaFototeca()), nil)
	a.lanzar = func(f func()) { f() }
	a.GaleriaCifrada = func(context.Context, bool) (*api.Drive, error) { return nil, ErrDriveCifradoCerrado }
	almacen, _ := galeriaDePrueba(t, a)
	foto := a.cif.indice.Fotos[0]

	a.abrirEditorCifrado(foto)
	e := a.ed
	if e == nil || e.cifrada == nil || e.original == nil || e.cargando {
		t.Fatalf("el editor no cargó la previa: %+v", e)
	}
	if b := e.original.Bounds(); b.Dx() != 30 || b.Dy() != 20 {
		t.Fatalf("previa = %v", b)
	}
	textos, rotulos := textosDePantalla(e.capa.dentro), rotulosDeBotones(e.capa.dentro)
	if !slices.Contains(rotulos, a.v.Traduce("gallery.encrypted.saveVersion")) || slices.Contains(textos, a.v.Traduce("editor.tabAi")) {
		t.Fatalf("editor cifrado: botones %q", rotulos)
	}
	a.editarConIA("auto")
	if e.iaCargando {
		t.Fatal("Sytue AI no edita una cifrada")
	}
	a.anyadirPaso(api.PasoReceta{Tipo: api.PasoGiro, Params: map[string]float64{"grados": 90}})
	a.guardarEdicion()
	if a.ed != nil {
		t.Fatal("guardada, el editor se cierra")
	}
	f := almacen.ficheros[foto.Fichero.ID]
	if f.Version != 2 {
		t.Fatalf("versión = %d", f.Version)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(almacen.contenido[foto.Fichero.ID]))
	if err != nil || cfg.Width != 20 || cfg.Height != 30 {
		t.Fatalf("la versión nueva mide %+v (%v)", cfg, err)
	}
	ahora := a.cif.indice.Fotos[0]
	if ahora.Fichero.Version != 2 || ahora.Entrada.Ancho != 20 || ahora.Entrada.Alto != 30 || !ahora.Entrada.Tomada.Equal(foto.Entrada.Tomada) {
		t.Fatalf("en el índice = %+v", ahora)
	}
	if avisos := a.avisos.Avisos(); len(avisos) != 1 || avisos[0].Titulo != a.v.Traduce("gallery.encrypted.saved") {
		t.Fatal("no dijo que se guardó como versión nueva")
	}
}

func rotulosDeBotones(o fyne.CanvasObject) []string {
	var out []string
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			out = append(out, rotulosDeBotones(h)...)
		}
	case *ui.Boton:
		out = append(out, x.Texto)
	}
	return out
}

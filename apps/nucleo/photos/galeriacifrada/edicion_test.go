// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

func leerIndice(t *testing.T, a Almacen) Indice {
	t.Helper()
	i, err := Nueva(a, nil).Leer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// Editar sube una versión nueva del MISMO fichero (la anterior queda en su historial),
// con la receta aplicada sobre la foto ya orientada, y la entrada conserva la fecha de
// captura del EXIF, la favorita y los álbumes.
func TestEditarGuardaUnaVersionNueva(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	f := subirFoto(t, a, "IMG_0001.jpg", datos)
	g := Nueva(a, nil)
	if err := g.Indexar(ctx, f, datos); err != nil {
		t.Fatal(err)
	}
	album, err := g.CrearAlbum(ctx, "Verano")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.PonerEnAlbum(ctx, f, album.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Actualizar(ctx, f, func(e *Entrada) { e.Favorita = true }); err != nil {
		t.Fatal(err)
	}
	foto := leerIndice(t, a).Fotos[0]

	if _, err := g.Editar(ctx, foto, model.RecetaVacia()); !errors.Is(err, ErrRecetaVacia) {
		t.Fatal("una receta vacía no guarda nada", err)
	}
	receta := model.Receta{Version: model.VersionReceta, Pasos: []model.Paso{{Tipo: model.PasoGiro, Params: map[string]float64{"grados": 90}}}}
	hecha, err := g.Editar(ctx, foto, receta)
	if err != nil {
		t.Fatal(err)
	}
	e := hecha.Entrada
	if hecha.Fichero.Version != 2 || hecha.Fichero.ID != f.ID {
		t.Fatalf("fichero = %+v", hecha.Fichero)
	}
	if a.versiones[f.ID] != 2 || len(a.ficheros) == 0 {
		t.Fatalf("versiones = %d", a.versiones[f.ID])
	}
	// La foto (20×40 ya orientada) girada 90° más: 40×20.
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(a.contenido[f.ID]))
	if err != nil || cfg.Width != 40 || cfg.Height != 20 {
		t.Fatalf("la versión nueva mide %dx%d (%v)", cfg.Width, cfg.Height, err)
	}
	if !e.Exif || !e.Tomada.Equal(time.Date(2026, 7, 14, 18, 32, 5, 0, time.UTC)) {
		t.Fatalf("la edición perdió la fecha de captura: %+v", e)
	}
	if !e.Favorita || !slices.Equal(e.Albumes, []string{album.ID}) || e.Ancho != 40 || e.Alto != 20 {
		t.Fatalf("entrada = %+v", e)
	}
	despues := leerIndice(t, a).Fotos[0]
	if despues.Pendiente || despues.Entrada.Suma != a.ficheros[f.ID].Checksum {
		t.Fatalf("la entrada no es la de la versión nueva: %+v", despues)
	}
}

// Sin EXIF en la foto, la edición sigue con la fecha que ya se enseñaba (la de
// subida del original), no la de la subida de la versión.
func TestEditarSinExifConservaLaFecha(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imagenDe(8, 4), nil); err != nil {
		t.Fatal(err)
	}
	f := subirFoto(t, a, "sin.jpg", buf.Bytes())
	g := Nueva(a, nil)
	if err := g.Indexar(ctx, f, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	foto := leerIndice(t, a).Fotos[0]
	receta := model.Receta{Version: model.VersionReceta, Pasos: []model.Paso{{Tipo: model.PasoEspejo, Eje: "h"}}}
	hecha, err := g.Editar(ctx, foto, receta)
	if err != nil {
		t.Fatal(err)
	}
	if e := hecha.Entrada; e.Exif || !e.Tomada.Equal(foto.Entrada.Tomada) {
		t.Fatalf("tomada %v, quería %v", e.Tomada, foto.Entrada.Tomada)
	}
}

// Una foto que ya no está en «Fotos» (o es de otra carpeta) no se edita.
func TestEditarFueraDeFotos(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	f := subirFoto(t, a, "IMG.jpg", datos)
	foto := cifradaDe(f.ID, time.Now())
	foto.Fichero = f
	foto.Fichero.FolderID = "otra"
	if _, err := Nueva(a, nil).GuardarEdicion(ctx, foto, datos, ""); !errors.Is(err, ErrSinFoto) {
		t.Fatal("una foto de otra carpeta no se toca", err)
	}
	if a.versiones[f.ID] != 1 {
		t.Fatal("se escribió una versión")
	}
}

// Meter y sacar de un álbum toca solo la entrada, y repetirlo no escribe.
func TestPonerEnAlbum(t *testing.T) {
	ctx := context.Background()
	a := nuevoAlmacen()
	datos := fotoConExif(t)
	f := subirFoto(t, a, "IMG.jpg", datos)
	g := Nueva(a, nil)
	album, err := g.CrearAlbum(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.PonerEnAlbum(ctx, f, "../x", true); !errors.Is(err, ErrSinAlbum) {
		t.Fatal("un id de álbum raro no se escribe", err)
	}
	if _, err := g.PonerEnAlbum(ctx, f, album.ID, true); err != nil {
		t.Fatal(err)
	}
	antes := a.subidas
	if _, err := g.PonerEnAlbum(ctx, f, album.ID, true); err != nil || a.subidas != antes {
		t.Fatal("repetir no escribe", err)
	}
	if got := leerIndice(t, a).DelAlbum(album.ID); len(got) != 1 {
		t.Fatalf("en el álbum = %d", len(got))
	}
	e, err := g.PonerEnAlbum(ctx, f, album.ID, false)
	if err != nil || len(e.Albumes) != 0 {
		t.Fatalf("sacar: %+v %v", e, err)
	}
	if got := leerIndice(t, a).DelAlbum(album.ID); len(got) != 0 {
		t.Fatalf("sigue en el álbum = %d", len(got))
	}
	if versiones := a.versiones[f.ID]; versiones != 1 {
		t.Fatal("los álbumes no tocan la foto", versiones)
	}
}

func imagenDe(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 40), B: 90, A: 255})
		}
	}
	return img
}

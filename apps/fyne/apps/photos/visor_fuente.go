// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/galeriacifrada"
)

// # De dónde saca el visor la foto
//
// El visor a pantalla completa enseña dos clases de foto en la misma lista (la línea de
// tiempo mezclada, ADR 0086): las del servidor y las de la galería cifrada (`cifrada:`).
// Lo que es de la foto y no del visor —la foto entera del tamaño real, «Descargar» y si
// admite lo que pasa por el servidor— lo da su [fuenteVisor]:
//
//   - [fuenteServidor]: la exportación del servicio de Photos, con su receta.
//   - [fuenteCifrada]: el original de la bóveda, descifrado en memoria
//     ([galeriacifrada.Galeria.Imagen], [galeriacifrada.Galeria.Exportar]). La descarga
//     se escribe SOLO por [App.Guardar], que es donde la persona guarda sus descargas;
//     nada pasa por un fichero temporal ni por la caché.
//
// La foto grande la pinta la Fototeca del visor, que para las cifradas pide a
// [miniaturasCifradas.grande] y nunca a la red.

// errSinFuente: la foto no se puede leer (sin cliente del servidor, o la galería cifrada
// ya no la tiene).
var errSinFuente = errors.New("photos: la foto no se puede leer")

// fuenteVisor es de dónde saca el visor lo que es de la foto abierta.
type fuenteVisor interface {
	// disponible: hay de dónde leerla.
	disponible() bool
	// delServidor: la foto vive en el servidor y admite lo que pasa por él (Sytue AI,
	// parecidas, cerca, girar, la papelera, el mapa, la galería del sistema).
	delServidor() bool
	// original es la foto entera, decodificada, para el tamaño real.
	original(ctx context.Context, f api.Photo) (image.Image, error)
	// exportar es la foto lista para guardar en `formato` ("jpeg" o "png").
	exportar(ctx context.Context, f api.Photo, formato string) ([]byte, error)
}

// fuenteDe es la fuente de una foto del visor.
func (a *App) fuenteDe(f api.Photo) fuenteVisor {
	if EsCifrada(f.ID) {
		c, ok := a.cifradaDeLinea(f.ID)
		if !ok {
			return fuenteCifrada{}
		}
		return fuenteCifrada{g: a.cif.galeria, foto: c}
	}
	return fuenteServidor{cli: a.cli}
}

// fuenteServidor es una foto de la galería del servidor.
type fuenteServidor struct{ cli *api.Photos }

func (s fuenteServidor) disponible() bool { return s.cli != nil }
func (fuenteServidor) delServidor() bool  { return true }

// formatoOriginal es el formato en que se pide la foto entera: PNG si lo era, JPEG si no.
func formatoOriginal(f api.Photo) string {
	if strings.Contains(f.ContentType, "png") {
		return "png"
	}
	return "jpeg"
}

func (s fuenteServidor) original(ctx context.Context, f api.Photo) (image.Image, error) {
	if s.cli == nil {
		return nil, errSinFuente
	}
	r, err := s.cli.Export(ctx, f.ID, formatoOriginal(f))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	img, _, err := image.Decode(r)
	return img, err
}

func (s fuenteServidor) exportar(ctx context.Context, f api.Photo, formato string) ([]byte, error) {
	if s.cli == nil {
		return nil, errSinFuente
	}
	r, err := s.cli.Export(ctx, f.ID, formato)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// fuenteCifrada es una foto de la galería cifrada, en «Fotos» del Drive de la bóveda.
type fuenteCifrada struct {
	g    *galeriacifrada.Galeria
	foto galeriacifrada.Foto
}

func (c fuenteCifrada) disponible() bool { return c.g != nil && c.foto.Fichero.ID != "" }
func (fuenteCifrada) delServidor() bool  { return false }

func (c fuenteCifrada) original(ctx context.Context, _ api.Photo) (image.Image, error) {
	if !c.disponible() {
		return nil, errSinFuente
	}
	return c.g.Imagen(ctx, c.foto.Fichero)
}

func (c fuenteCifrada) exportar(ctx context.Context, _ api.Photo, formato string) ([]byte, error) {
	if !c.disponible() {
		return nil, errSinFuente
	}
	var out bytes.Buffer
	if err := c.g.Exportar(ctx, c.foto.Fichero, formato, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---- Los álbumes de una foto cifrada en el visor ----

// albumesCifradosDe son los álbumes cifrados como los enseña el menú del visor, y en
// cuáles está la foto.
func (a *App) albumesCifradosDe(c galeriacifrada.Foto) ([]api.Album, map[string]bool) {
	lista := make([]api.Album, 0, len(a.cif.indice.Albumes))
	for _, al := range a.cif.indice.Albumes {
		lista = append(lista, api.Album{ID: al.ID, Title: al.Titulo})
	}
	dentro := map[string]bool{}
	for _, al := range c.Entrada.Albumes {
		if slices.ContainsFunc(lista, func(x api.Album) bool { return x.ID == al }) {
			dentro[al] = true
		}
	}
	return lista, dentro
}

// visorCifrada ajusta el visor a una foto cifrada: su favorita y sus álbumes se
// escriben solo en el índice de la bóveda, «Editar» abre el editor cifrado y lo que
// pasa por el servidor (Sytue AI, parecidas, cerca, girar, la papelera, el mapa, la
// galería del sistema) no se ofrece; la ficha dice por qué (`gallery.encrypted.note`).
func (a *App) visorCifrada(d *Visor, f api.Photo) {
	d.Cifrada = true
	d.Descripcion, d.Etiquetas, d.Accion = "", nil, nil
	d.ConBorrar, d.ConDescribir = false, false
	d.AlDescribir, d.AlGirar, d.AlBorrar, d.AlVerIA = nil, nil, nil, nil
	d.AlParecidas, d.AlCerca, d.AlMapa = nil, nil, nil
	d.AlPreguntar, d.AlEditarDesc, d.AlGuardarDesc = nil, nil, nil
	d.AlSistema, d.ClaveSistema = nil, ""
	d.AlEditar, d.AlMenuAlbumes, d.AlAnyadirA, d.AlFavorita = nil, nil, nil, nil
	c, ok := a.cifradaDeLinea(f.ID)
	d.ConFavorita = ok
	if !ok || a.cif.galeria == nil {
		return
	}
	d.AlFavorita = func() { a.favoritaCifrada(c, !c.Entrada.Favorita) }
	d.AlEditar = func() { a.abrirEditorCifrado(c) }
	lista, dentro := a.albumesCifradosDe(c)
	d.EnAlbumes = dentro
	d.AlMenuAlbumes = func(mostrar func([]api.Album)) { mostrar(lista) }
	// Marcar es meterla; desmarcar, sacarla.
	d.AlAnyadirA = func(al api.Album) { a.albumCifrado(c, al.ID, !dentro[al.ID]) }
	d.AlNuevoAlbum = func() { a.nuevoAlbumCifrado(&c) }
}

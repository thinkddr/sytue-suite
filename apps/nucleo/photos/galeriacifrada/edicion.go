// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package galeriacifrada

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/exif"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/render"
)

// # Editar y los álbumes
//
// Editar una foto cifrada es lo mismo que en la web: la receta (giro, espejo, recorte,
// exposición, contraste, saturación, temperatura) se aplica en el dispositivo a la foto
// entera, con el motor de Photos (`photos/render`), y el resultado sube como una
// VERSIÓN NUEVA del mismo fichero de «Fotos». El original sigue en el historial de
// versiones de Drive: no se pierde nada. Luego la entrada se rehace con el contenido
// nuevo, conservando la fecha de captura (la edición sale sin EXIF; ver
// [Galeria.Indexar]), la favorita y los álbumes.

// ErrVersion: la edición no quedó como versión del mismo fichero (otro dispositivo lo
// cambió o lo movió entretanto). No se reintenta solo: lo que hay en Drive manda.
var ErrVersion = errors.New("galeriacifrada: la edición no es una versión de la misma foto")

// ErrRecetaVacia: no hay nada que guardar.
var ErrRecetaVacia = errors.New("galeriacifrada: la receta no cambia nada")

// GuardarEdicion sube `editada` como versión nueva de la foto `foto` y rehace su
// entrada. Si el almacén sabe reemplazar un fichero por la versión vista
// ([api.DriveFileReplacer], como el Drive de la bóveda), lo usa: una foto que otro
// dispositivo ya cambió no se pisa. Si no, sube con el mismo nombre en la misma
// carpeta, que en Drive es una versión nueva del mismo fichero. Devuelve la foto con
// su fichero y su entrada nuevos (la versión vista para la siguiente edición).
func (g *Galeria) GuardarEdicion(ctx context.Context, foto Foto, editada []byte, tipo string) (Foto, error) {
	if !idValido(foto.Fichero.ID) {
		return Foto{}, ErrSinFoto
	}
	carpeta, err := g.carpeta(ctx, "fotos", false)
	if err != nil {
		return Foto{}, err
	}
	if carpeta == "" || foto.Fichero.FolderID != "" && foto.Fichero.FolderID != carpeta {
		return Foto{}, ErrSinFoto
	}
	if tipo == "" {
		tipo = "image/jpeg"
	}
	original := foto.Fichero
	if original.FolderID == "" {
		original.FolderID = carpeta
	}
	var subida api.File
	if r, ok := g.a.(api.DriveFileReplacer); ok {
		subida, err = r.ReplaceFile(ctx, original, tipo, bytes.NewReader(editada), int64(len(editada)))
	} else {
		subida, err = g.a.Upload(ctx, carpeta, original.Name, tipo, bytes.NewReader(editada), int64(len(editada)))
	}
	if err != nil {
		return Foto{}, err
	}
	if subida.ID != foto.Fichero.ID {
		return Foto{}, ErrVersion
	}
	// Sin EXIF en la edición ni entrada previa con él, la fecha es la que ya se
	// enseñaba (no la de esta subida).
	indexada := subida
	if !foto.Entrada.Tomada.IsZero() {
		indexada.CreatedAt = foto.Entrada.Tomada
	}
	if err := g.Indexar(ctx, indexada, editada); err != nil {
		// La versión ya está a salvo en Drive; la entrada se reconstruye después.
		return Foto{}, err
	}
	e, hay, err := g.entradaActual(ctx, foto.Fichero.ID)
	if err != nil {
		return Foto{}, err
	}
	if !hay {
		return Foto{}, ErrEntrada
	}
	return Foto{Fichero: subida, Entrada: e, ConEntrada: true, Pendiente: e.Miniatura == ""}, nil
}

// Editar aplica `receta` a la foto entera (descifrada solo en memoria) y la guarda
// como versión nueva con [Galeria.GuardarEdicion]. Devuelve la foto rehecha.
func (g *Galeria) Editar(ctx context.Context, foto Foto, receta model.Receta) (Foto, error) {
	if len(receta.Pasos) == 0 {
		return Foto{}, ErrRecetaVacia
	}
	if err := receta.Valida(); err != nil {
		return Foto{}, err
	}
	if foto.Fichero.SizeBytes > render.MaxOriginal {
		return Foto{}, miniatura.ErrMedioNoSoportado
	}
	editada, err := g.renderizar(ctx, foto.Fichero.ID, receta)
	if err != nil {
		return Foto{}, err
	}
	defer clear(editada)
	return g.GuardarEdicion(ctx, foto, editada, "image/jpeg")
}

// renderizar descifra el original, aplica la orientación EXIF y la receta, y lo
// codifica en JPEG con la calidad de exportar de Photos.
func (g *Galeria) renderizar(ctx context.Context, id string, receta model.Receta) ([]byte, error) {
	release, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	datos, err := g.leerFichero(ctx, id, render.MaxOriginal)
	if err != nil {
		return nil, err
	}
	defer clear(datos)
	meta, _ := exif.Leer(datos) // sin EXIF no es un fallo
	img, err := render.Imagen(ctx, datos, model.OrientacionDe(int(meta.Orientacion)), receta)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, render.AplanarSobreBlanco(img), &jpeg.Options{Quality: render.CalidadExportacion}); err != nil {
		return nil, fmt.Errorf("galeriacifrada: codificar edición: %w", err)
	}
	return out.Bytes(), ctx.Err()
}

// LeerOriginal descifra el original de una foto, en memoria, hasta `tope` bytes.
func (g *Galeria) LeerOriginal(ctx context.Context, f api.File, tope int64) ([]byte, error) {
	if !idValido(f.ID) {
		return nil, ErrSinFoto
	}
	if tope <= 0 || tope > render.MaxOriginal {
		tope = render.MaxOriginal
	}
	if f.SizeBytes > tope {
		return nil, miniatura.ErrMedioNoSoportado
	}
	return g.leerFichero(ctx, f.ID, tope)
}

// PonerEnAlbum mete (`dentro`) o saca la foto del álbum `album`, solo en su entrada
// del índice. Repetirlo no escribe nada.
func (g *Galeria) PonerEnAlbum(ctx context.Context, foto api.File, album string, dentro bool) (Entrada, error) {
	if !idValido(album) {
		return Entrada{}, ErrSinAlbum
	}
	return g.Actualizar(ctx, foto, func(e *Entrada) {
		if dentro {
			if !slices.Contains(e.Albumes, album) {
				e.Albumes = append(e.Albumes, album)
			}
			return
		}
		e.Albumes = slices.DeleteFunc(e.Albumes, func(a string) bool { return a == album })
	})
}
